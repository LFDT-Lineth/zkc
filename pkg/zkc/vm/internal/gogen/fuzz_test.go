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
package gogen_test

import (
	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// TestGenFuzz drives the differential corpus with pseudo-random inputs (a
// deterministic seed), comparing the generated program against the reference
// executor on outputs AND failure verdicts — the safety net under the bound
// analysis (a missing width check shows up as a verdict divergence).  Values
// mix small integers, width boundaries and full-range words; out-of-width
// inputs are deliberate (ROM contents are untrusted) and exercise the reject
// paths.
//
// GOGEN_FUZZ_N sets the vectors per (fixture, shape); e.g. GOGEN_FUZZ_N=320
// yields ≥ 10^4 total runs (the acceptance bar).
func TestGenFuzz(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available")
	}

	n := 25

	if s := os.Getenv("GOGEN_FUZZ_N"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			n = v
		}
	}

	rng := rand.New(rand.NewSource(0xC0FFEE))

	for _, tc := range diffCases {
		for _, shape := range shapes {
			t.Run(tc.name+"/"+shape.name, func(t *testing.T) {
				src, err := vm.GenerateGo(compileUint(t, tc.src, shape.lowered), vm.GoGenConfig{})
				if err != nil {
					t.Fatalf("GenerateGo: %v", err)
				}

				prog := buildProgram(t, goBin, src)

				for i := 0; i < n; i++ {
					in := randomInputs(rng, tc.vectors[0])

					refOut, refErr := referenceRun(t, compileUint(t, tc.src, shape.lowered), in)

					genOut, genErr := runProgram(t, prog, in)
					if refErr != genErr {
						t.Fatalf("verdict mismatch: reference err=%v, generated err=%v (in=%v)", refErr, genErr, in)
					}

					if !refErr && !reflect.DeepEqual(refOut, genOut) {
						t.Fatalf("output mismatch (in=%v):\n  reference=%v\n  generated=%v", in, refOut, genOut)
					}
				}
			})
		}
	}
}

// randomInputs builds inputs with the same memories and lengths as a sample
// vector, with values drawn from small / boundary / full-range classes.
func randomInputs(rng *rand.Rand, sample map[string][]uint64) map[string][]uint64 {
	boundaries := []uint64{
		0, 1, 2,
		1<<8 - 1, 1 << 8,
		1<<16 - 1, 1 << 16,
		1<<32 - 1, 1 << 32,
		1<<63 - 1, 1 << 63,
		^uint64(0) - 1, ^uint64(0),
	}

	in := make(map[string][]uint64, len(sample))

	for name, words := range sample {
		vs := make([]uint64, len(words))
		for i := range vs {
			switch rng.Intn(3) {
			case 0:
				vs[i] = uint64(rng.Intn(17))
			case 1:
				vs[i] = boundaries[rng.Intn(len(boundaries))]
			default:
				vs[i] = rng.Uint64()
			}
		}

		in[name] = vs
	}

	return in
}
