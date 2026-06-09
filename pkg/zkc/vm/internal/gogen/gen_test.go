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
// This test lives in an external package (gogen_test) rather than package gogen:
// it drives the generator through the public vm.WordToGoSource entry point, and
// vm imports gogen — so an internal test would form an import cycle.
package gogen_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/decl"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/variable"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/instruction"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

const tt = `pub input data(address:u16) -> (word:u16)

// prove that two numbers add up to a third.
fn main() {
    var n:u16 = data[0]
    var k:u16 = data[1]
    var res:u32 = data[2] as u32

    if pow(n, k) != res {
        fail
    }
}

// compute n^k
fn pow(n:u16, k:u16) -> (res:u32) {
    var i:u16, acc:u32, b:u1
    //
    res = 1
    acc = n as u32
    i = k
    //
    while i != 0 {
        // divide by 2
        i::b = i as u17
        // check odd/even
        if b == 1 {
            res = res * acc
        }
        //
        acc = acc * acc
    }
}
`

// tutorialSrc mirrors pkg/zkc/tutorial: branchless u16 arithmetic with single
// register targets (result[0]=a+b, result[1]=(a+b)*c, result[2]=a-b).
const tutorialSrc = `pub input args(address:u16) -> (word:u16)
pub output result(address:u16) -> (word:u16)
fn main() {
    var a:u16 = args[0]
    var b:u16 = args[1]
    var c:u16 = args[2]
    var sum:u16 = a + b
    result[0] = sum
    result[1] = sum * c
    result[2] = a - b
    return
}
`

// destructSrc exercises a MULTI-register target (carry-style distribution via
// StoreAcross): hi::lo = word splits a u32 across two u16 registers.
const destructSrc = `pub input args(address:u16) -> (w:u32)
pub output result(address:u16) -> (w:u16)
fn main() {
    var word:u32 = args[0]
    var hi:u16
    var lo:u16
    hi::lo = word
    result[0] = hi
    result[1] = lo
}
`

// ---------------------------------------------------------------------------
// Phase 2 fixtures: control flow (SKIP / SKIP_IF / JUMP, intra-vector branches).
// ---------------------------------------------------------------------------

// branchSrc exercises an if/else (SKIP_IF + SKIP): clamp-ish branch where the
// two arms write the same register via different instructions.
const branchSrc = `pub input data(address:u8) -> (byte:u8)
pub output result(address:u8) -> (byte:u8)
fn main() {
    var x:u8 = data[0]
    var y:u8
    if x <= 10 {
        y = x + 1
    } else {
        y = x - 1
    }
    result[0] = y
    return
}
`

// loopSrc exercises a JUMP-based loop with a SKIP_IF guard: acc ends up equal to
// n (n iterations of +1).
const loopSrc = `pub input data(address:u8) -> (byte:u8)
pub output result(address:u8) -> (byte:u8)
fn main() {
    var n:u8 = data[0]
    var acc:u8 = 0
    for i:u8 = 0; i<n; i = i + 1 {
        acc = acc + 1
    }
    result[0] = acc
    return
}
`

// doubleSrc loops r=r+r x times (r=2^x), overflowing u8 once x>=8: a control-flow
// fixture that also exercises error parity inside a loop body.
const doubleSrc = `pub input data(address:u8) -> (byte:u8)
pub output result(address:u8) -> (byte:u8)
fn main() {
    var x:u8 = data[0]
    var r:u8 = 1
    for i:u8 = 0; i<x; i = i + 1 {
        r = r + r
    }
    result[0] = r
    return
}
`

// ---------------------------------------------------------------------------
// Phase 3 fixtures: calls (CALL / non-boot RETURN, argument/return width checks).
// ---------------------------------------------------------------------------

// callSrc exercises a simple non-recursive call returning a value.
const callSrc = `pub input data(address:u16) -> (byte:u8)
pub output result(address:u16) -> (byte:u8)
fn main() {
    result[0] = inc(data[0])
    return
}
fn inc(x:u8) -> (r:u8) {
    r = x + 1
    return
}
`

// callFailSrc exercises a call to a void function that may FAIL, proving the call
// actually executes and error parity is preserved across frames.
const callFailSrc = `pub input data(address:u16) -> (byte:u8)
pub output result(address:u16) -> (byte:u8)
fn check(x:u8) {
    if x == 0 {
        fail
    }
    return
}
fn main() {
    check(data[0])
    result[0] = data[0]
    return
}
`

// recSumSrc exercises recursion: sum(n) = n + (n-1) + ... + 0, which overflows
// u16 for large n (error parity through a deep call stack).
const recSumSrc = `pub input data(address:u16) -> (word:u16)
pub output result(address:u16) -> (word:u16)
fn main() {
    result[0] = sum(data[0])
    return
}
fn sum(n:u16) -> (s:u16) {
    if n == 0 {
        s = 0
        return
    }
    s = n + sum(n - 1)
    return
}
`

// ---------------------------------------------------------------------------
// Phase 7.1 fixtures: bitwise / shift / concat (WordTypeB + BIT_CONCAT).
// ---------------------------------------------------------------------------

// bitwiseSrc exercises AND/OR/XOR (binary) and NOT (unary, width-masked).
const bitwiseSrc = `pub input data(address:u8) -> (byte:u8)
pub output result(address:u8) -> (byte:u8)
fn main() {
    var x:u8 = data[0]
    var y:u8 = data[1]
    result[0] = x & y
    result[1] = x | y
    result[2] = x ^ y
    result[3] = ~x
    return
}
`

// shiftSrc exercises SHL (width-masked) and SHR, including shift amounts >= width
// (Go and the reference word both yield 0 there).
const shiftSrc = `pub input data(address:u8) -> (byte:u8)
pub output result(address:u8) -> (byte:u8)
fn main() {
    var x:u8 = data[0]
    var n:u8 = data[1]
    result[0] = x << n
    result[1] = x >> n
    return
}
`

// concatSrc exercises BIT_CONCAT: byte-swap a u16 by destructuring then
// re-concatenating in the opposite order (sources[0] lands in the low bits).
const concatSrc = `pub input data(address:u8) -> (w:u16)
pub output result(address:u8) -> (w:u16)
fn main() {
    var w:u16 = data[0]
    var hi:u8
    var lo:u8
    hi::lo = w
    result[0] = lo::hi
    return
}
`

// endianSrc is an integration fixture combining shifts, AND, OR, concat and calls
// (a u64 byte-order switch), close to the bit-twiddling keccak performs.
const endianSrc = `pub input data(address:u1) -> (word:u64)
pub output result(address:u1) -> (word:u64)
fn main() {
    result[0] = switch_endian_u64(data[0])
}
fn switch_endian_u64(x:u64) -> (switched_x:u64) {
    var hi:u32 = (x >> 32) as u32
    var lo:u32 = (x & 0xFFFFFFFF) as u32
    var sw_hi:u32 = switch_endian_u32(hi)
    var sw_lo:u32 = switch_endian_u32(lo)
    switched_x = ((sw_lo as u64) << 32) | (sw_hi as u64)
    return
}
fn switch_endian_u32(x:u32) -> (switched_x:u32) {
    var hi:u16 = (x >> 16) as u16
    var lo:u16 = (x & 0xFFFF) as u16
    var sw_hi:u16 = switch_endian_u16(hi)
    var sw_lo:u16 = switch_endian_u16(lo)
    switched_x = ((sw_lo as u32) << 16) | (sw_hi as u32)
    return
}
fn switch_endian_u16(x:u16) -> (switched_x:u16) {
    var hi:u8 = (x >> 8) as u8
    var lo:u8 = (x & 0xFF) as u8
    switched_x = ((lo as u16) << 8) | (hi as u16)
    return
}
`

// compileU64 compiles a ZkC source string into a fresh, unlowered, vectorised
// u64 WordMachine (vectorisation is required for execution; LowerNatives is off
// to keep native integer ops — the codegen start point).  A fresh machine is
// required per reference execution because execution mutates memory state.
func compileU64(t testing.TB, src string) *vm.WordMachine[word.Uint64] {
	t.Helper()
	return compileU64Program(t, compileProgram(t, src))
}

func compileProgram(t testing.TB, src string) ast.Program {
	t.Helper()

	sf := source.NewSourceFile("gogen_test.zkc", []byte(src))

	program, _, errs := compiler.Compile(field.KOALABEAR_16, *sf)
	if len(errs) > 0 {
		t.Fatalf("compile: %v", errs)
	}

	return program
}

func compileU64Program(t testing.TB, program ast.Program) *vm.WordMachine[word.Uint64] {
	t.Helper()

	cfg := codegen.DEFAULT_CONFIG.Field(field.KOALABEAR_16).LowerNatives(false).Vectorize(true)

	wmUint, errs := program.Compile(cfg)
	if len(errs) > 0 {
		t.Fatalf("codegen: %v", errs)
	}

	return vm.WordToWordMachine[vm.Uint, word.Uint64](wmUint)
}

func TestGenTutorialStages(t *testing.T) {
	program := compileProgram(t, tt)
	wm := compileU64Program(t, program)

	goSrc, err := vm.WordToGoSource(wm)
	if err != nil {
		t.Fatalf("WordToGoSource: %v", err)
	}

	if _, err := format.Source([]byte(goSrc)); err != nil {
		t.Fatalf("generated source not valid Go: %v", err)
	}

	t.Logf("manual codegen stage dump for tutorialSrc\n\n%s\n\n%s\n\n%s",
		section("WIR", dumpWIR(program)),
		section("WORD MACHINE", dumpWordMachine(wm)),
		section("GENERATED GO", goSrc),
	)
}

func TestGenValidGo(t *testing.T) {
	srcs := map[string]string{
		"tutorial":    tutorialSrc,
		"destructure": destructSrc,
		"branch":      branchSrc,
		"loop":        loopSrc,
		"double":      doubleSrc,
		"call":        callSrc,
		"callFail":    callFailSrc,
		"recSum":      recSumSrc,
		"bitwise":     bitwiseSrc,
		"shift":       shiftSrc,
		"concat":      concatSrc,
		"endian":      endianSrc,
	}
	for name, src := range srcs {
		t.Run(name, func(t *testing.T) {
			out, err := vm.WordToGoSource(compileU64(t, src))
			if err != nil {
				t.Fatalf("WordToGoSource: %v", err)
			}

			if _, err := format.Source([]byte(out)); err != nil {
				t.Fatalf("generated source not valid Go: %v", err)
			}

			t.Logf("generated %d bytes of Go", len(out))
		})
	}
}

// TestGenDifferential compiles each generated program once and checks that, for
// a range of inputs, it produces outputs identical to the reference executor —
// and errors exactly when the reference errors.
func TestGenDifferential(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available")
	}

	cases := []struct {
		name    string
		src     string
		vectors []map[string][]uint64
	}{
		{
			name: "tutorial",
			src:  tutorialSrc,
			vectors: []map[string][]uint64{
				{"args": {5, 4, 3}},         // [9, 27, 1]
				{"args": {0, 0, 0}},         // [0, 0, 0]
				{"args": {7, 7, 2}},         // [14, 28, 0]
				{"args": {1, 0, 65535}},     // [1, 65535, 1]
				{"args": {60000, 60000, 1}}, // a+b overflow -> error
				{"args": {300, 300, 300}},   // (a+b)*c overflow -> error
				{"args": {3, 4, 1}},         // a-b underflow -> error
			},
		},
		{
			name: "destructure", // multi-register target (StoreAcross distribution)
			src:  destructSrc,
			vectors: []map[string][]uint64{
				{"args": {0x12345678}}, // hi=0x1234, lo=0x5678
				{"args": {0}},          // hi=0, lo=0
				{"args": {0xFFFFFFFF}}, // hi=0xFFFF, lo=0xFFFF
				{"args": {0x0000ABCD}}, // hi=0, lo=0xABCD
			},
		},
		{
			name: "branch", // Phase 2: if/else (SKIP_IF + SKIP)
			src:  branchSrc,
			vectors: []map[string][]uint64{
				{"data": {0}},   // x<=10 -> 1
				{"data": {10}},  // x<=10 -> 11
				{"data": {11}},  // x>10  -> 10
				{"data": {255}}, // x>10  -> 254
			},
		},
		{
			name: "loop", // Phase 2: JUMP-based loop, acc == n
			src:  loopSrc,
			vectors: []map[string][]uint64{
				{"data": {0}},
				{"data": {1}},
				{"data": {17}},
				{"data": {255}},
			},
		},
		{
			name: "double", // Phase 2: loop body overflows u8 once x>=8
			src:  doubleSrc,
			vectors: []map[string][]uint64{
				{"data": {0}}, // 1
				{"data": {3}}, // 8
				{"data": {7}}, // 128
				{"data": {8}}, // overflow -> error
				{"data": {9}}, // overflow -> error
			},
		},
		{
			name: "call", // Phase 3: simple value-returning call
			src:  callSrc,
			vectors: []map[string][]uint64{
				{"data": {0}},
				{"data": {41}},
				{"data": {254}},
				{"data": {255}}, // inc overflows u8 -> error
			},
		},
		{
			name: "callFail", // Phase 3: void call that may FAIL (error parity across frames)
			src:  callFailSrc,
			vectors: []map[string][]uint64{
				{"data": {0}}, // check fails -> error
				{"data": {7}},
				{"data": {255}},
			},
		},
		{
			name: "recSum", // Phase 3: recursion; large n overflows u16 -> error
			src:  recSumSrc,
			vectors: []map[string][]uint64{
				{"data": {0}},   // 0
				{"data": {1}},   // 1
				{"data": {5}},   // 15
				{"data": {255}}, // 32640
				{"data": {361}}, // 65341
				{"data": {362}}, // 65703 -> overflow u16 -> error
			},
		},
		{
			name: "bitwise", // Phase 7.1: AND/OR/XOR/NOT
			src:  bitwiseSrc,
			vectors: []map[string][]uint64{
				{"data": {0x0F, 0x3C}},
				{"data": {0xFF, 0x00}},
				{"data": {0xAA, 0x55}},
				{"data": {0x00, 0x00}},
			},
		},
		{
			name: "shift", // Phase 7.1: SHL (masked) / SHR, incl. amounts >= width
			src:  shiftSrc,
			vectors: []map[string][]uint64{
				{"data": {0x01, 3}},
				{"data": {0xFF, 1}},
				{"data": {0x80, 7}},
				{"data": {0x12, 8}},  // shift by width -> 0
				{"data": {0x12, 20}}, // shift beyond width -> 0
			},
		},
		{
			name: "concat", // Phase 7.1: BIT_CONCAT (byte-swap a u16)
			src:  concatSrc,
			vectors: []map[string][]uint64{
				{"data": {0x1234}},
				{"data": {0x00FF}},
				{"data": {0xFF00}},
				{"data": {0x0000}},
			},
		},
		{
			name: "endian", // Phase 7.1: shifts + AND + OR + concat + calls
			src:  endianSrc,
			vectors: []map[string][]uint64{
				{"data": {0x0123456789ABCDEF}},
				{"data": {0x0000000000000001}},
				{"data": {0xFFFFFFFFFFFFFFFF}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := vm.WordToGoSource(compileU64(t, tc.src))
			if err != nil {
				t.Fatalf("WordToGoSource: %v", err)
			}

			prog := buildProgram(t, goBin, src)
			for _, in := range tc.vectors {
				t.Run(inputName(in), func(t *testing.T) {
					refOut, refErr := referenceRun(t, compileU64(t, tc.src), in)

					genOut, genErr := runProgram(t, prog, in)
					if refErr != genErr {
						t.Fatalf("error mismatch: reference err=%v, generated err=%v (in=%v)", refErr, genErr, in)
					}

					if refErr {
						return
					}

					if !reflect.DeepEqual(refOut, genOut) {
						t.Fatalf("output mismatch (in=%v):\n  reference=%v\n  generated=%v", in, refOut, genOut)
					}
				})
			}
		})
	}
}

func section(name, body string) string {
	return fmt.Sprintf("==== %s ====\n%s", name, strings.TrimRight(body, "\n"))
}

func dumpWIR(program ast.Program) string {
	var b strings.Builder

	env := program.Environment()
	components := program.Components()

	fmt.Fprintf(&b, "components: %d\n", len(components))

	for i, component := range components {
		fmt.Fprintf(&b, "\n[%d] %s %q\n", i, wirDeclKind(component), component.Name())

		switch c := component.(type) {
		case *decl.ResolvedConstant:
			fmt.Fprintf(&b, "  type: %s\n", c.DataType.String(env))
			fmt.Fprintf(&b, "  value: %s\n", c.ConstExpr.String(nil))
		case *decl.ResolvedTypeAlias:
			fmt.Fprintf(&b, "  type: %s\n", c.DataType.String(env))
		case *decl.ResolvedMemory:
			fmt.Fprintf(&b, "  kind: %s\n", wirMemoryKind(c.Kind))
			fmt.Fprintf(&b, "  address: %s\n", wirVariables(c.Address, env))
			fmt.Fprintf(&b, "  data: %s\n", wirVariables(c.Data, env))
		case *decl.ResolvedFunction:
			fmt.Fprintf(&b, "  inputs: %s\n", wirVariables(c.Inputs(), env))
			fmt.Fprintf(&b, "  outputs: %s\n", wirVariables(c.Outputs(), env))

			if len(c.Effects) > 0 {
				fmt.Fprintf(&b, "  effects: %s\n", wirEffects(c))
			}

			fmt.Fprintf(&b, "  locals: %s\n", wirVariables(c.Variables[c.NumInputs+c.NumOutputs:], env))
			fmt.Fprintf(&b, "  code:\n")

			for pc, stmt := range c.Code {
				fmt.Fprintf(&b, "    %02d: %s\n", pc, stmt.String(c))
			}
		default:
			fmt.Fprintf(&b, "  detail: %T\n", c)
		}
	}

	return b.String()
}

func wirDeclKind(d decl.Resolved) string {
	switch d.(type) {
	case *decl.ResolvedConstant:
		return "constant"
	case *decl.ResolvedTypeAlias:
		return "type alias"
	case *decl.ResolvedMemory:
		return "memory"
	case *decl.ResolvedFunction:
		return "function"
	default:
		return fmt.Sprintf("%T", d)
	}
}

func wirMemoryKind(kind decl.MemoryKind) string {
	switch kind {
	case decl.PUBLIC_STATIC_MEMORY:
		return "public static"
	case decl.PRIVATE_STATIC_MEMORY:
		return "private static"
	case decl.PUBLIC_READ_ONLY_MEMORY:
		return "public read-only"
	case decl.PRIVATE_READ_ONLY_MEMORY:
		return "private read-only"
	case decl.PUBLIC_WRITE_ONCE_MEMORY:
		return "public write-once"
	case decl.PRIVATE_WRITE_ONCE_MEMORY:
		return "private write-once"
	case decl.RANDOM_ACCESS_MEMORY:
		return "random access"
	default:
		return fmt.Sprintf("memory kind %d", kind)
	}
}

func wirVariables(vars []variable.ResolvedDescriptor, env ast.Environment) string {
	if len(vars) == 0 {
		return "<none>"
	}

	parts := make([]string, len(vars))
	for i, v := range vars {
		parts[i] = fmt.Sprintf("%s:%s", v.Name, v.DataType.String(env))
	}

	return strings.Join(parts, ", ")
}

func wirEffects(fn *decl.ResolvedFunction) string {
	parts := make([]string, 0, len(fn.Effects))
	for _, effect := range fn.Effects {
		if effect == nil {
			parts = append(parts, "<nil>")
		} else {
			parts = append(parts, effect.String())
		}
	}

	return strings.Join(parts, ", ")
}

type memoryFlags interface {
	IsPublic() bool
	IsStatic() bool
	IsReadOnly() bool
	IsWriteOnly() bool
	IsReadWrite() bool
}

func dumpWordMachine(wm *vm.WordMachine[word.Uint64]) string {
	var b strings.Builder

	modules := wm.Modules()

	fmt.Fprintf(&b, "modules: %d\n", len(modules))

	for id, module := range modules {
		fmt.Fprintf(&b, "\n[%d] %s %q\n", id, machineModuleKind(module), module.Name())
		fmt.Fprintf(&b, "  native: %t\n", module.IsNative())

		if mem, ok := module.(memoryFlags); ok {
			fmt.Fprintf(&b, "  memory: public=%t static=%t readOnly=%t writeOnly=%t readWrite=%t\n",
				mem.IsPublic(), mem.IsStatic(), mem.IsReadOnly(), mem.IsWriteOnly(), mem.IsReadWrite())
		}

		fmt.Fprintf(&b, "  registers:\n")

		for rid, reg := range module.Registers() {
			fmt.Fprintf(&b, "    r%-2d %s\n", rid, reg.String())
		}

		if fn, ok := module.(*vm.WordFunction); ok {
			mapping := instruction.NewSystemMap(fn.RegisterMap(), modules)

			fmt.Fprintf(&b, "  code:\n")

			for pc, vec := range fn.Code() {
				fmt.Fprintf(&b, "    %02d: %s\n", pc, vec.String(mapping))
			}
		}
	}

	return b.String()
}

func machineModuleKind(module vm.Module) string {
	switch module.(type) {
	case *vm.WordFunction:
		return "function"
	case memoryFlags:
		return "memory"
	default:
		return fmt.Sprintf("%T", module)
	}
}

func inputName(in map[string][]uint64) string {
	b, err := json.Marshal(in)
	if err != nil {
		return fmt.Sprintf("%v", in)
	}

	return string(b)
}

// referenceRun executes the program on a fresh reference u64 machine, returning
// output memories as []uint64 and whether execution errored.
func referenceRun(t *testing.T, wm *vm.WordMachine[word.Uint64], in map[string][]uint64) (map[string][]uint64, bool) {
	t.Helper()

	inputs := make(map[string][]word.Uint64, len(in))
	for name, values := range in {
		inputs[name] = toWords(values)
	}

	if err := wm.Boot("main", inputs); err != nil {
		return nil, true
	}

	if _, err := vm.ExecuteAll(wm, 1<<20); err != nil {
		return nil, true
	}

	out := map[string][]uint64{}

	for it := wm.Outputs(); it.HasNext(); {
		m := it.Next()
		out[m.Name()] = fromWords(m.Contents())
	}

	return out, false
}

func buildProgram(t *testing.T, goBin, src string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module zkcgen\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prog := filepath.Join(dir, "prog")
	cmd := exec.Command(goBin, "build", "-o", prog, ".")

	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s\n--- source ---\n%s", err, out, src)
	}

	return prog
}

// runProgram runs the compiled program with JSON inputs on stdin, returning
// parsed outputs and whether it reported an execution error (exit 1).
func runProgram(t *testing.T, prog string, in map[string][]uint64) (map[string][]uint64, bool) {
	t.Helper()

	inJSON, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	cmd := exec.Command(prog)
	cmd.Stdin = bytes.NewReader(inJSON)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			switch ee.ExitCode() {
			case 1:
				return nil, true
			default:
				t.Fatalf("generated program failed (exit %d): %s", ee.ExitCode(), stderr.String())
			}
		}

		t.Fatalf("running generated program: %v", err)
	}

	var out map[string][]uint64
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decoding generated output %q: %v", stdout.String(), err)
	}

	return out, false
}

func toWords(vs []uint64) []word.Uint64 {
	out := make([]word.Uint64, len(vs))
	for i, v := range vs {
		out[i] = out[i].SetUint64(v)
	}

	return out
}

func fromWords(ws []word.Uint64) []uint64 {
	out := make([]uint64, len(ws))
	for i, w := range ws {
		out[i] = w.Uint64()
	}

	return out
}
