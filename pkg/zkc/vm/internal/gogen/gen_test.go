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
// it drives the generator through the public vm.GenerateGo entry point, and
// vm imports gogen — so an internal test would form an import cycle.
package gogen_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/cmd/zkc/gogen"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
)

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

// carrySrc exercises the 128-bit pair path: a u64 + u64 sum destructured into a
// carry bit and a u64 — exact on the Uint machine (no accumulator trap), which
// is the RISC-V ADD shape.
const carrySrc = `pub input data(address:u8) -> (word:u64)
pub output result(address:u8) -> (word:u64)
fn main() {
    var a:u64 = data[0]
    var b:u64 = data[1]
    var c:u1
    var s:u64
    c::s = (a + b) as u65
    result[0] = s
    result[1] = c as u64
    return
}
`

// divModSrc exercises INT_DIV / INT_REM (plain shape) and, under the lowered
// shape, the HINT_DIVISION + validation sequence LowerDivisions produces.  A
// zero divisor must error in both shapes.
const divModSrc = `pub input data(address:u8) -> (byte:u8)
pub output result(address:u8) -> (byte:u8)
fn main() {
    var x:u8 = data[0]
    var y:u8 = data[1]
    result[0] = x / y
    result[1] = x % y
    return
}
`

// addwSrc is the RISC-V ADDW shape: a u32 + u32 sum widened to u33 and
// destructured into a carry bit and the low word.
const addwSrc = `pub input data(address:u8) -> (word:u32)
pub output result(address:u8) -> (word:u32)
fn main() {
    var a:u32 = data[0]
    var b:u32 = data[1]
    var c:u1
    var s:u32
    c::s = (a + b) as u33
    result[0] = s
    result[1] = c as u32
    return
}
`

// mulWideSrc exercises a 128-bit product of two u64s destructured into two
// u64 limbs (the widening-multiply shape).
const mulWideSrc = `pub input data(address:u8) -> (word:u64)
pub output result(address:u8) -> (word:u64)
fn main() {
    var a:u64 = data[0]
    var b:u64 = data[1]
    var hi:u64
    var lo:u64
    hi::lo = (a * b) as u128
    result[0] = lo
    result[1] = hi
    return
}
`

// wideRegSrc routes a value through an actual u65 REGISTER (not just a wide
// destructure): t = a + b at u65, then t - b (= a) destructures back into a
// dead carry bit and the original u64.
const wideRegSrc = `pub input data(address:u8) -> (word:u64)
pub output result(address:u8) -> (word:u64)
fn main() {
    var a:u64 = data[0]
    var b:u64 = data[1]
    var t:u65 = (a + b) as u65
    var z:u1
    var s:u64
    z::s = t - (b as u65)
    result[0] = s
    result[1] = z as u64
    return
}
`

// wideConstAddSrc mixes a bare constant into a u128 sum of casts: the emitted
// literal must be uint64-typed, or the `lo, hi := 1, uint64(0)` accumulator
// declaration types lo as int and the generated code does not compile
// (PR #1860 review finding).
const wideConstAddSrc = `pub input data(address:u8) -> (word:u64)
pub output result(address:u8) -> (word:u64)
fn add(x:u64, y:u64) -> (r:u64) {
    var tmp:u64
    tmp::r = (x as u128) + (y as u128) + 1
    return
}
fn main() {
    result[0] = add(data[0], data[1])
    return
}
`

// divMod64Src is u64 division: under the lowered shape this produces a
// division HINT with u128 quotient/remainder targets and a u128 validation
// multiply — the prover-shape pattern that needs both intervals and two-limb
// registers.
const divMod64Src = `pub input data(address:u8) -> (word:u64)
pub output result(address:u8) -> (word:u64)
fn main() {
    var x:u64 = data[0]
    var y:u64 = data[1]
    result[0] = x / y
    result[1] = x % y
    return
}
`

// pagedSrc exercises the paged scratch memory across page boundaries
// (PAGE_SIZE = 1M words): writes land in pages 0, 1 and 3, and reads hit both
// written cells and never-written cells in allocated (page 0) and unallocated
// (page 2) pages, which must read zero.
const pagedSrc = `pub input data(address:u8) -> (word:u64)
#[paged]
memory ram(address:u32) -> (word:u64)
pub output result(address:u8) -> (word:u64)
fn main<ram>() {
    ram[3] = data[0]
    ram[1048579] = data[1]
    ram[3145731] = data[2]
    result[0] = ram[3]
    result[1] = ram[1048579]
    result[2] = ram[3145731]
    result[3] = ram[2097155]
    result[4] = ram[7]
    return
}
`

// compileUint compiles a ZkC source string into a fresh, vectorised
// WordMachine over vm.Uint — the machine the generator consumes and the
// reference executor interprets.  `fastMode` selects the prover shape
// (FastMode off: bitwise/division/comparisons rewritten into helper calls
// and hints) versus the plain shape with native integer ops.  A fresh machine
// is required per reference execution because execution mutates memory state.
func compileUint(t testing.TB, src string, fastMode bool) vm.Program[vm.Uint] {
	t.Helper()
	return compileUintProgram(t, compileProgram(t, src), fastMode)
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

func compileUintProgram(t testing.TB, program ast.Program, fastMode bool) vm.Program[vm.Uint] {
	t.Helper()

	var (
		cfg = codegen.DEFAULT_CONFIG.
			Field(field.KOALABEAR_16).
			FastMode(fastMode).
			Quiet(true)
		// compile into bytecode program
		p, errs = ast.Compile(program, cfg)
	)
	//
	if len(errs) > 0 {
		t.Fatalf("codegen: %v", errs)
	}
	//
	return p
}

// shapes enumerates the two machine shapes every test runs against.
var shapes = []struct {
	name     string
	fastMode bool
}{
	{"fast", true},
	{"lowered", false},
}

func TestGenValidGo(t *testing.T) {
	srcs := map[string]string{
		"tutorial":     tutorialSrc,
		"destructure":  destructSrc,
		"branch":       branchSrc,
		"loop":         loopSrc,
		"double":       doubleSrc,
		"call":         callSrc,
		"callFail":     callFailSrc,
		"recSum":       recSumSrc,
		"bitwise":      bitwiseSrc,
		"shift":        shiftSrc,
		"concat":       concatSrc,
		"endian":       endianSrc,
		"carry":        carrySrc,
		"divmod":       divModSrc,
		"addw":         addwSrc,
		"mulWide":      mulWideSrc,
		"wideReg":      wideRegSrc,
		"wideConstAdd": wideConstAddSrc,
		"divmod64":     divMod64Src,
	}
	for name, src := range srcs {
		for _, shape := range shapes {
			t.Run(name+"/"+shape.name, func(t *testing.T) {
				out, err := vm.GenerateGo(compileUint(t, src, shape.fastMode), vm.GoGenConfig{})
				if err != nil {
					t.Fatalf("GenerateGo: %v", err)
				}

				if _, err := format.Source([]byte(out)); err != nil {
					t.Fatalf("generated source not valid Go: %v", err)
				}

				t.Logf("generated %d bytes of Go", len(out))
			})
		}
	}
}

// TestGenDifferential compiles each generated program once and checks that, for
// a range of inputs, it produces outputs identical to the reference executor —
// and errors exactly when the reference errors.  The corpus is shared with the
// fuzzer (fuzz_test.go).
type diffCase struct {
	name    string
	src     string
	vectors []map[string][]uint64
}

var diffCases = []diffCase{
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
	{
		name: "carry", // 128-bit pair path: u64+u64 destructured into c::s
		src:  carrySrc,
		vectors: []map[string][]uint64{
			{"data": {0, 0}},                                   // s=0, c=0
			{"data": {5, 7}},                                   // s=12, c=0
			{"data": {0xFFFFFFFFFFFFFFFF, 1}},                  // s=0, c=1
			{"data": {0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF}}, // s=2^64-2, c=1
			{"data": {0x8000000000000000, 0x8000000000000000}}, // s=0, c=1
		},
	},
	{
		name: "divmod", // INT_DIV/INT_REM (plain); HINT_DIVISION + validation (lowered)
		src:  divModSrc,
		vectors: []map[string][]uint64{
			{"data": {7, 3}},    // q=2, r=1
			{"data": {255, 16}}, // q=15, r=15
			{"data": {0, 9}},    // q=0, r=0
			{"data": {9, 1}},    // q=9, r=0
			{"data": {5, 0}},    // division by zero -> error
		},
	},
	{
		name: "addw", // RISC-V ADDW shape: u33 carry destructure
		src:  addwSrc,
		vectors: []map[string][]uint64{
			{"data": {0, 0}},
			{"data": {5, 7}},
			{"data": {0xFFFFFFFF, 1}},          // s=0, c=1
			{"data": {0xFFFFFFFF, 0xFFFFFFFF}}, // s=2^32-2, c=1
		},
	},
	{
		name: "mulWide", // u64×u64 → u128 destructure (widening multiply)
		src:  mulWideSrc,
		vectors: []map[string][]uint64{
			{"data": {0, 0}},
			{"data": {5, 7}},
			{"data": {0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF}}, // lo=1, hi=2^64-2
			{"data": {0x8000000000000000, 2}},                  // lo=0, hi=1
			{"data": {0x0123456789ABCDEF, 0xFEDCBA9876543210}},
		},
	},
	{
		name: "wideReg", // value through a real u65 register (add then sub)
		src:  wideRegSrc,
		vectors: []map[string][]uint64{
			{"data": {0, 0}},
			{"data": {5, 7}},
			{"data": {0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF}},
			{"data": {0xDEADBEEF, 0xCAFEBABE}},
		},
	},
	{
		name: "wideConstAdd", // bare constant inside a u128 sum (typed-literal regression)
		src:  wideConstAddSrc,
		vectors: []map[string][]uint64{
			{"data": {0, 0}},                                   // r=1
			{"data": {3, 4}},                                   // r=8
			{"data": {0xFFFFFFFFFFFFFFFF, 0}},                  // sum=2^64 -> r=0, tmp=1
			{"data": {0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF}}, // sum=2^65-1 -> r=2^64-1
		},
	},
	{
		name: "divmod64", // u64 division: u128 hint targets + validation (lowered)
		src:  divMod64Src,
		vectors: []map[string][]uint64{
			{"data": {7, 3}},
			{"data": {0xFFFFFFFFFFFFFFFF, 0xFFFFFFFF}},
			{"data": {0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF}},
			{"data": {0, 9}},
			{"data": {12345678901234567, 1}},
			{"data": {5, 0}}, // division by zero -> error
		},
	},
	{
		name: "paged", // paged scratch RAM: sparse pages, zero on unwritten reads
		src:  pagedSrc,
		vectors: []map[string][]uint64{
			{"data": {1, 2, 3}},
			{"data": {0xFFFFFFFFFFFFFFFF, 0, 0xDEADBEEF}},
		},
	},
	{
		name: "subWrap", // x - y - c: single-step two's-complement wrap on underflow
		src:  subWrapSrc,
		vectors: []map[string][]uint64{
			{"data": {10, 3}},    // 5
			{"data": {5, 10}},    // -7 -> 1017 (mod 2^10)
			{"data": {0, 255}},   // -257 -> 767
			{"data": {255, 0}},   // 253
			{"data": {0, 0}},     // -2 -> 1022
			{"data": {255, 255}}, // -2 -> 1022
		},
	},
}

// subWrapSrc compiles its subtraction into a single SUB with two register
// sources AND a constant subtrahend: the shape whose two-step (SUB_2n1 + SUBC)
// encoding used to wrap each step separately rather than once at the
// CalculateSubBitwidth width.  The result type is u10, so an underflow wraps
// modulo 2^10 and the wrapped value is observable through the widening cast.
const subWrapSrc = `pub input data(address:u8) -> (byte:u8)
pub output result(address:u8) -> (word:u16)
fn main() {
    var x:u8 = data[0]
    var y:u8 = data[1]
    result[0] = (x - y - 2) as u16
    return
}
`

// TestGenDifferential runs the shared corpus: see the comment on diffCase.
func TestGenDifferential(t *testing.T) {
	for _, tc := range diffCases {
		for _, shape := range shapes {
			t.Run(tc.name+"/"+shape.name, func(t *testing.T) {
				p := compileUint(t, tc.src, shape.fastMode)

				src, err := vm.GenerateGo(p, vm.GoGenConfig{})
				if err != nil {
					t.Fatalf("GenerateGo: %v", err)
				}

				prog := buildProgram(t, src)
				for _, in := range tc.vectors {
					t.Run(inputName(in), func(t *testing.T) {
						inBytes := encodeInputs(p, in)

						refOut, refErr := referenceRun(t, compileUint(t, tc.src, shape.fastMode), inBytes)

						genOut, genErr := runProgram(t, prog, inBytes)
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
}

// TestGenSubConstWrapWidth pins the wrap width of a single-source subtraction
// whose constant is a power of two WIDER than the source register — a shape
// the surface language cannot express (the type checker rejects a constant
// exceeding the operand type), so the program is assembled directly from
// bytecodes.  Per CalculateSubBitwidth, "x:u4 - 16" wraps at 1+max(4,
// BitLen(16-1)) = 5 bits, so 1 - 16 = -15 wraps to 17 (not 49, which the
// SUBC executor's former 1+max(4, BitLen(16)) = 6-bit width produced).  The
// program asserts the wrapped value in-machine via skip_if/fail, and the
// verdict is cross-checked against the generated Go.
func TestGenSubConstWrapWidth(t *testing.T) {
	var (
		zero, c16, c17 vm.Uint
		u4, u8         = util.Some(uint(4)), util.Some(uint(8))
		regs           = []vm.Register[vm.Uint]{
			vm.NewComputedRegister("x", u4, zero),
			vm.NewComputedRegister("t", u8, zero),
			vm.NewComputedRegister("e", u8, zero),
		}
		main = vm.NewBytecodeFunction("main", false, regs,
			vm.NewBytecodeVector[vm.Uint](vm.LoadConst(0, zero.SetUint64(1))),               // x = 1
			vm.NewBytecodeVector[vm.Uint](vm.Sub(1, []vm.RegisterId{0}, c16.SetUint64(16))), // t = x - 16
			vm.NewBytecodeVector[vm.Uint](vm.LoadConst(2, c17.SetUint64(17))),               // e = 17
			vm.NewBytecodeVector[vm.Uint]( // if t == e { ret } else { fail }
				vm.SkipIf[vm.Uint](bytecode.CONDITION_EQ, 1, 1, 2),
				vm.Fail[vm.Uint](nil, nil),
				vm.Return[vm.Uint]()),
		)
		p = vm.NewBytecodeProgram(field.KOALABEAR_16, main)
	)

	_, refErr := referenceRun(t, p, map[string][]byte{})

	src, err := vm.GenerateGo(p, vm.GoGenConfig{})
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}

	prog := buildProgram(t, src)

	_, genErr := runProgram(t, prog, map[string][]byte{})
	if refErr || genErr {
		t.Fatalf("subtraction wrapped at the wrong width: reference err=%v, generated err=%v", refErr, genErr)
	}
}

func TestMainHarnessRejectsBadInputNames(t *testing.T) {
	src, err := vm.GenerateGo(compileUint(t, doubleSrc, false), vm.GoGenConfig{})
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}

	prog := buildProgram(t, src)

	for _, tc := range []struct {
		name string
		in   map[string][]byte
		want string
	}{
		{"missing", map[string][]byte{}, `missing input "data"`},
		{"unknown", map[string][]byte{"data": {3}, "extra": {1}}, `unknown input "extra"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errored, err := gogen.Run(prog, tc.in, io.Discard)
			if err == nil {
				t.Fatal("expected harness error")
			}

			if errored {
				t.Fatal("bad input names should not be execution errors")
			}

			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func inputName(in map[string][]uint64) string {
	b, err := json.Marshal(in)
	if err != nil {
		return fmt.Sprintf("%v", in)
	}

	return string(b)
}

// referenceRun executes the program on a fresh reference Uint machine from the
// same packed-byte inputs the generated harness consumes, returning output
// memories as bytes and whether execution errored.  Sharing the bytes (rather
// than the raw cells) guarantees both executors see identical, width-valid
// inputs — see encodeInputs.
func referenceRun(t *testing.T, p vm.Program[vm.Uint], in map[string][]byte) (map[string][]byte, bool) {
	t.Helper()

	var (
		interpreter  = vm.NewBytecodeInterpreter(p)
		inputs, errs = vm.DecodeInputs(interpreter, in)
	)
	if len(errs) > 0 {
		t.Fatalf("decode inputs: %v", errs)
	}

	if err := interpreter.Boot("main", inputs); err != nil {
		return nil, true
	}

	if _, err := vm.ExecuteAll(interpreter, 1<<20); err != nil {
		return nil, true
	}

	return vm.EncodeOutputs(interpreter), false
}

// buildProgram compiles generated source into a test-owned temporary directory.
func buildProgram(t *testing.T, src string) string {
	t.Helper()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	prog, err := buildGeneratedProgram(t, src)
	if err != nil {
		t.Fatal(err)
	}

	return prog
}

func buildGeneratedProgram(t *testing.T, src string) (string, error) {
	t.Helper()

	dir := t.TempDir()
	prog := filepath.Join(dir, "prog")

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		return "", err
	}

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module zkcgen\n\ngo 1.24\n"), 0o644); err != nil {
		return "", err
	}

	cmd := exec.Command("go", "build", "-o", prog, ".")
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build failed: %v\n%s\n--- source ---\n%s", err, out, src)
	}

	return prog, nil
}

// runProgram runs the compiled program on packed-byte inputs (see encodeInputs),
// returning its output memories as bytes and whether it reported an execution
// error.
func runProgram(t *testing.T, prog string, in map[string][]byte) (map[string][]byte, bool) {
	t.Helper()

	out, errored, err := gogen.Run(prog, in, io.Discard)
	if err != nil {
		t.Fatalf("running generated program: %v", err)
	}

	return out, errored
}

// encodeInputs packs cell-valued inputs into the byte form the harness reads,
// using each input memory's geometry.  Each value is masked to its data-line
// width: the byte encoding carries exactly that many bits, so an out-of-width
// cell is not expressible — masking yields the canonical input both executors
// then consume identically.
func encodeInputs(p vm.Program[vm.Uint], in map[string][]uint64) map[string][]byte {
	out := map[string][]byte{}

	for it := p.Inputs(); it.HasNext(); {
		m := it.Next()
		regs := m.DataRegisters()
		cells := in[m.Name()]
		// rowWidth is the total width of one row's data field.  Under register
		// splitting a word wider than the field register (e.g. a u32 on a 16-bit
		// field) is spread across several limb registers, so DataRegisters holds
		// more than one entry per row and their widths sum to the logical width.
		var rowWidth uint
		for _, r := range regs {
			rowWidth += r.Bitwidth().Unwrap()
		}

		words := make([]vm.Uint, len(cells)*len(regs))
		k := 0
		// Split each logical value across its row's limb registers, most-
		// significant limb first (matching DataRegisters / the Base=MSB split
		// convention), so EncodeBytes/DecodeBytes round-trip the full value
		// rather than truncating it to the first limb.
		for _, v := range cells {
			hi := rowWidth

			for _, r := range regs {
				w := r.Bitwidth().Unwrap()
				hi -= w
				limb := v >> hi

				if w < 64 {
					limb &= (1 << w) - 1
				}

				words[k] = words[k].SetUint64(limb)
				k++
			}
		}

		out[m.Name()] = vm.EncodeBytes(words, m)
	}

	return out
}

// ---------------------------------------------------------------------------
// printf / fail messages (#1868): non-quiet builds emit printf to stderr and
// surface fail messages, byte-compatible with the reference interpreter.
// ---------------------------------------------------------------------------

const printfSrc = `pub input args(address:u16) -> (word:u16)
pub output result(address:u16) -> (word:u16)
fn main() {
    var a:u16 = args[0]
    printf "dec=%d hex=%x bin=%b pad=%04x\n", a, a, a, a
    result[0] = a
    return
}
`

// printfWideSrc prints a value wider than 64 bits, exercising the big.Int
// printf path (the lo/hi pair folds into a *big.Int via the u128 helper).
const printfWideSrc = `pub input args(address:u8) -> (w:u64)
pub output result(address:u8) -> (w:u64)
fn main() {
    var a:u64 = args[0]
    var b:u64 = args[1]
    var p:u128 = (a as u128) * (b as u128)
    printf "p=%d\n", p
    result[0] = a
    return
}
`

const printfCharSrc = `pub input args(address:u8) -> (byte:u8)
pub output result(address:u8) -> (byte:u8)
fn main() {
    var a:u8 = args[0]
    printf "%c%c!", a, a
    result[0] = a
    return
}
`

const failMsgSrc = `pub input args(address:u8) -> (byte:u8)
pub output result(address:u8) -> (byte:u8)
fn main() {
    var a:u8 = args[0]
    fail "boom a=%d", a
}
`

// compileUintVerbose compiles with printf retained (Quiet(false)), unlike the
// default helper which strips it.
func compileUintVerbose(t testing.TB, src string) vm.Program[vm.Uint] {
	t.Helper()

	var (
		cfg = codegen.DEFAULT_CONFIG.Field(field.KOALABEAR_16).Quiet(false)
		// Compile source file into an AST
		program = compileProgram(t, src)
		// Compile AST into a bytecode program
		p, errs = ast.Compile(program, cfg)
	)
	if len(errs) > 0 {
		t.Fatalf("codegen: %v", errs)
	}

	return p
}

// runStderr builds and runs a generated program, returning its stderr and exit
// status.  Unlike runProgram it captures stderr on success (where printf lands).
func runStderr(t *testing.T, p vm.Program[vm.Uint], in map[string][]uint64) (stderr string, exitErr bool) {
	t.Helper()

	src, err := vm.GenerateGo(p, vm.GoGenConfig{})
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}

	prog := buildProgram(t, src)

	inJSON, err := gogen.MarshalInputs(encodeInputs(p, in))
	if err != nil {
		t.Fatalf("marshal inputs: %v", err)
	}

	var out, errBuf bytes.Buffer

	cmd := exec.Command(prog)
	cmd.Stdin = bytes.NewReader(inJSON)
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return errBuf.String(), ee.ExitCode() != 0
		}

		t.Fatalf("running generated program: %v", err)
	}

	return errBuf.String(), false
}

func TestGenPrintf(t *testing.T) {
	cases := []struct {
		name, src string
		in        map[string][]uint64
		want      string
	}{
		{"verbs", printfSrc, map[string][]uint64{"args": {255}}, "dec=255 hex=ff bin=11111111 pad=00ff\n"},
		{"char", printfCharSrc, map[string][]uint64{"args": {65}}, "AA!"},
		// 2^33 * 2^33 = 2^66 = 73786976294838206464 (wider than u64).
		{"wide", printfWideSrc, map[string][]uint64{"args": {1 << 33, 1 << 33}}, "p=73786976294838206464\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := compileUintVerbose(t, tc.src)

			got, exitErr := runStderr(t, m, tc.in)
			if exitErr {
				t.Fatalf("unexpected non-zero exit; stderr=%q", got)
			}

			if got != tc.want {
				t.Fatalf("printf output mismatch:\n  got  %q\n  want %q", got, tc.want)
			}
		})
	}
}

func TestGenFailMessage(t *testing.T) {
	m := compileUintVerbose(t, failMsgSrc)

	got, exitErr := runStderr(t, m, map[string][]uint64{"args": {7}})
	if !exitErr {
		t.Fatalf("expected non-zero exit on fail")
	}

	if want := "machine panic: boom a=7"; !bytes.Contains([]byte(got), []byte(want)) {
		t.Fatalf("fail message missing: got %q, want substring %q", got, want)
	}
}
