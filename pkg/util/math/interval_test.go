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
package math

import (
	"math/big"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util"
)

const ADD = 0
const SUB = 1
const MUL = 2

func Test_Interval_01(t *testing.T) {
	checkInterval(t, []uint{}, [][]int{})
}
func Test_Interval_02a(t *testing.T) {
	checkInterval(t, []uint{ADD}, [][]int{{1, 2, 3}})
}
func Test_Interval_02b(t *testing.T) {
	checkInterval(t, []uint{ADD}, [][]int{{-1, 2, 3}})
}
func Test_Interval_02c(t *testing.T) {
	checkInterval(t, []uint{ADD}, [][]int{{-1, -2, -3}})
}
func Test_Interval_03a(t *testing.T) {
	checkInterval(t, []uint{SUB}, [][]int{{1, 2, 3}})
}
func Test_Interval_03b(t *testing.T) {
	checkInterval(t, []uint{SUB}, [][]int{{-1, 2, 3}})
}
func Test_Interval_03c(t *testing.T) {
	checkInterval(t, []uint{SUB}, [][]int{{-1, -2, -3}})
}
func Test_Interval_04a(t *testing.T) {
	checkInterval(t, []uint{MUL}, [][]int{{1, 2, 3}})
}
func Test_Interval_04b(t *testing.T) {
	checkInterval(t, []uint{MUL}, [][]int{{-1, 2, 3}})
}
func Test_Interval_04c(t *testing.T) {
	checkInterval(t, []uint{MUL}, [][]int{{-1, -2, -3}})
}

func Test_Interval_05a(t *testing.T) {
	checkInterval(t, []uint{ADD, ADD}, [][]int{{1, 2, 3}, {4, 5}})
}
func Test_Interval_05b(t *testing.T) {
	checkInterval(t, []uint{ADD, ADD}, [][]int{{-1, 2, 3}, {4, 5}})
}
func Test_Interval_05c(t *testing.T) {
	checkInterval(t, []uint{ADD, ADD}, [][]int{{1, 2, 3}, {-4, 5}})
}
func Test_Interval_05d(t *testing.T) {
	checkInterval(t, []uint{ADD, ADD}, [][]int{{-1, 2, 3}, {-4, 5}})
}

func Test_Interval_05e(t *testing.T) {
	checkInterval(t, []uint{ADD, ADD}, [][]int{{-1, -2, -3}, {4, 5}})
}

func Test_Interval_05f(t *testing.T) {
	checkInterval(t, []uint{ADD, ADD}, [][]int{{-1, -2, -3}, {-4, 5}})
}

func Test_Interval_05g(t *testing.T) {
	checkInterval(t, []uint{ADD, ADD}, [][]int{{-1, -2, -3}, {-4, -5}})
}

func Test_Interval_06a(t *testing.T) {
	checkInterval(t, []uint{ADD, SUB}, [][]int{{1, 2, 3}, {4, 5}})
}
func Test_Interval_06b(t *testing.T) {
	checkInterval(t, []uint{ADD, SUB}, [][]int{{-1, 2, 3}, {4, 5}})
}
func Test_Interval_06c(t *testing.T) {
	checkInterval(t, []uint{ADD, SUB}, [][]int{{1, 2, 3}, {-4, 5}})
}
func Test_Interval_06d(t *testing.T) {
	checkInterval(t, []uint{ADD, SUB}, [][]int{{-1, 2, 3}, {-4, 5}})
}

func Test_Interval_06e(t *testing.T) {
	checkInterval(t, []uint{ADD, SUB}, [][]int{{-1, -2, -3}, {4, 5}})
}

func Test_Interval_06f(t *testing.T) {
	checkInterval(t, []uint{ADD, SUB}, [][]int{{-1, -2, -3}, {-4, 5}})
}

func Test_Interval_06g(t *testing.T) {
	checkInterval(t, []uint{ADD, SUB}, [][]int{{-1, -2, -3}, {-4, -5}})
}

func Test_Interval_07a(t *testing.T) {
	checkInterval(t, []uint{ADD, MUL}, [][]int{{1, 2, 3}, {4, 5}})
}
func Test_Interval_07b(t *testing.T) {
	checkInterval(t, []uint{ADD, MUL}, [][]int{{-1, 2, 3}, {4, 5}})
}
func Test_Interval_07c(t *testing.T) {
	checkInterval(t, []uint{ADD, MUL}, [][]int{{1, 2, 3}, {-4, 5}})
}
func Test_Interval_07d(t *testing.T) {
	checkInterval(t, []uint{ADD, MUL}, [][]int{{-1, 2, 3}, {-4, 5}})
}

func Test_Interval_07e(t *testing.T) {
	checkInterval(t, []uint{ADD, MUL}, [][]int{{-1, -2, -3}, {4, 5}})
}

func Test_Interval_07f(t *testing.T) {
	checkInterval(t, []uint{ADD, MUL}, [][]int{{-1, -2, -3}, {-4, 5}})
}

func Test_Interval_07g(t *testing.T) {
	checkInterval(t, []uint{ADD, MUL}, [][]int{{-1, -2, -3}, {-4, -5}})
}

func Test_Interval_15a(t *testing.T) {
	checkInterval(t, []uint{SUB, ADD}, [][]int{{1, 2, 3}, {4, 5}})
}
func Test_Interval_15b(t *testing.T) {
	checkInterval(t, []uint{SUB, ADD}, [][]int{{-1, 2, 3}, {4, 5}})
}
func Test_Interval_15c(t *testing.T) {
	checkInterval(t, []uint{SUB, ADD}, [][]int{{1, 2, 3}, {-4, 5}})
}
func Test_Interval_15d(t *testing.T) {
	checkInterval(t, []uint{SUB, ADD}, [][]int{{-1, 2, 3}, {-4, 5}})
}

func Test_Interval_15e(t *testing.T) {
	checkInterval(t, []uint{SUB, ADD}, [][]int{{-1, -2, -3}, {4, 5}})
}

func Test_Interval_15f(t *testing.T) {
	checkInterval(t, []uint{SUB, ADD}, [][]int{{-1, -2, -3}, {-4, 5}})
}

func Test_Interval_15g(t *testing.T) {
	checkInterval(t, []uint{SUB, ADD}, [][]int{{-1, -2, -3}, {-4, -5}})
}

func Test_Interval_16a(t *testing.T) {
	checkInterval(t, []uint{SUB, SUB}, [][]int{{1, 2, 3}, {4, 5}})
}
func Test_Interval_16b(t *testing.T) {
	checkInterval(t, []uint{SUB, SUB}, [][]int{{-1, 2, 3}, {4, 5}})
}
func Test_Interval_16c(t *testing.T) {
	checkInterval(t, []uint{SUB, SUB}, [][]int{{1, 2, 3}, {-4, 5}})
}
func Test_Interval_16d(t *testing.T) {
	checkInterval(t, []uint{SUB, SUB}, [][]int{{-1, 2, 3}, {-4, 5}})
}

func Test_Interval_16e(t *testing.T) {
	checkInterval(t, []uint{SUB, SUB}, [][]int{{-1, -2, -3}, {4, 5}})
}

func Test_Interval_16f(t *testing.T) {
	checkInterval(t, []uint{SUB, SUB}, [][]int{{-1, -2, -3}, {-4, 5}})
}

func Test_Interval_16g(t *testing.T) {
	checkInterval(t, []uint{SUB, SUB}, [][]int{{-1, -2, -3}, {-4, -5}})
}

func Test_Interval_17a(t *testing.T) {
	checkInterval(t, []uint{SUB, MUL}, [][]int{{1, 2, 3}, {4, 5}})
}
func Test_Interval_17b(t *testing.T) {
	checkInterval(t, []uint{SUB, MUL}, [][]int{{-1, 2, 3}, {4, 5}})
}
func Test_Interval_17c(t *testing.T) {
	checkInterval(t, []uint{SUB, MUL}, [][]int{{1, 2, 3}, {-4, 5}})
}
func Test_Interval_17d(t *testing.T) {
	checkInterval(t, []uint{SUB, MUL}, [][]int{{-1, 2, 3}, {-4, 5}})
}

func Test_Interval_17e(t *testing.T) {
	checkInterval(t, []uint{SUB, MUL}, [][]int{{-1, -2, -3}, {4, 5}})
}

func Test_Interval_17f(t *testing.T) {
	checkInterval(t, []uint{SUB, MUL}, [][]int{{-1, -2, -3}, {-4, 5}})
}

func Test_Interval_17g(t *testing.T) {
	checkInterval(t, []uint{SUB, MUL}, [][]int{{-1, -2, -3}, {-4, -5}})
}

func Test_Interval_25a(t *testing.T) {
	checkInterval(t, []uint{MUL, ADD}, [][]int{{1, 2, 3}, {4, 5}})
}
func Test_Interval_25b(t *testing.T) {
	checkInterval(t, []uint{MUL, ADD}, [][]int{{-1, 2, 3}, {4, 5}})
}
func Test_Interval_25c(t *testing.T) {
	checkInterval(t, []uint{MUL, ADD}, [][]int{{1, 2, 3}, {-4, 5}})
}
func Test_Interval_25d(t *testing.T) {
	checkInterval(t, []uint{MUL, ADD}, [][]int{{-1, 2, 3}, {-4, 5}})
}

func Test_Interval_25e(t *testing.T) {
	checkInterval(t, []uint{MUL, ADD}, [][]int{{-1, -2, -3}, {4, 5}})
}

func Test_Interval_25f(t *testing.T) {
	checkInterval(t, []uint{MUL, ADD}, [][]int{{-1, -2, -3}, {-4, 5}})
}

func Test_Interval_25g(t *testing.T) {
	checkInterval(t, []uint{MUL, ADD}, [][]int{{-1, -2, -3}, {-4, -5}})
}

func Test_Interval_26a(t *testing.T) {
	checkInterval(t, []uint{MUL, SUB}, [][]int{{1, 2, 3}, {4, 5}})
}
func Test_Interval_26b(t *testing.T) {
	checkInterval(t, []uint{MUL, SUB}, [][]int{{-1, 2, 3}, {4, 5}})
}
func Test_Interval_26c(t *testing.T) {
	checkInterval(t, []uint{MUL, SUB}, [][]int{{1, 2, 3}, {-4, 5}})
}
func Test_Interval_26d(t *testing.T) {
	checkInterval(t, []uint{MUL, SUB}, [][]int{{-1, 2, 3}, {-4, 5}})
}

func Test_Interval_26e(t *testing.T) {
	checkInterval(t, []uint{MUL, SUB}, [][]int{{-1, -2, -3}, {4, 5}})
}

func Test_Interval_26f(t *testing.T) {
	checkInterval(t, []uint{MUL, SUB}, [][]int{{-1, -2, -3}, {-4, 5}})
}

func Test_Interval_26g(t *testing.T) {
	checkInterval(t, []uint{MUL, SUB}, [][]int{{-1, -2, -3}, {-4, -5}})
}

func Test_Interval_27a(t *testing.T) {
	checkInterval(t, []uint{MUL, MUL}, [][]int{{1, 2, 3}, {4, 5}})
}
func Test_Interval_27b(t *testing.T) {
	checkInterval(t, []uint{MUL, MUL}, [][]int{{-1, 2, 3}, {4, 5}})
}
func Test_Interval_27c(t *testing.T) {
	checkInterval(t, []uint{MUL, MUL}, [][]int{{1, 2, 3}, {-4, 5}})
}
func Test_Interval_27d(t *testing.T) {
	checkInterval(t, []uint{MUL, MUL}, [][]int{{-1, 2, 3}, {-4, 5}})
}

func Test_Interval_27e(t *testing.T) {
	checkInterval(t, []uint{MUL, MUL}, [][]int{{-1, -2, -3}, {4, 5}})
}

func Test_Interval_27f(t *testing.T) {
	checkInterval(t, []uint{MUL, MUL}, [][]int{{-1, -2, -3}, {-4, 5}})
}

func Test_Interval_27g(t *testing.T) {
	checkInterval(t, []uint{MUL, MUL}, [][]int{{-1, -2, -3}, {-4, -5}})
}

// Random

func Test_Interval_30(t *testing.T) {
	checkRandomWalk(t, 3, 10)
}

func Test_Interval_31(t *testing.T) {
	checkRandomWalk(t, 4, 20)
}

func Test_Interval_32(t *testing.T) {
	checkRandomWalk(t, 5, 100)
}

func Test_Interval_33(t *testing.T) {
	checkRandomWalk(t, 6, 10)
}

func checkRandomWalk(t *testing.T, n uint, m int) {
	ops := util.GenerateRandomUints(n, 3)
	sets := make([][]int, n)
	// Fill out steps
	for i := uint(0); i < n; i++ {
		sets[i] = util.GenerateRandomInts(n, m)
	}
	// Check the operations
	checkInterval(t, ops, sets)
}

func checkInterval(t *testing.T, ops []uint, sets [][]int) {
	var (
		r Interval
		s = []int{0}
	)

	for i, set := range sets {
		ith := toInterval(set)
		//
		switch ops[i] {
		case ADD:
			r.Add(ith)
			//
			s = add(s, set)
		case SUB:
			r.Sub(ith)
			//
			s = sub(s, set)
		case MUL:
			r.Mul(ith)
			//
			s = mul(s, set)
		default:
			panic("unknown operation")
		}
	}
	// final check
	checkSubSet(t, &r, s)
}

func checkSubSet(t *testing.T, i *Interval, set []int) {
	for _, item := range set {
		ith := big.NewInt(int64(item))
		if !i.Contains(*ith) {
			t.Errorf("value %d not contained in %s", item, i.String())
		}
	}
}

func add(lhs []int, rhs []int) []int {
	res := make([]int, 0)
	//
	for i := range lhs {
		for j := range rhs {
			res = append(res, lhs[i]+rhs[j])
		}
	}
	//
	return res
}

func sub(lhs []int, rhs []int) []int {
	res := make([]int, 0)
	//
	for i := range lhs {
		for j := range rhs {
			res = append(res, lhs[i]-rhs[j])
		}
	}
	//
	return res
}

func mul(lhs []int, rhs []int) []int {
	res := make([]int, 0)
	//
	for i := range lhs {
		for j := range rhs {
			res = append(res, lhs[i]*rhs[j])
		}
	}
	//
	return res
}

func toInterval(items []int) Interval {
	var r Interval
	//
	for i, item := range items {
		iv := big.NewInt(int64(item))
		ith := NewInterval(*iv, *iv)
		//
		if i == 0 {
			r.Set(ith)
		} else {
			r.Insert(ith)
		}
	}
	//
	return r
}
