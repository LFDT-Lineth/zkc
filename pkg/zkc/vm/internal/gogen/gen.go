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
	memByID  map[uint]memInfo
	inputs   []memInfo
	outputs  []memInfo
	sroms    []memInfo // static read-only memories (baked contents)
	rams     []memInfo // read-write scratch memories
	modules  []instruction.Module
	funcByID map[uint]*wordFunction
	usesBits bool   // whether math/bits is referenced (decides the import)
	cur      fnView // the function currently being emitted (return shape)
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

// fail emits `if <cond> { return <zeros>, fmt.Errorf(<msg>) }`, where the return
// shape matches the function currently being emitted (g.cur).
func (g *generator) fail(c *code, cond, msg string) {
	c.linef("if %s {", cond)
	c.line(g.returnErr(fmt.Sprintf("fmt.Errorf(%q)", msg)))
	c.line("}")
}

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

	c.line("func run(in map[string][]uint64) (map[string][]uint64, error) {")

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
	c.line("out := map[string][]uint64{}")

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
	//
	c.commentf("%s corresponds to ZkC function %q.", goFuncName(fn), fn.Name())
	c.linef("func %s(%s) %s {", goFuncName(fn), strings.Join(paramFmt, ", "), retType)
	// Declare the non-input registers (zero-init matches the VM frame).
	for i := ni; i < uint(len(regs)); i++ {
		id := register.NewId(i)
		c.linef("var %s uint64", reg(id))
		c.linef("_ = %s", reg(id))
	}

	if err := g.emitFunctionBody(c, fn); err != nil {
		return err
	}

	c.line("}")
	c.line("")

	return nil
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

			c.commentf("gogen input [%d.%d] %s: %s", vi, ci, opName(insn.OpCode()), insn.String(mapping))

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

// emitCall emits a Go call to the callee function, mirroring CallStack.Enter /
// Leave: arguments are width-checked against the callee's input registers, and
// returns are width-checked against the caller's target registers.
func (g *generator) emitCall(c *code, fn *wordFunction, x *instruction.Call) error {
	callee, ok := g.funcByID[x.Id]
	if !ok {
		return fmt.Errorf("gogen: CALL to non-function module id %d", x.Id)
	}

	calleeInputs := callee.Inputs()
	if len(x.Arguments) != len(calleeInputs) {
		return fmt.Errorf("gogen: CALL argument count mismatch (%d vs %d) for %q",
			len(x.Arguments), len(calleeInputs), callee.Name())
	}

	if len(x.Returns) != int(callee.NumOutputs()) {
		return fmt.Errorf("gogen: CALL return count mismatch (%d vs %d) for %q",
			len(x.Returns), callee.NumOutputs(), callee.Name())
	}

	args, err := g.operands(fn, x.Arguments)
	if err != nil {
		return err
	}

	var inner error

	c.block(func() {
		// Argument width checks against the callee parameter widths.
		for i, arg := range args {
			if w := calleeInputs[i].Width(); !calleeInputs[i].IsNative() && w < 64 {
				g.fail(c, fmt.Sprintf("%s >= (1 << %d)", arg, w),
					fmt.Sprintf("bit overflow (value exceeds u%d)", w))
			}
		}

		call := fmt.Sprintf("%s(%s)", goFuncName(callee), strings.Join(args, ", "))
		if len(x.Returns) == 0 {
			c.linef("if e := %s; e != nil {", call)
			c.line(g.returnErr("e"))
			c.line("}")

			return
		}
		// Capture returns in temporaries, then width-check and assign them into
		// the caller's target registers.
		tmps := make([]string, len(x.Returns))
		for i := range x.Returns {
			tmps[i] = fmt.Sprintf("ret%d", i)
		}

		c.linef("%s, e := %s", strings.Join(tmps, ", "), call)
		c.line("if e != nil {")
		c.line(g.returnErr("e"))
		c.line("}")

		for i, target := range x.Returns {
			w, e := g.regWidth(fn, target)
			if e != nil {
				inner = e
				return
			}

			if w < 64 {
				g.fail(c, fmt.Sprintf("%s >= (1 << %d)", tmps[i], w),
					fmt.Sprintf("bit overflow (value exceeds u%d)", w))
			}

			c.linef("%s = %s", reg(target), tmps[i])
		}
	})

	return inner
}

// emitArith dispatches the three integer arithmetic opcodes.  Each computes a
// value into local `v` and then stores it via emitStore.
func (g *generator) emitArith(c *code, fn *wordFunction, x *instruction.WordTypeA[word.Uint64]) error {
	// BIT_CONCAT shares the WordTypeA shape (vector target, register sources) but
	// has its own packing semantics.
	if x.Op == opcode.BIT_CONCAT {
		return g.emitConcat(c, fn, x)
	}

	store, err := g.buildStore(fn, x.Target)
	if err != nil {
		return err
	}

	srcs, err := g.operands(fn, x.Sources)
	if err != nil {
		return err
	}

	konst := x.Constant.Uint64()
	switch x.Op {
	case opcode.INT_ADD:
		g.emitAdd(c, srcs, konst, store)
	case opcode.INT_SUB:
		if len(srcs) == 0 {
			return fmt.Errorf("gogen: INT_SUB with no sources unsupported")
		}

		g.emitSub(c, srcs, konst, store)
	case opcode.INT_MUL:
		g.emitMul(c, srcs, konst, store)
	default:
		return fmt.Errorf("gogen: unsupported arithmetic op %s", opName(x.Op))
	}

	return nil
}

// emitAdd emits `v = const + Σ sources`, with a carry-out check per addition
// (matching executeAdd's val.Add), then StoreAcross.  A constant-zero operand
// adds nothing and is skipped (it can never carry).
func (g *generator) emitAdd(c *code, sources []string, konst uint64, store storeView) {
	c.block(func() {
		c.linef("v := uint64(%d)", konst)

		adds := nonZero(sources)
		if len(adds) > 0 {
			g.usesBits = true

			c.line("var carry uint64")
		}

		for _, s := range adds {
			c.linef("v, carry = bits.Add64(v, %s, 0)", s)
			g.fail(c, "carry != 0", "arithmetic overflow")
		}

		g.emitStore(c, store)
	})
}

// emitSub emits `v = sources[0] - sources[1] - … - const`, each step checked for
// underflow (matching executeSub), then StoreAcross.
func (g *generator) emitSub(c *code, sources []string, konst uint64, store storeView) {
	c.block(func() {
		c.linef("v := %s", sources[0])

		rest := sources[1:]
		if len(rest) > 0 || konst != 0 {
			g.usesBits = true

			c.line("var borrow uint64")
		}

		for _, s := range rest {
			c.linef("v, borrow = bits.Sub64(v, %s, 0)", s)
			g.fail(c, "borrow != 0", "arithmetic underflow")
		}

		if konst != 0 {
			c.linef("v, borrow = bits.Sub64(v, uint64(%d), 0)", konst)
			g.fail(c, "borrow != 0", "arithmetic underflow")
		}

		g.emitStore(c, store)
	})
}

// emitMul emits `v = const · Π sources`, flagging overflow when a 64-bit product
// overflows and the low word is non-zero (matching executeMul), then
// StoreAcross.
func (g *generator) emitMul(c *code, sources []string, konst uint64, store storeView) {
	c.block(func() {
		c.linef("v := uint64(%d)", konst)

		if len(sources) > 0 {
			g.usesBits = true

			c.line("var ov bool")
			c.line("var hi uint64")

			for _, s := range sources {
				c.linef("hi, v = bits.Mul64(v, %s)", s)
				c.line("ov = ov || hi != 0")
			}

			g.fail(c, "ov && v != 0", "arithmetic overflow")
		}

		g.emitStore(c, store)
	})
}

// emitStore writes the accumulated value `v` into the target, mirroring
// StackFrame.StoreAcross:
//   - single register: bit-width-check, then assign;
//   - multi register: distribute v big-endian (lowest register = LSB), masking
//     each register to its width; any bits left beyond the total width are an
//     overflow.  This is where carry bits land in the higher registers.
func (g *generator) emitStore(c *code, store storeView) {
	if store.single != nil {
		w := store.single.width
		if w < 64 {
			g.fail(c, fmt.Sprintf("v >= (1 << %d)", w), fmt.Sprintf("bit overflow (value exceeds u%d)", w))
		}

		c.linef("%s = v", store.single.reg)

		return
	}

	for _, l := range store.limbs {
		if l.width < 64 {
			c.linef("%s = v & ((1 << %d) - 1)", l.reg, l.width)
		} else {
			c.linef("%s = v", l.reg)
		}

		c.linef("v >>= %d", l.width)
	}

	g.fail(c, "v != 0", "bit overflow (value exceeds total target width)")
}

// emitConcat emits a BIT_CONCAT (`tn::…::t0 = sn::…::s0`): it packs the source
// registers into one value with sources[0] in the least-significant bits, then
// distributes that value across the (possibly multi-limb) target via StoreAcross.
// Mirrors executeConcat in the reference word machine.
func (g *generator) emitConcat(c *code, fn *wordFunction, x *instruction.WordTypeA[word.Uint64]) error {
	store, err := g.buildStore(fn, x.Target)
	if err != nil {
		return err
	}

	type source struct {
		expr  string
		width uint
	}

	srcs := make([]source, len(x.Sources))
	for i, id := range x.Sources {
		expr, err := g.operand(fn, id)
		if err != nil {
			return err
		}

		w, err := g.regWidth(fn, id)
		if err != nil {
			return err
		}

		srcs[i] = source{expr, w}
	}

	c.block(func() {
		c.line("var v uint64")
		// Build from the most-significant source down so sources[0] ends up in the
		// low bits (each source is shifted in above the ones already placed).
		for i := len(srcs); i > 0; i-- {
			c.linef("v = (v << %d) | %s", srcs[i-1].width, srcs[i-1].expr)
		}

		g.emitStore(c, store)
	})

	return nil
}

// emitBitwise emits a WordTypeB bitwise/shift op into its single target register,
// mirroring executeAnd/Or/Xor/Not/Shl/Shr.  AND/OR/XOR and SHR map to the plain Go
// operators; NOT and SHL additionally mask to the operation bit-width (matching
// word.Not / word.Shl).  The result is then bit-width-checked against the target
// via emitStore (i.e. frame.Store).
func (g *generator) emitBitwise(c *code, fn *wordFunction, x *instruction.WordTypeB) error {
	store, err := g.buildStore(fn, register.NewVector(x.Target))
	if err != nil {
		return err
	}

	lhs, err := g.operand(fn, x.LeftSource)
	if err != nil {
		return err
	}

	// BIT_NOT is unary; the rest read a right operand.
	var rhs string
	if x.Op != opcode.BIT_NOT {
		if rhs, err = g.operand(fn, x.RightSource); err != nil {
			return err
		}
	}

	var valExpr string

	switch x.Op {
	case opcode.BIT_AND:
		valExpr = fmt.Sprintf("%s & %s", lhs, rhs)
	case opcode.BIT_OR:
		valExpr = fmt.Sprintf("%s | %s", lhs, rhs)
	case opcode.BIT_XOR:
		valExpr = fmt.Sprintf("%s ^ %s", lhs, rhs)
	case opcode.BIT_NOT:
		valExpr = maskExpr(fmt.Sprintf("^%s", lhs), x.Bitwidth)
	case opcode.BIT_SHL:
		valExpr = maskExpr(fmt.Sprintf("%s << %s", lhs, rhs), x.Bitwidth)
	case opcode.BIT_SHR:
		valExpr = fmt.Sprintf("%s >> %s", lhs, rhs)
	default:
		// INT_DIV / INT_REM are not on the supported path yet (§6.5).
		return fmt.Errorf("gogen: unsupported bitwise op %s", opName(x.Op))
	}

	c.block(func() {
		c.linef("v := %s", valExpr)
		g.emitStore(c, store)
	})

	return nil
}

// maskExpr masks expr to the low bitwidth bits, mirroring word.mask64: a width of
// 64 or more needs no mask (the full word is already in range, and Go's shift
// already yields 0 when the count reaches the word width).
func maskExpr(expr string, bitwidth uint) string {
	if bitwidth >= 64 {
		return expr
	}

	return fmt.Sprintf("(%s) & ((1 << %d) - 1)", expr, bitwidth)
}

// emitMemRead emits a read from a readable memory (input ROM, static ROM or RAM
// scratch): decode the address, then load each data word into its target register
// (bit-width-checked).
func (g *generator) emitMemRead(c *code, fn *wordFunction, x *instruction.MemRead) error {
	mi, ok := g.memByID[x.Id]
	if !ok {
		return fmt.Errorf("gogen: MEMORY_READ from unknown module id %d", x.Id)
	}

	switch mi.role {
	case romInput, sromStatic, ramScratch:
	default:
		return fmt.Errorf("gogen: MEMORY_READ from non-readable memory %q", mi.name)
	}

	start, err := g.addrExpr(fn, mi, x.Address())
	if err != nil {
		return err
	}

	var inner error

	c.block(func() {
		c.linef("start := %s", start)

		for i, d := range x.Data() {
			w, e := g.regWidth(fn, d)
			if e != nil {
				inner = e
				return
			}

			idx := i

			c.block(func() {
				if mi.role == ramScratch {
					// RAM is zero-initialised: an unwritten cell reads 0.
					c.linef("val := memGet(%s, start+%d)", mi.varName, idx)
				} else {
					// ROM/SROM read is data[address]; OOB panics, like StaticArray.Read.
					c.linef("val := %s[start+%d]", mi.varName, idx)
				}

				if w < 64 {
					g.fail(c, fmt.Sprintf("val >= (1 << %d)", w), fmt.Sprintf("bit overflow (value exceeds u%d)", w))
				}

				c.linef("%s = val", reg(d))
			})
		}
	})

	return inner
}

// emitMemWrite emits a write to a writable memory (output WOM or RAM scratch):
// decode the address, then store each data word (checked against the memory's
// data-register width).  Both back onto memGrow (grow-on-write).
func (g *generator) emitMemWrite(c *code, fn *wordFunction, x *instruction.MemWrite) error {
	mi, ok := g.memByID[x.Id]
	if !ok {
		return fmt.Errorf("gogen: MEMORY_WRITE to unknown module id %d", x.Id)
	}

	switch mi.role {
	case womOutput, ramScratch:
	default:
		return fmt.Errorf("gogen: MEMORY_WRITE to non-writable memory %q", mi.name)
	}

	start, err := g.addrExpr(fn, mi, x.Address())
	if err != nil {
		return err
	}

	dataRegs := mi.geom.DataRegisters()
	if len(x.Data()) != len(dataRegs) {
		return fmt.Errorf("gogen: MEMORY_WRITE data lines mismatch (%d vs %d)", len(x.Data()), len(dataRegs))
	}

	var inner error

	c.block(func() {
		c.linef("start := %s", start)

		for i, s := range x.Data() {
			src, e := g.operand(fn, s)
			if e != nil {
				inner = e
				return
			}

			idx := i
			w := dataRegs[i].Width() // mem write checks the memory data-register width

			c.block(func() {
				c.linef("val := %s", src)

				if w < 64 {
					g.fail(c, fmt.Sprintf("val >= (1 << %d)", w), fmt.Sprintf("bit overflow (value exceeds u%d)", w))
				}

				c.linef("%s = memGrow(%s, start+%d, val)", mi.varName, mi.varName, idx)
			})
		}
	})

	return inner
}

// ===========================================================================
// Control-flow helpers
// ===========================================================================

// labelName renders the Go label for a 2-D PC position.
func labelName(p pos) string { return fmt.Sprintf("L_%d_%d", p.macro, p.micro) }

// skipTarget computes the destination of a skip/skip_if at (vi, ci) skipping
// `skip` micro-instructions.  Per the VM (machine/base.go), a skip advances the
// micro counter to ci+skip and then falls through one step, so the destination
// is ci+skip+1; if that lands past the end of the vector it falls through to the
// start of the next macro vector.
func skipTarget(vi, ci, skip, vecLen uint) pos {
	micro := ci + skip + 1
	if micro >= vecLen {
		return pos{vi + 1, 0}
	}

	return pos{vi, micro}
}

// collectLabels gathers every 2-D PC position targeted by a skip or jump, so the
// emitter knows exactly which positions need a Go label (Go rejects unused
// labels, so we must not over-emit).
func collectLabels(code wordCode) map[pos]bool {
	labels := map[pos]bool{}

	for vi, vec := range code {
		n := uint(len(vec.Codes))
		for ci, insn := range vec.Codes {
			switch x := insn.(type) {
			case *instruction.Skip:
				labels[skipTarget(uint(vi), uint(ci), x.Skip, n)] = true
			case *instruction.SkipIf:
				labels[skipTarget(uint(vi), uint(ci), x.Skip, n)] = true
			case *instruction.Jump:
				labels[pos{x.Immediate, 0}] = true
			}
		}
	}

	return labels
}

// condExpr renders the boolean Go expression under which a SkipIf takes its
// skip.  Vectors are compared lexicographically with the most-significant
// register at the highest index, matching machine/base.go's cmp.
func (g *generator) condExpr(fn *wordFunction, x *instruction.SkipIf) (string, error) {
	lhs, err := g.operands(fn, x.Left.Registers())
	if err != nil {
		return "", err
	}

	rhs, err := g.operands(fn, x.Right.Registers())
	if err != nil {
		return "", err
	}

	if len(lhs) != len(rhs) {
		return "", fmt.Errorf("gogen: skip_if compares vectors of differing length (%d vs %d)", len(lhs), len(rhs))
	}

	switch x.Cond {
	case opcode.EQ:
		return eqExpr(lhs, rhs), nil
	case opcode.NEQ:
		return "!(" + eqExpr(lhs, rhs) + ")", nil
	case opcode.LT:
		return ordExpr(lhs, rhs, "<"), nil
	case opcode.GT:
		return ordExpr(lhs, rhs, ">"), nil
	case opcode.LTEQ:
		return "!(" + ordExpr(lhs, rhs, ">") + ")", nil
	case opcode.GTEQ:
		return "!(" + ordExpr(lhs, rhs, "<") + ")", nil
	default:
		return "", fmt.Errorf("gogen: unsupported skip condition 0x%x", uint(x.Cond))
	}
}

// eqExpr renders elementwise equality of two operand lists.
func eqExpr(lhs, rhs []string) string {
	parts := make([]string, len(lhs))
	for i := range lhs {
		parts[i] = fmt.Sprintf("%s == %s", lhs[i], rhs[i])
	}

	return strings.Join(parts, " && ")
}

// ordExpr renders a strict lexicographic comparison (op is "<" or ">") of two
// operand lists, most significant register first.
func ordExpr(lhs, rhs []string, op string) string {
	var build func(i int) string

	build = func(i int) string {
		if i == 0 {
			return fmt.Sprintf("(%s %s %s)", lhs[0], op, rhs[0])
		}

		return fmt.Sprintf("(%s %s %s || (%s == %s && %s))",
			lhs[i], op, rhs[i], lhs[i], rhs[i], build(i-1))
	}

	return build(len(lhs) - 1)
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

	switch x.Op {
	case opcode.INT_ADD:
		switch {
		case len(x.Sources) == 0:
			return fmt.Sprintf("materialize literal %d into %s; this can look like a no-op, but lowered "+
				"memory ops and constants need values in registers.", konst, target)
		case len(x.Sources) == 1 && konst == 0:
			return fmt.Sprintf("copy %s into %s, then enforce the target bit width.", regDebug(fn, x.Sources[0]), target)
		default:
			return fmt.Sprintf("add into local v with carry checks, then StoreAcross into %s.", target)
		}
	case opcode.INT_SUB:
		return fmt.Sprintf("subtract into local v with borrow checks, then StoreAcross into %s.", target)
	case opcode.INT_MUL:
		return fmt.Sprintf("multiply into local v with 64-bit overflow tracking, then StoreAcross into %s.", target)
	case opcode.BIT_CONCAT:
		return fmt.Sprintf("concatenate sources (sources[0] in the low bits) into v, then StoreAcross into %s.", target)
	default:
		return ""
	}
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
