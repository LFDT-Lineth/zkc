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

// Package gogen generates native Go source from a (u64) ZkC WordMachine, as a
// "fast execution mode" alternative to the bytecode interpreter.
//
// Scope (see plan_gogenerate.md):
//
//   - Phase 1: straight-line u64 arithmetic (INT_ADD/SUB/MUL, incl. multi-register
//     StoreAcross), MEMORY_READ (input ROMs), MEMORY_WRITE (output WOMs) and
//     RETURN.
//   - Phase 2: control flow.  SKIP/SKIP_IF model intra-vector branching (the
//     micro half of the 2-D PC) as forward/backward gotos; JUMP models the macro
//     half as a goto to a vector-entry label; FAIL maps to an error return.
//   - Phase 3: calls.  Each ZkC function becomes a Go function; CALL copies
//     arguments/returns with the same width checks the reference call stack
//     performs (machine/call_stack.go), and a non-boot RETURN returns the
//     function's output registers.  Shared memories become package-level globals
//     so callees can read/write them, matching the VM's shared memory banks.
//
// Semantics faithfully mirror the reference WordExecutor
// (pkg/zkc/vm/internal/machine/word.go + stack_frame.go + base.go):
//
//   - Arithmetic accumulates at the machine-word bandwidth (u64); a carry/borrow
//     OUT of the word is an "arithmetic overflow/underflow" error, exactly as
//     val.Add / val.Sub.
//   - The result is then written via StoreAcross (emitStore): a single-register
//     target is bit-width-checked; a MULTI-register target distributes the value
//     big-endian (lowest register = least significant), so carry bits are
//     CAPTURED into the higher registers rather than discarded.
//   - The 2-D program counter (macro vector + micro code) becomes labelled Go:
//     every macro vector is a sequence of micro-instructions, and skips/jumps
//     transfer control between labelled positions.
//
// The generator emits unindented Go and relies on go/format.Source (gofmt) to
// reformat, so the emission logic reads as ordinary imperative Go.
package gogen

import (
	"fmt"
	"go/format"
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

type wordFunction = function.Function[instruction.Word]

// wordCode is the body of a function: a slice of (vectorised) macro instructions.
type wordCode = []instruction.Vector[instruction.Word]

type memRole int

const (
	romInput   memRole = iota // non-static read-only: loaded from the program inputs
	womOutput                 // write-once: forms the program outputs (grow-on-write)
	sromStatic                // static read-only: fixed contents baked into the program
	ramScratch                // read-write scratch: zero-initialised, grows on write
)

type memInfo struct {
	name    string
	varName string
	role    memRole
	geom    memory.Geometry[word.Uint64]
	// contents holds the baked initial values of a static (SROM) memory; nil for
	// all other roles.
	contents []uint64
}

// limb identifies a target register and its bit-width (used by emitStore).
type limb struct {
	reg   string // Go lvalue, e.g. "r3"
	width uint
}

// pos is a 2-D program-counter position: a macro vector index plus a micro code
// index within that vector.  It is the target of skips and jumps.
type pos struct {
	macro uint
	micro uint
}

// WordToGoSource compiles a u64 WordMachine into a self-contained, runnable Go
// program (package main) exposing run(in) (out, error) plus a JSON stdin/stdout
// `main`, suitable for differential testing against the reference executor.
func WordToGoSource(wm *machine.Word[word.Uint64]) (string, error) {
	g := &generator{memByID: map[uint]memInfo{}, funcByID: map[uint]*wordFunction{}, modules: wm.Modules()}
	mainID, hasMain := uint(0), false

	for id, m := range wm.Modules() {
		switch mm := m.(type) {
		case memory.Memory[word.Uint64]:
			info, err := classifyMemory(mm)
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
	memByID            map[uint]memInfo
	inputs             []memInfo
	outputs            []memInfo
	sroms              []memInfo // static read-only memories (baked contents)
	rams               []memInfo // read-write scratch memories
	modules            []instruction.Module
	funcByID           map[uint]*wordFunction
	usesBits           bool // whether math/bits is referenced (decides the import)
	usesOverflowCheck  bool
	usesUnderflowCheck bool
	cur                fnView    // the function currently being emitted (return shape)
	curTemps           tempUsage // arithmetic temporaries used by the current function
}

// tempUsage records which shared arithmetic temporaries the function currently
// being emitted needs.  The direct (single-register) arithmetic paths accumulate
// straight into the target register, so their carry/borrow/multiply scratch must
// be declared once at function scope rather than inside a per-instruction block.
type tempUsage struct {
	carry  bool // `carry` for bits.Add64
	borrow bool // `borrow` for bits.Sub64
	mulHi  bool // `hi` for bits.Mul64
	mulOv  bool // `ov` for tracking overflow across a chain of multiplications
}

// fnView captures the parts of the function currently being emitted that the
// per-instruction emitters need: how to spell a successful or failing return.
type fnView struct {
	isBoot  bool          // the boot frame ('main'); its outputs are discarded
	outRegs []register.Id // output register ids (empty for boot)
	zeros   string        // zero literals for the outputs, e.g. "0, 0"
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

// returnErr renders a `return` that propagates an error expression, padding with
// zero values for any output registers the current function declares.
func (g *generator) returnErr(errExpr string) string {
	if g.cur.zeros == "" {
		return "return " + errExpr
	}

	return "return " + g.cur.zeros + ", " + errExpr
}

// returnOk renders the `return` performed by a RETURN instruction: the boot
// frame discards its outputs (matching CallStack.Leave at depth 0), whereas a
// callee returns its output registers.
func (g *generator) returnOk() string {
	if len(g.cur.outRegs) == 0 {
		return "return nil"
	}

	parts := make([]string, len(g.cur.outRegs))
	for i, id := range g.cur.outRegs {
		parts[i] = reg(id)
	}

	return "return " + strings.Join(parts, ", ") + ", nil"
}

// ===========================================================================
// File scaffold
// ===========================================================================

// emitFile writes the package, imports, memory globals, optional WOM helper, the
// run() entry point, every reachable function, and the JSON stdin/stdout main().
func (g *generator) emitFile(c *code, order []uint, bodies map[uint]*code) {
	c.line("// Code generated by zkc gogen. DO NOT EDIT.")
	c.line("package main")
	c.line("")
	c.line("import (")
	c.line(`"bufio"`)
	c.line(`"encoding/json"`)
	c.line(`"fmt"`)

	if g.usesBits {
		c.line(`"math/bits"`)
	}

	c.line(`"os"`)
	c.line(")")
	c.line("")

	// Memories are shared across the call stack, so they live as package-level
	// globals (matching the VM's shared memory banks); callees read/write them
	// without threading slices through every signature.
	for _, m := range g.inputs {
		c.linef("var %s []uint64", m.varName)
	}

	for _, m := range g.outputs {
		c.linef("var %s []uint64", m.varName)
	}

	for _, m := range g.rams {
		c.linef("var %s []uint64", m.varName)
	}

	// Static read-only memories have fixed contents baked in as a literal.
	for _, m := range g.sroms {
		c.linef("var %s = %s", m.varName, sromLiteral(m.contents))
	}

	c.line("")

	// memGrow backs both write-once (WOM) outputs and read-write (RAM) scratch:
	// it grows the slice to cover addr and stores v, matching the VM's
	// grow-on-write memories.
	if len(g.outputs) > 0 || len(g.rams) > 0 {
		c.line("func memGrow(s []uint64, addr uint64, v uint64) []uint64 {")
		c.line("for uint64(len(s)) <= addr { s = append(s, 0) }")
		c.line("s[addr] = v")
		c.line("return s")
		c.line("}")
		c.line("")
	}

	// memGet reads RAM scratch: an unwritten cell reads 0, matching the VM's
	// zero-initialised RandomAccess memory.
	if len(g.rams) > 0 {
		c.line("func memGet(s []uint64, addr uint64) uint64 {")
		c.line("if addr < uint64(len(s)) { return s[addr] }")
		c.line("return 0")
		c.line("}")
		c.line("")
	}

	g.emitCheckHelpers(c)

	c.line("func run(in map[string][]uint64) (out map[string][]uint64, err error) {")

	if g.usesCheckPanic() {
		c.line("defer func() {")
		c.line("if r := recover(); r != nil {")
		c.line("if e, ok := r.(checkError); ok {")
		c.line("out = nil")
		c.line("err = e")
		c.line("return")
		c.line("}")
		c.line("panic(r)")
		c.line("}")
		c.line("}()")
	}

	for _, m := range g.inputs {
		c.linef("%s = in[%q]", m.varName, m.name)
	}

	for _, m := range g.outputs {
		c.linef("%s = nil", m.varName)
	}

	// RAM is zero-initialised scratch: reset it on every run.
	for _, m := range g.rams {
		c.linef("%s = nil", m.varName)
	}

	c.line("if err := fn_main(); err != nil {")
	c.line("return nil, err")
	c.line("}")
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

	c.raw(mainHarness)
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
	out, err := run(in)
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
// them unchanged); the remaining registers are zero-initialised locals.
func (g *generator) emitFunction(c *code, fn *wordFunction) error {
	var (
		isBoot   = fn.Name() == "main"
		ni       = fn.NumInputs()
		no       = fn.NumOutputs()
		regs     = fn.Registers()
		outRegs  []register.Id
		zeros    string
		retType  = "error"
		paramFmt []string
	)
	// Inputs are parameters; the boot frame takes none and discards its outputs.
	for i := range ni {
		paramFmt = append(paramFmt, fmt.Sprintf("%s uint64", reg(register.NewId(i))))
	}

	if !isBoot && no > 0 {
		zeroParts := make([]string, no)
		for i := range no {
			outRegs = append(outRegs, register.NewId(ni+i))
			zeroParts[i] = "0"
		}

		zeros = strings.Join(zeroParts, ", ")
		retType = "(" + strings.Repeat("uint64, ", int(no)) + "error)"
	}

	g.cur = fnView{isBoot: isBoot, outRegs: outRegs, zeros: zeros}
	g.curTemps = tempUsage{}
	// Emit the body first so the set of arithmetic temporaries it needs (g.curTemps)
	// is known before we write their declarations at the top of the function.
	var body code
	if err := g.emitFunctionBody(&body, fn); err != nil {
		return err
	}

	c.commentf("%s corresponds to ZkC function %q.", goFuncName(fn), fn.Name())
	c.linef("func %s(%s) %s {", goFuncName(fn), strings.Join(paramFmt, ", "), retType)
	// Declare the non-input registers (zero-init matches the VM frame).
	for i := ni; i < uint(len(regs)); i++ {
		id := register.NewId(i)
		c.linef("var %s uint64", reg(id))
		c.linef("_ = %s", reg(id))
	}

	g.emitTempDecls(c)
	c.raw(body.String())
	c.line("}")
	c.line("")

	return nil
}

// emitTempDecls declares, at function scope, the shared arithmetic temporaries
// the direct (single-register) paths accumulate through.  Each is only declared
// when actually used, and each is always read where used (e.g. `carry != 0`), so
// no spurious "declared and not used" results.
func (g *generator) emitTempDecls(c *code) {
	if g.curTemps.carry {
		c.line("var carry uint64")
	}

	if g.curTemps.borrow {
		c.line("var borrow uint64")
	}

	if g.curTemps.mulHi {
		c.line("var hi uint64")
	}

	if g.curTemps.mulOv {
		c.line("var ov bool")
	}
}

// ===========================================================================
// Instruction emission
// ===========================================================================

// emitFunctionBody walks the (vectorised) code, emitting one block per micro-
// instruction in program order, preceded by a label wherever a skip or jump
// targets that position.  A final (unreachable for well-formed code) return
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
			}

			c.commentf("[%d.%d] %s: %s", vi, ci, opName(insn.OpCode()), insn.String(mapping))

			if note := g.commentNote(fn, insn); note != "" {
				c.commentf("gogen: %s", note)
			}

			if err := g.emitInstruction(c, fn, insn, uint(vi), uint(ci), n); err != nil {
				return err
			}
		}
	}
	// Fall-off-the-end target: only a malformed (non-terminating) program reaches
	// it, but it also guarantees the Go function ends in a terminating statement.
	end := pos{uint(len(code)), 0}
	if labels[end] {
		c.linef("%s:", labelName(end))
	}

	c.line(g.returnErr(`fmt.Errorf("machine fell off end of function")`))

	return nil
}

func (g *generator) emitInstruction(c *code, fn *wordFunction, insn instruction.Word, vi, ci, vecLen uint) error {
	switch x := insn.(type) {
	case *instruction.WordTypeA[word.Uint64]:
		return g.emitArith(c, fn, x)
	case *instruction.WordTypeB:
		return g.emitBitwise(c, fn, x)
	case *instruction.Debug:
		// DEBUG only prints diagnostics; it has no effect on program outputs (which
		// is all the tests assert), so emit nothing — matching the reference machine.
		return nil
	case *instruction.MemRead:
		return g.emitMemRead(c, fn, x)
	case *instruction.MemWrite:
		return g.emitMemWrite(c, fn, x)
	case *instruction.Call:
		return g.emitCall(c, fn, x)
	case *instruction.Skip:
		c.linef("goto %s", labelName(skipTarget(vi, ci, x.Skip, vecLen)))
		return nil
	case *instruction.SkipIf:
		cond, err := g.condExpr(fn, x)
		if err != nil {
			return err
		}

		c.linef("if %s {", cond)
		c.linef("goto %s", labelName(skipTarget(vi, ci, x.Skip, vecLen)))
		c.line("}")

		return nil
	case *instruction.Jump:
		c.linef("goto %s", labelName(pos{x.Immediate, 0}))
		return nil
	case *instruction.Return:
		c.line(g.returnOk())
		return nil
	case *instruction.Fail:
		c.line(g.returnErr(`fmt.Errorf("machine panic")`))
		return nil
	default:
		return fmt.Errorf("gogen: unsupported instruction %T (op %s)", insn, opName(insn.OpCode()))
	}
}

// ===========================================================================
// Operand / store / address helpers
// ===========================================================================

// storeView models a StoreAcross target: exactly one of single / limbs is set.
type storeView struct {
	single *limb  // single-register target (bit-width-checked)
	limbs  []limb // multi-register target, lowest register first (LSB)
}

// buildStore translates a target register vector into a storeView.
func (g *generator) buildStore(fn *wordFunction, vec register.Vector) (storeView, error) {
	regs := vec.Registers()
	if len(regs) == 1 {
		w, err := g.regWidth(fn, regs[0])
		if err != nil {
			return storeView{}, err
		}

		return storeView{single: &limb{reg: reg(regs[0]), width: w}}, nil
	}

	limbs := make([]limb, len(regs))
	for i, id := range regs {
		w, err := g.regWidth(fn, id)
		if err != nil {
			return storeView{}, err
		}

		limbs[i] = limb{reg: reg(id), width: w}
	}

	return storeView{limbs: limbs}, nil
}

// addrExpr mirrors memory.Geometry.Decode: pack the address registers big-endian
// by their geometry widths, then multiply by the number of data lines.
func (g *generator) addrExpr(fn *wordFunction, mi memInfo, addr []register.Id) (string, error) {
	addrRegs := mi.geom.AddressRegisters()
	if len(addr) != len(addrRegs) {
		return "", fmt.Errorf("gogen: address lines mismatch (%d vs %d) for %q", len(addr), len(addrRegs), mi.name)
	}

	expr := "uint64(0)"

	for i, id := range addr {
		src, err := g.operand(fn, id)
		if err != nil {
			return "", err
		}

		expr = fmt.Sprintf("((%s << %d) | %s)", expr, addrRegs[i].Width(), src)
	}

	return fmt.Sprintf("(%s) * %d", expr, mi.geom.DataLines()), nil
}

func (g *generator) operands(fn *wordFunction, ids []register.Id) ([]string, error) {
	out := make([]string, len(ids))
	for i, id := range ids {
		s, err := g.operand(fn, id)
		if err != nil {
			return nil, err
		}

		out[i] = s
	}

	return out, nil
}

// operand returns a Go expression reading a source register: a literal for a
// constant register, otherwise the register local.
func (g *generator) operand(fn *wordFunction, id register.Id) (string, error) {
	r := fn.Register(id)
	if r.IsNative() {
		return "", fmt.Errorf("gogen: native register r%d unsupported", id.Unwrap())
	}

	if r.IsConst() {
		return fmt.Sprintf("uint64(%d)", r.ConstValue()), nil
	}

	return reg(id), nil
}

func (g *generator) regWidth(fn *wordFunction, id register.Id) (uint, error) {
	r := fn.Register(id)
	if r.IsNative() {
		return 0, fmt.Errorf("gogen: native register r%d unsupported", id.Unwrap())
	}

	return r.Width(), nil
}

// ===========================================================================
// Generated comments
// ===========================================================================

func (g *generator) commentNote(fn *wordFunction, insn instruction.Word) string {
	switch x := insn.(type) {
	case *instruction.WordTypeA[word.Uint64]:
		return arithNote(fn, x)
	case *instruction.WordTypeB:
		return bitwiseNote(fn, x)
	case *instruction.Debug:
		return "DEBUG only prints diagnostics; it has no effect on program outputs."
	case *instruction.MemRead:
		if mi, ok := g.memByID[x.Id]; ok {
			return fmt.Sprintf("read %s memory %q; address registers are packed with memory.Geometry.Decode.",
				roleNote(mi.role), mi.name)
		}
	case *instruction.MemWrite:
		if mi, ok := g.memByID[x.Id]; ok {
			return fmt.Sprintf("write %s memory %q; values are checked against the memory data-line widths.",
				roleNote(mi.role), mi.name)
		}
	case *instruction.Call:
		if callee, ok := g.funcByID[x.Id]; ok {
			return fmt.Sprintf("call %q; arguments/returns are width-checked like CallStack.Enter/Leave.", callee.Name())
		}
	case *instruction.Jump:
		return "JUMP transfers control to the start of another macro vector."
	case *instruction.Skip:
		return "SKIP advances the micro PC unconditionally (intra-vector branch)."
	case *instruction.SkipIf:
		return "SKIP_IF advances the micro PC when the condition holds (intra-vector branch)."
	case *instruction.Return:
		if g.cur.isBoot {
			return "RETURN from the boot frame terminates execution."
		}

		return "RETURN copies the output registers back to the caller."
	case *instruction.Fail:
		return "FAIL aborts execution with a machine panic (an error)."
	}

	return ""
}

func arithNote(fn *wordFunction, x *instruction.WordTypeA[word.Uint64]) string {
	target := vectorDebug(fn, x.Target)
	konst := x.Constant.Uint64()
	// A multi-register target distributes the accumulated value big-endian via
	// StoreAcross; a single-register target is accumulated into directly and
	// bit-width-checked.
	dst := storeNote(fn, x.Target)

	switch x.Op {
	case opcode.INT_ADD:
		switch {
		case len(x.Sources) == 0:
			return ""
		case len(x.Sources) == 1 && konst == 0:
			return fmt.Sprintf("copy %s into %s, then enforce the target bit width.", regDebug(fn, x.Sources[0]), target)
		default:
			return fmt.Sprintf("add the operands with carry checks %s.", dst)
		}
	case opcode.INT_SUB:
		return fmt.Sprintf("subtract with borrow checks %s.", dst)
	case opcode.INT_MUL:
		return fmt.Sprintf("multiply with 64-bit overflow tracking %s.", dst)
	case opcode.BIT_CONCAT:
		return fmt.Sprintf("concatenate sources (sources[0] in the low bits) %s.", dst)
	default:
		return ""
	}
}

// storeNote renders the StoreAcross half of an arithmetic note for a target
// vector: either an accumulate-into-the-register phrase (single register) or a
// distribute-across phrase (multi register).
func storeNote(fn *wordFunction, vec register.Vector) string {
	target := vectorDebug(fn, vec)
	if len(vec.Registers()) > 1 {
		return fmt.Sprintf("then distribute the result across %s via StoreAcross", target)
	}

	return fmt.Sprintf("into %s, enforcing the target bit width", target)
}

// roleNote describes a memory role for generated comments.
func roleNote(r memRole) string {
	switch r {
	case romInput:
		return "input ROM"
	case womOutput:
		return "output WOM"
	case sromStatic:
		return "static ROM"
	case ramScratch:
		return "scratch RAM"
	default:
		return "memory"
	}
}

// bitwiseNote describes a WordTypeB bitwise/shift instruction.
func bitwiseNote(fn *wordFunction, x *instruction.WordTypeB) string {
	target := regDebug(fn, x.Target)

	switch x.Op {
	case opcode.BIT_AND, opcode.BIT_OR, opcode.BIT_XOR:
		return fmt.Sprintf("%s of two registers into %s, then enforce the target bit width.", opName(x.Op), target)
	case opcode.BIT_NOT:
		return fmt.Sprintf("bitwise complement of %s masked to u%d into %s.", regDebug(fn, x.LeftSource), x.Bitwidth, target)
	case opcode.BIT_SHL:
		return fmt.Sprintf("shift %s left (masked to u%d) into %s.", regDebug(fn, x.LeftSource), x.Bitwidth, target)
	case opcode.BIT_SHR:
		return fmt.Sprintf("shift %s right into %s.", regDebug(fn, x.LeftSource), target)
	default:
		return ""
	}
}

func vectorDebug(fn *wordFunction, vec register.Vector) string {
	regs := vec.Registers()

	names := make([]string, 0, len(regs))
	for i := len(regs); i > 0; i-- {
		names = append(names, regDebug(fn, regs[i-1]))
	}

	return strings.Join(names, "::")
}

func regName(fn *wordFunction, id register.Id) string {
	return fn.Register(id).Name()
}

func regDebug(fn *wordFunction, id register.Id) string {
	return fmt.Sprintf("%s/%s", regName(fn, id), reg(id))
}

func commentText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ===========================================================================
// Misc helpers
// ===========================================================================

// reg returns the Go local name for a register id.
func reg(id register.Id) string { return fmt.Sprintf("r%d", id.Unwrap()) }

// nonZero filters out literal "uint64(0)" operands (adding zero never carries),
// which lets emitAdd skip dead carry checks.
func nonZero(operands []string) []string {
	out := operands[:0:0]
	for _, s := range operands {
		if s != "uint64(0)" {
			out = append(out, s)
		}
	}

	return out
}

func classifyMemory(m memory.Memory[word.Uint64]) (memInfo, error) {
	info := memInfo{name: m.Name(), varName: "mem_" + sanitize(m.Name()), geom: m.Geometry()}
	switch {
	case m.IsStatic():
		// Static read-only: bake the fixed contents into the generated program.
		info.role = sromStatic
		info.contents = toU64s(m.Contents())
	case m.IsReadOnly():
		info.role = romInput
	case m.IsWriteOnly():
		info.role = womOutput
	case m.IsReadWrite():
		info.role = ramScratch
	default:
		return info, fmt.Errorf("gogen: unsupported memory %q", m.Name())
	}

	return info, nil
}

// toU64s converts a slice of u64 words into plain uint64s (for baking SROM data).
func toU64s(words []word.Uint64) []uint64 {
	out := make([]uint64, len(words))
	for i, w := range words {
		out[i] = w.Uint64()
	}

	return out
}

// sromLiteral renders a Go []uint64 literal for a static memory's baked contents.
func sromLiteral(contents []uint64) string {
	parts := make([]string, len(contents))
	for i, v := range contents {
		parts[i] = fmt.Sprintf("0x%x", v)
	}

	return "[]uint64{" + strings.Join(parts, ", ") + "}"
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
