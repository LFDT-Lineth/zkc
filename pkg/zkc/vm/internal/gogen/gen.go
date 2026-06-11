// Copyright Consensys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
// specific language governing permissions and limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

// Package gogen compiles a ZkC WordMachine into native Go source: the "fast
// execution mode" alternative to interpreting the machine.
//
// The generator consumes the machine over word.Uint — the same machine the
// reference executor interprets — so its semantics are an exact mirror of
// pkg/zkc/vm/internal/machine (word.go, stack_frame.go, base.go):
//
//   - Arithmetic is EXACT (word.Uint is unbounded): there is no accumulator
//     overflow.  All width enforcement happens at store time, where a single
//     register store checks the declared bit width and a multi-register store
//     distributes the value across the limbs (lowest register = least
//     significant) and fails only on bits beyond the total width.
//   - The generator picks a Go representation per result from a static bound
//     derived from register widths: plain uint64 when the bound fits 64 bits,
//     a lo/hi pair (math/bits) up to 128 bits, and a clean "unsupported" error
//     beyond that.  The same bounds prove most store-width checks dead, so
//     they are simply not emitted.
//   - Failures (width checks, underflow, division by zero, FAIL) panic with a
//     `failure` value recovered once in Run, so generated functions have plain
//     value signatures with no error plumbing.
//   - The 2-D program counter (macro vector + micro code) becomes labelled Go:
//     skips and jumps are gotos between labelled positions.
//   - Functions become Go functions (the Go stack is the call stack, matching
//     CallStack.Enter/Leave); shared memories become package-level globals.
//
// Registers wider than 64 bits, constants beyond 64 bits and moduli beyond 64
// bits are rejected with descriptive errors (wide-register support is future
// work); callers treat these programs as out of scope, not failures.
package gogen

import (
	"fmt"
	"go/format"
	"math/big"
	"regexp"
	"slices"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction/opcode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/function"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/machine"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/memory"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// Config controls the shape of the generated artefact.
type Config struct {
	// Package is the name of the generated Go package.  The package "main"
	// additionally gets a JSON stdin/stdout main() harness, used by the
	// differential tests; any other name yields an importable package whose
	// only entry point is Run.
	Package string
	// NoIntervals disables the flow-sensitive interval analysis, falling back
	// to width-derived bounds only (more emitted checks, otherwise identical
	// semantics).  A debugging escape hatch.
	NoIntervals bool
	// Source, when non-empty, is recorded in the generated header (e.g.
	// "keccakf.zkc sha256:ab12…"), so go:generate workflows can detect a stale
	// artefact by comparing it against the current source.
	Source string
}

type wordFunction = function.Function[instruction.Word]

// wordCode is the body of a function: a slice of (vectorised) macro instructions.
type wordCode = []instruction.Vector[instruction.Word]

type memRole int

const (
	romInput     memRole = iota // non-static read-only: loaded from the program inputs
	womOutput                   // write-once: forms the program outputs (grow-on-write)
	sromStatic                  // static read-only: fixed contents baked into the program
	ramScratch                  // read-write scratch: zero-initialised, grows on write
	pagedScratch                // paged read-write scratch (demand-allocated pages)
)

type memInfo struct {
	name    string
	varName string
	role    memRole
	geom    memory.Geometry[word.Uint]
	// contents holds the baked initial values of a static (SROM) memory; nil
	// for all other roles.
	contents []uint64
}

// pos is a 2-D program-counter position: a macro vector index plus a micro code
// index within that vector.  It is the target of skips and jumps.
type pos struct {
	macro uint
	micro uint
}

// Generate compiles a word machine into a self-contained Go source file
// exposing Run(inputs) (outputs, error).  See the package documentation for
// the semantics contract and Config for the artefact shape.
func Generate(wm *machine.Word[word.Uint], cfg Config) (string, error) {
	if cfg.Package == "" {
		cfg.Package = "main"
	}

	g := &generator{
		pkg:         cfg.Package,
		noIntervals: cfg.NoIntervals,
		source:      cfg.Source,
		memByID:     map[uint]memInfo{},
		funcByID:    map[uint]*wordFunction{},
		modules:     wm.Modules(),
		modulus:     wm.Executor().Modulus().BigInt(),
		names:       map[string]bool{},
	}
	mainID, hasMain := uint(0), false

	for id, m := range wm.Modules() {
		switch mm := m.(type) {
		case memory.Memory[word.Uint]:
			info, err := g.classifyMemory(mm)
			if err != nil {
				return "", err
			}

			g.memByID[uint(id)] = info
			switch info.role {
			case romInput:
				g.inputs = append(g.inputs, info)
			case womOutput:
				g.outputs = append(g.outputs, info)
			case sromStatic:
				g.sroms = append(g.sroms, info)
			case ramScratch:
				g.rams = append(g.rams, info)
			case pagedScratch:
				g.pageds = append(g.pageds, info)
			}
		case *wordFunction:
			g.funcByID[uint(id)] = mm
			if mm.Name() == "main" {
				mainID, hasMain = uint(id), true
			}
		default:
			return "", fmt.Errorf("gogen: unsupported module %q (%T)", m.Name(), m)
		}
	}

	if !hasMain {
		return "", fmt.Errorf("gogen: no 'main' function found")
	}
	// Only emit functions reachable from main (transitively via CALL); this keeps
	// generation scoped to the program actually being executed and avoids failing
	// on unrelated helper functions that use out-of-scope instructions.
	order := g.reachableFunctions(mainID)
	// Emit each function body first so g.usesBits is known before the imports.
	bodies := map[uint]*code{}

	for _, id := range order {
		fn := g.funcByID[id]
		if fn.IsNative() {
			return "", fmt.Errorf("gogen: native function %q unsupported", fn.Name())
		}

		var b code
		if err := g.emitFunction(&b, fn); err != nil {
			return "", err
		}

		bodies[id] = &b
	}
	// Assemble the full file.
	var file code
	g.emitFile(&file, order, bodies)

	return formatSource(file.String())
}

type generator struct {
	pkg         string
	noIntervals bool
	source      string // provenance line for the generated header (may be empty)
	memByID     map[uint]memInfo
	inputs      []memInfo
	outputs     []memInfo
	sroms       []memInfo // static read-only memories (baked contents)
	rams        []memInfo // read-write scratch memories
	pageds      []memInfo // paged read-write scratch memories
	modules     []instruction.Module
	funcByID    map[uint]*wordFunction
	modulus     *big.Int        // the machine's prime modulus (for mod-P ops)
	names       map[string]bool // sanitized identifiers already taken
	usesBits    bool            // whether math/bits is referenced (decides the import)
	usesShl128  bool            // whether the 128-bit left-shift helper is referenced
	usesShr128  bool            // whether the 128-bit right-shift helper is referenced
	usesModP    fieldHelpers    // which mod-P helpers are referenced
	cur         fnView          // the function currently being emitted (return shape)
	iv          *intervals      // bound analysis for the function being emitted
}

// fnView captures the parts of the function currently being emitted that the
// per-instruction emitters need: how to spell a RETURN.
type fnView struct {
	isBoot   bool     // the boot frame ('main'); its outputs are discarded
	outNames []string // output result names, limb-expanded (empty for boot)
}

// reachableFunctions returns the ids of the functions reachable from the entry
// (transitively via CALL), with the entry first and the rest in ascending id
// order for deterministic output.
func (g *generator) reachableFunctions(entry uint) []uint {
	seen := map[uint]bool{entry: true}
	worklist := []uint{entry}

	var rest []uint

	for len(worklist) > 0 {
		id := worklist[0]
		worklist = worklist[1:]

		fn := g.funcByID[id]
		for _, vec := range fn.Code() {
			for _, insn := range vec.Codes {
				if call, ok := insn.(*instruction.Call); ok && !seen[call.Id] {
					seen[call.Id] = true
					worklist = append(worklist, call.Id)
					rest = append(rest, call.Id)
				}
			}
		}
	}

	slices.Sort(rest)

	return append([]uint{entry}, rest...)
}

// ===========================================================================
// code: a tiny line-oriented Go source buffer
// ===========================================================================

// code accumulates generated Go line by line.  It intentionally does NOT manage
// indentation — formatSource (gofmt) reformats the result — which keeps the
// generation logic free of whitespace bookkeeping.
type code struct{ b strings.Builder }

// line appends a single line of source.
func (c *code) line(s string) { c.b.WriteString(s); c.b.WriteByte('\n') }

// linef appends a single formatted line of source.
func (c *code) linef(format string, a ...any) { fmt.Fprintf(&c.b, format, a...); c.b.WriteByte('\n') }

// commentf appends a sanitized line comment.
func (c *code) commentf(format string, a ...any) {
	if text := commentText(fmt.Sprintf(format, a...)); text != "" {
		c.linef("// %s", text)
	}
}

// raw appends already-rendered source verbatim (no trailing newline added).
func (c *code) raw(s string) { c.b.WriteString(s) }

// block emits `{ <body> }`, scoping any temporaries the body declares (and
// letting a forward goto legally jump over the whole block).
func (c *code) block(body func()) {
	c.line("{")
	body()
	c.line("}")
}

func (c *code) String() string { return c.b.String() }

// ===========================================================================
// File scaffold
// ===========================================================================

// emitFile writes the package, imports, failure helpers, memory globals and
// helpers, the Run entry point, every reachable function, and (for package
// main) the JSON stdin/stdout harness.
func (g *generator) emitFile(c *code, order []uint, bodies map[uint]*code) {
	c.line("// Code generated by zkc gogen. DO NOT EDIT.")

	if g.source != "" {
		c.linef("// Source: %s", g.source)
	}

	c.linef("package %s", g.pkg)
	c.line("")
	g.emitImports(c)
	emitFailureHelpers(c)

	// Memories are shared across the call stack, so they live as package-level
	// globals (matching the VM's shared memory banks); callees read/write them
	// without threading slices through every signature.  Run resets them, so
	// the package is single-execution-at-a-time (like the VM itself).
	for _, m := range g.inputs {
		c.linef("var %s []uint64 // input ROM %q", m.varName, m.name)
	}

	for _, m := range g.outputs {
		c.linef("var %s []uint64 // output WOM %q", m.varName, m.name)
	}

	for _, m := range g.rams {
		c.linef("var %s []uint64 // scratch RAM %q", m.varName, m.name)
	}

	for _, m := range g.pageds {
		c.linef("var %s paged // paged RAM %q", m.varName, m.name)
	}

	// Static read-only memories have fixed contents baked in as a literal.
	for _, m := range g.sroms {
		c.linef("var %s = %s // static ROM %q", m.varName, sromLiteral(m.contents), m.name)
	}

	c.line("")
	g.emitMemHelpers(c)
	g.emitShiftHelpers(c)
	g.emitModPHelpers(c)

	c.line("// Run executes the program on the given input memories, returning its")
	c.line("// output memories, or an error if the execution fails (a rejected trace).")
	c.line("func Run(in map[string][]uint64) (out map[string][]uint64, err error) {")
	c.line("defer func() {")
	c.line("if r := recover(); r != nil {")
	c.line("if f, ok := r.(failure); ok {")
	c.line("out, err = nil, f")
	c.line("return")
	c.line("}")
	c.line("panic(r)")
	c.line("}")
	c.line("}()")

	for _, m := range g.inputs {
		c.linef("%s = in[%q]", m.varName, m.name)
	}
	// Outputs and scratch memories start empty on every run.
	for _, m := range g.outputs {
		c.linef("%s = nil", m.varName)
	}

	for _, m := range g.rams {
		c.linef("%s = nil", m.varName)
	}

	for _, m := range g.pageds {
		c.linef("%s = paged{}", m.varName)
	}

	c.line("fn_main()")
	c.line("out = map[string][]uint64{}")

	for _, m := range g.outputs {
		c.linef("out[%q] = %s", m.name, m.varName)
	}

	c.line("return out, nil")
	c.line("}")
	c.line("")

	for _, id := range order {
		c.raw(bodies[id].String())
	}

	if g.pkg == "main" {
		c.raw(mainHarness)
	}
}

func (g *generator) emitImports(c *code) {
	deps := []string{}
	if g.pkg == "main" {
		deps = append(deps, `"bufio"`, `"encoding/json"`, `"fmt"`, `"os"`)
	}

	if g.usesBits {
		deps = append(deps, `"math/bits"`)
	}
	// memGrow (WOM/RAM) and paged.set both use slices.Grow.
	if len(g.outputs) > 0 || len(g.rams) > 0 || len(g.pageds) > 0 {
		deps = append(deps, `"slices"`)
	}

	switch len(deps) {
	case 0:
		return
	case 1:
		c.linef("import %s", deps[0])
	default:
		slices.Sort(deps)
		c.line("import (")

		for _, d := range deps {
			c.line(d)
		}

		c.line(")")
	}

	c.line("")
}

// emitMemHelpers writes the grow-on-write / zero-on-miss helpers backing the
// writable memories, each only when some memory of that kind exists.
func (g *generator) emitMemHelpers(c *code) {
	// memGrow backs both write-once (WOM) outputs and read-write (RAM) scratch:
	// it grows the slice to cover addr and stores v, matching the VM's
	// grow-on-write memories.
	if len(g.outputs) > 0 || len(g.rams) > 0 {
		c.line("func memGrow(s []uint64, addr uint64, v uint64) []uint64 {")
		c.line("if n := addr + 1; uint64(len(s)) < n {")
		c.line("s = slices.Grow(s, int(n)-len(s))[:n]")
		c.line("}")
		c.line("s[addr] = v")
		c.line("return s")
		c.line("}")
		c.line("")
	}

	// memGet reads RAM scratch: an unwritten cell reads 0, matching the VM's
	// zero-initialised RandomAccess memory.
	if len(g.rams) > 0 {
		c.line("func memGet(s []uint64, addr uint64) uint64 {")
		c.line("if addr < uint64(len(s)) {")
		c.line("return s[addr]")
		c.line("}")
		c.line("return 0")
		c.line("}")
		c.line("")
	}

	// paged mirrors the VM's PagedRandomAccess: a page table indexed densely by
	// address/pageSize, pages allocated on first write, unwritten reads are 0.
	if len(g.pageds) > 0 {
		c.line("const pageSize = 1 << 20")
		c.line("")
		c.line("type paged struct{ pages [][]uint64 }")
		c.line("")
		c.line("func (m *paged) get(addr uint64) uint64 {")
		c.line("page, off := addr/pageSize, addr%pageSize")
		c.line("if page < uint64(len(m.pages)) && m.pages[page] != nil {")
		c.line("return m.pages[page][off]")
		c.line("}")
		c.line("return 0")
		c.line("}")
		c.line("")
		c.line("func (m *paged) set(addr uint64, v uint64) {")
		c.line("page, off := addr/pageSize, addr%pageSize")
		c.line("if n := page + 1; uint64(len(m.pages)) < n {")
		c.line("m.pages = slices.Grow(m.pages, int(n)-len(m.pages))[:n]")
		c.line("}")
		c.line("if m.pages[page] == nil {")
		c.line("m.pages[page] = make([]uint64, pageSize)")
		c.line("}")
		c.line("m.pages[page][off] = v")
		c.line("}")
		c.line("")
	}
}

// mainHarness reads JSON inputs from stdin, runs the program, and writes JSON
// outputs to stdout.  Exit 1 signals an execution error (matching the reference
// error path); exit 2 signals an I/O/harness problem.
const mainHarness = `func main() {
	var in map[string][]uint64
	if err := json.NewDecoder(bufio.NewReader(os.Stdin)).Decode(&in); err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(2)
	}
	out, err := Run(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "exec error:", err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(2)
	}
}
`

// ===========================================================================
// Function emission
// ===========================================================================

// goFuncName returns the Go function name for a ZkC function.
func goFuncName(fn *wordFunction) string { return "fn_" + sanitize(fn.Name()) }

// emitFunction emits a complete Go function: signature, register locals, body.
// Input registers become parameters (named rN so the operand helpers address
// them unchanged) and output registers become plain results.  The boot frame
// ('main') takes no parameters — its inputs are zero, matching CallStack.Boot —
// and discards its outputs.
func (g *generator) emitFunction(c *code, fn *wordFunction) error {
	var (
		isBoot   = fn.Name() == "main"
		ni       = fn.NumInputs()
		no       = fn.NumOutputs()
		regs     = fn.Registers()
		outNames []string
		retType  string
		paramFmt []string
		params   = uint(0)
	)

	if !isBoot {
		params = ni
		for i := range ni {
			l, err := g.limbOf(fn, register.NewId(i))
			if err != nil {
				return err
			}

			if l.width > 64 {
				paramFmt = append(paramFmt, fmt.Sprintf("%s, %s uint64", l.lo(), l.hiName()))
			} else {
				paramFmt = append(paramFmt, fmt.Sprintf("%s uint64", l.lo()))
			}
		}

		for i := range no {
			l, err := g.limbOf(fn, register.NewId(ni+i))
			if err != nil {
				return err
			}

			outNames = append(outNames, l.lo())
			if l.width > 64 {
				outNames = append(outNames, l.hiName())
			}
		}

		switch len(outNames) {
		case 0:
		case 1:
			retType = " uint64"
		default:
			retType = " (" + strings.TrimSuffix(strings.Repeat("uint64, ", len(outNames)), ", ") + ")"
		}
	}

	g.cur = fnView{isBoot: isBoot, outNames: outNames}
	// Emit the body to a fixpoint: each pass replays the emitters (the
	// analysis transfer function) over the whole body; label states stabilise
	// within a few passes thanks to widening (see intervals).  The final
	// stable pass IS the emitted body.  Helper flags are reset per pass so
	// only the final pass decides them.
	var (
		body               code
		savedBits, savedMP = g.usesBits, g.usesModP
	)

	g.iv = newIntervals(fn, isBoot, g.noIntervals)

	for pass := 0; ; pass++ {
		body = code{}
		g.usesBits, g.usesModP = savedBits, savedMP
		g.iv.beginPass()

		if err := g.emitFunctionBody(&body, fn); err != nil {
			return err
		}

		if g.iv.stable() {
			break
		}
		// Safety valve: an unexpected non-terminating fixpoint falls back to
		// width-derived bounds (the next pass is then trivially stable).
		if pass >= 8 {
			g.iv.disabled = true
		}
	}

	c.commentf("%s corresponds to ZkC function %q.", goFuncName(fn), fn.Name())
	c.linef("func %s(%s)%s {", goFuncName(fn), strings.Join(paramFmt, ", "), retType)

	used, read := usedRegisters(body.String())

	for i := params; i < uint(len(regs)); i++ {
		l, err := g.limbOf(fn, register.NewId(i))
		if err != nil {
			return err
		}

		tokens := []string{l.lo()}
		if l.width > 64 {
			tokens = append(tokens, l.hiName())
		}

		for _, tok := range tokens {
			if !used[tok] {
				continue
			}

			c.linef("var %s uint64 // %s", tok, regs[i].Name())
			// Go rejects a variable that is only ever assigned; a write-only
			// register (e.g. a dead destructure limb) needs a blank use.
			if !read[tok] {
				c.linef("_ = %s", tok)
			}
		}
	}

	c.raw(body.String())
	c.line("}")
	c.line("")

	return nil
}

// usedRegisters scans generated code for register-limb tokens (rN, rN_0,
// rN_1), so emitFunction only declares the locals the body actually uses —
// and, separately, which of them are ever READ (an emitted assignment always
// has the `rX[, rY…] = rhs` shape, so tokens left of a leading `=` are writes;
// everything else is a read).  Comments are excluded: they quote ZkC register
// names, which may themselves look like rN.
func usedRegisters(body string) (used, read map[string]bool) {
	used, read = map[string]bool{}, map[string]bool{}

	collect := func(s string, into map[string]bool) {
		for _, tok := range regRefPattern.FindAllString(s, -1) {
			into[tok] = true
		}
	}

	for line := range strings.SplitSeq(body, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}

		if m := assignPattern.FindStringSubmatch(line); m != nil {
			collect(m[1], used) // assignment targets: used, not read
			collect(m[2], read) // right-hand side: reads
			collect(m[2], used)
		} else {
			collect(line, read)
			collect(line, used)
		}
	}

	return used, read
}

var (
	regRefPattern = regexp.MustCompile(`\br\d+(?:_[01])?\b`)
	// assignPattern matches the emitted assignment shape `rX[, rY…] = rhs`
	// (plain `=` only — the emitters never use register tokens with `:=` or
	// compound ops).
	assignPattern = regexp.MustCompile(`^\s*(r\d+(?:_[01])?(?:, r\d+(?:_[01])?)*) = (.*)$`)
)

// returnOk renders the `return` performed by a RETURN instruction: the boot
// frame discards its outputs (matching CallStack.Leave at depth 0), whereas a
// callee returns its output registers (limb-expanded).
func (g *generator) returnOk() string {
	if len(g.cur.outNames) == 0 {
		return "return"
	}

	return "return " + strings.Join(g.cur.outNames, ", ")
}

// ===========================================================================
// Instruction emission
// ===========================================================================

// emitFunctionBody walks the (vectorised) code, emitting one block per micro-
// instruction in program order, preceded by a label wherever a skip or jump
// targets that position.  A final (unreachable for well-formed code) panic
// satisfies Go's terminating-statement requirement and models falling off the
// end of the function.
func (g *generator) emitFunctionBody(c *code, fn *wordFunction) error {
	mapping := instruction.NewSystemMap(fn.RegisterMap(), g.modules)
	code := fn.Code()
	labels := collectLabels(code)

	for vi, vec := range code {
		n := uint(len(vec.Codes))
		for ci, insn := range vec.Codes {
			at := pos{uint(vi), uint(ci)}
			if labels[at] {
				c.linef("%s:", labelName(at))
				g.iv.atLabel(at)
			}

			c.commentf("[%d.%d] %s: %s", vi, ci, opName(insn.OpCode()), insn.String(mapping))

			if err := g.emitInstruction(c, fn, insn, uint(vi), uint(ci), n); err != nil {
				return err
			}
		}
	}
	// Fall-off-the-end target: only a malformed (non-terminating) program reaches
	// it, but it also guarantees the Go function ends in a terminating statement.
	// When the last instruction already transfers control and nothing jumps past
	// the end, the panic would be unreachable (go vet objects) — omit it.
	end := pos{uint(len(code)), 0}
	if labels[end] {
		c.linef("%s:", labelName(end))
	}

	if labels[end] || !endsTerminated(code) {
		c.line(`panic(failure("machine fell off end of function"))`)
	}

	return nil
}

// endsTerminated reports whether the final instruction of the code (if any)
// unconditionally transfers control, i.e. execution cannot fall off the end.
func endsTerminated(code wordCode) bool {
	if len(code) == 0 {
		return false
	}

	last := code[len(code)-1].Codes

	if len(last) == 0 {
		return false
	}

	switch last[len(last)-1].(type) {
	case *instruction.Skip, *instruction.Jump, *instruction.Return, *instruction.Fail:
		return true
	default:
		return false
	}
}

func (g *generator) emitInstruction(c *code, fn *wordFunction, insn instruction.Word, vi, ci, vecLen uint) error {
	switch x := insn.(type) {
	case *instruction.WordTypeA[word.Uint]:
		return g.emitArith(c, fn, x)
	case *instruction.WordTypeB:
		return g.emitTypeB(c, fn, x)
	case *instruction.WordTypeF[word.Uint]:
		return g.emitFieldOp(c, fn, x)
	case *instruction.FieldHint:
		return g.emitHint(c, fn, x)
	case *instruction.Debug:
		// DEBUG only prints diagnostics; it has no effect on program outputs.
		return nil
	case *instruction.MemRead:
		return g.emitMemRead(c, fn, x)
	case *instruction.MemWrite:
		return g.emitMemWrite(c, fn, x)
	case *instruction.Call:
		return g.emitCall(c, fn, x)
	case *instruction.Skip:
		target := skipTarget(vi, ci, x.Skip, vecLen)
		c.linef("goto %s", labelName(target))
		g.iv.edgeTo(target)
		g.iv.endOfFlow()

		return nil
	case *instruction.SkipIf:
		cond, err := g.condExpr(fn, x)
		if err != nil {
			return err
		}

		target := skipTarget(vi, ci, x.Skip, vecLen)

		c.linef("if %s {", cond)
		c.linef("goto %s", labelName(target))
		c.line("}")
		g.iv.edgeTo(target)

		return nil
	case *instruction.Jump:
		target := pos{x.Immediate, 0}
		c.linef("goto %s", labelName(target))
		g.iv.edgeTo(target)
		g.iv.endOfFlow()

		return nil
	case *instruction.Return:
		c.line(g.returnOk())
		g.iv.endOfFlow()

		return nil
	case *instruction.Fail:
		c.line(`panic(failure("machine panic"))`)
		g.iv.endOfFlow()

		return nil
	default:
		return fmt.Errorf("gogen: unsupported instruction %T (op 0x%x)", insn, uint8(insn.OpCode()))
	}
}

// ===========================================================================
// Misc helpers
// ===========================================================================

// reg returns the Go local name for a register id.
func reg(id register.Id) string { return fmt.Sprintf("r%d", id.Unwrap()) }

func (g *generator) regWidth(fn *wordFunction, id register.Id) (uint, error) {
	r := fn.Register(id)
	if r.IsNative() {
		return 0, fmt.Errorf("gogen: native register r%d unsupported", id.Unwrap())
	}
	// Registers up to 64 bits are single uint64 locals; up to 128 bits they
	// are rN_0/rN_1 limb pairs.
	if w := r.Width(); w <= 128 {
		return w, nil
	}

	return 0, fmt.Errorf("gogen: register %q wider than 128 bits (u%d) unsupported", r.Name(), r.Width())
}

// uintConst converts a word.Uint constant into a uint64, erroring on wider
// constants (which only occur alongside wide registers, equally unsupported).
func uintConst(w word.Uint) (uint64, error) {
	if !w.FitsWithin(64) {
		return 0, fmt.Errorf("gogen: constant 0x%s wider than 64 bits unsupported", w.Text(16))
	}

	return w.Uint64(), nil
}

func (g *generator) classifyMemory(m memory.Memory[word.Uint]) (memInfo, error) {
	info := memInfo{name: m.Name(), varName: g.uniqueName("mem_" + sanitize(m.Name())), geom: m.Geometry()}
	// All memory traffic moves through uint64 cells.
	for _, r := range append(info.geom.AddressRegisters(), info.geom.DataRegisters()...) {
		if !r.IsNative() && r.Width() > 64 {
			return info, fmt.Errorf("gogen: memory %q register %q wider than 64 bits unsupported", m.Name(), r.Name())
		}
	}

	switch mm := m.(type) {
	case *memory.PagedRandomAccess[word.Uint]:
		info.role = pagedScratch
	case *memory.StaticReadOnly[word.Uint]:
		// Static read-only: bake the fixed contents into the generated program.
		info.role = sromStatic

		contents, err := toU64s(mm.Contents())
		if err != nil {
			return info, fmt.Errorf("gogen: memory %q: %w", m.Name(), err)
		}

		info.contents = contents
	default:
		switch {
		case m.IsReadOnly():
			info.role = romInput
		case m.IsWriteOnly():
			info.role = womOutput
		case m.IsReadWrite():
			info.role = ramScratch
		default:
			return info, fmt.Errorf("gogen: unsupported memory %q (%T)", m.Name(), m)
		}
	}

	return info, nil
}

// toU64s converts SROM contents into plain uint64s for baking.
func toU64s(words []word.Uint) ([]uint64, error) {
	out := make([]uint64, len(words))

	for i, w := range words {
		v, err := uintConst(w)
		if err != nil {
			return nil, err
		}

		out[i] = v
	}

	return out, nil
}

// sromLiteral renders a Go []uint64 literal for a static memory's baked contents.
func sromLiteral(contents []uint64) string {
	parts := make([]string, len(contents))
	for i, v := range contents {
		parts[i] = fmt.Sprintf("0x%x", v)
	}

	return "[]uint64{" + strings.Join(parts, ", ") + "}"
}

// uniqueName reserves a sanitized identifier, suffixing on collision (distinct
// ZkC names may sanitize to the same Go identifier).
func (g *generator) uniqueName(name string) string {
	for i := 0; ; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s_%d", name, i)
		}

		if !g.names[candidate] {
			g.names[candidate] = true
			return candidate
		}
	}
}

func sanitize(name string) string {
	var b strings.Builder

	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}

	return b.String()
}

func commentText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// formatSource runs gofmt over generated source, surfacing a numbered listing on
// failure to make codegen bugs easy to locate.
func formatSource(src string) (string, error) {
	out, err := format.Source([]byte(src))
	if err != nil {
		return "", fmt.Errorf("gogen: generated source is invalid Go: %w\n%s", err, numberLines(src))
	}

	return string(out), nil
}

func opName(op opcode.OpCode) string {
	switch op {
	case opcode.INT_ADD:
		return "INT_ADD"
	case opcode.INT_SUB:
		return "INT_SUB"
	case opcode.INT_MUL:
		return "INT_MUL"
	case opcode.INT_DIV:
		return "INT_DIV"
	case opcode.INT_REM:
		return "INT_REM"
	case opcode.INT_ADDMOD_P:
		return "INT_ADDMOD_P"
	case opcode.INT_SUBMOD_P:
		return "INT_SUBMOD_P"
	case opcode.INT_MULMOD_P:
		return "INT_MULMOD_P"
	case opcode.BIT_AND:
		return "BIT_AND"
	case opcode.BIT_OR:
		return "BIT_OR"
	case opcode.BIT_XOR:
		return "BIT_XOR"
	case opcode.BIT_NOT:
		return "BIT_NOT"
	case opcode.BIT_SHL:
		return "BIT_SHL"
	case opcode.BIT_SHR:
		return "BIT_SHR"
	case opcode.BIT_CONCAT:
		return "BIT_CONCAT"
	case opcode.HINT_DIVISION:
		return "HINT_DIVISION"
	case opcode.MEMORY_READ:
		return "MEMORY_READ"
	case opcode.MEMORY_WRITE:
		return "MEMORY_WRITE"
	case opcode.RETURN:
		return "RETURN"
	case opcode.JUMP:
		return "JUMP"
	case opcode.SKIP:
		return "SKIP"
	case opcode.SKIP_IF:
		return "SKIP_IF"
	case opcode.CALL:
		return "CALL"
	case opcode.FAIL:
		return "FAIL"
	case opcode.DEBUG:
		return "DEBUG"
	default:
		return fmt.Sprintf("op(0x%x)", uint8(op))
	}
}

func numberLines(src string) string {
	var b strings.Builder
	for i, line := range strings.Split(src, "\n") {
		fmt.Fprintf(&b, "%4d\t%s\n", i+1, line)
	}

	return b.String()
}
