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
package sexp

import (
	"reflect"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// ============================================================================
// Positive Tests
// ============================================================================

func TestSexp_0(t *testing.T) {
	CheckOk(t, nil, "")
}

func TestSexp_1(t *testing.T) {
	e1 := List{nil}
	CheckOk(t, &e1, "()")
}

func TestSexp_2(t *testing.T) {
	e1 := List{nil}
	e2 := List{[]SExp{&e1}}
	CheckOk(t, &e2, "(())")
}

func TestSexp_3(t *testing.T) {
	e1 := Set{nil}
	CheckOk(t, &e1, "{}")
}

func TestSexp_4(t *testing.T) {
	e1 := Set{nil}
	e2 := Set{[]SExp{&e1}}
	CheckOk(t, &e2, "{{}}")
}

func TestSexp_5(t *testing.T) {
	e1 := Set{nil}
	e2 := List{[]SExp{&e1}}
	CheckOk(t, &e2, "({})")
}
func TestSexp_6(t *testing.T) {
	e1 := List{nil}
	e2 := Set{[]SExp{&e1}}
	CheckOk(t, &e2, "{()}")
}

func TestSexp_7(t *testing.T) {
	e1 := Symbol{"symbol"}
	CheckOk(t, &e1, "symbol")
}

func TestSexp_8(t *testing.T) {
	e1 := Symbol{"12345"}
	CheckOk(t, &e1, "12345")
}

func TestSexp_9(t *testing.T) {
	e1 := Symbol{"+12345"}
	CheckOk(t, &e1, "+12345")
}

func TestSexp_10(t *testing.T) {
	e1 := Symbol{"symbol123"}
	e2 := List{[]SExp{&e1}}
	CheckOk(t, &e2, "(symbol123)")
}

func TestSexp_11(t *testing.T) {
	e1 := Symbol{"symbol"}
	e2 := List{[]SExp{&e1, &e1}}
	CheckOk(t, &e2, "(symbol symbol)")
}

func TestSexp_12(t *testing.T) {
	e1 := Symbol{"+"}
	e2 := Symbol{"1"}
	e3 := List{[]SExp{&e1, &e2}}
	CheckOk(t, &e3, "(+ 1)")
}

func TestSexp_13(t *testing.T) {
	e1 := Symbol{"hello"}
	e2 := Symbol{"world"}
	e3 := List{[]SExp{&e2}}
	e4 := List{[]SExp{&e1, &e3}}
	CheckOk(t, &e4, "(hello (world))")
}

func TestSexp_14(t *testing.T) {
	e1 := Symbol{"hello"}
	e2 := Symbol{"world"}
	e3 := List{[]SExp{&e2}}
	e4 := Set{[]SExp{&e1, &e3}}
	CheckOk(t, &e4, "{hello (world)}")
}

func TestSexp_15(t *testing.T) {
	e1 := Symbol{"hello"}
	e2 := Symbol{"world"}
	e3 := Set{[]SExp{&e2}}
	e4 := List{[]SExp{&e1, &e3}}
	CheckOk(t, &e4, "(hello {world})")
}

func TestSexp_16(t *testing.T) {
	e1 := Symbol{"hello"}
	e2 := Symbol{"world"}
	e3 := Set{[]SExp{&e2}}
	e4 := Set{[]SExp{&e1, &e3}}
	CheckOk(t, &e4, "{hello {world}}")
}

// ============================================================================
// Negative Tests
// ============================================================================

// unexpected end of list
func TestSexp_Err1(t *testing.T) {
	CheckErr(t, ")")
}

// unexpected end of list
func TestSexp_Err2(t *testing.T) {
	CheckErr(t, "())")
}

// unexpected end of list
func TestSexp_Err3(t *testing.T) {
	CheckErr(t, "(string))")
}

// unexpected end of list
func TestSexp_Err4(t *testing.T) {
	CheckErr(t, "(another string))")
}

// ============================================================================
// Helpers
// ============================================================================

func CheckOk(t *testing.T, sexp1 SExp, input string) {
	src := source.NewSourceFile("test", []byte(input))
	sexp2, _, err := Parse(src)
	//
	if err != nil {
		t.Error(err)
	} else if !reflect.DeepEqual(sexp1, sexp2) {
		t.Errorf("%s != %s", sexp1, sexp2)
	}
}

func CheckErr(t *testing.T, input string) {
	src := source.NewSourceFile("test", []byte(input))
	_, _, err := Parse(src)

	//
	if err == nil {
		t.Errorf("input should not have parsed!")
	}
}
