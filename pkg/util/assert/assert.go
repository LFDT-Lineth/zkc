// Copyright Consensys Incorporated
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
package assert

import (
	"math"
	"reflect"
	"testing"
)

// Equal errors if actual is not equal to expected.
func Equal(t *testing.T, expected, actual any, msg ...any) {
	if reflect.DeepEqual(expected, actual) || intEqual(expected, actual) {
		return
	}

	t.Errorf("expected: %v, actual: %v", expected, actual)

	if len(msg) != 0 {
		t.Errorf(msg[0].(string), msg[1:]...)
	}

	t.FailNow()
}

// intEqual returns whether expected and actual are both integers and whether they are equal
// if that is the case.
func intEqual(expected, actual any) bool {
	a, aInt64 := asInt64(expected)
	b, bInt64 := asInt64(actual)

	if aInt64 != bInt64 {
		return false
	}

	if aInt64 {
		return a == b
	}

	x, aUint64 := expected.(uint64)
	y, bUint64 := actual.(uint64)

	if !aUint64 || !bUint64 {
		return false
	}

	return x == y
}

// asInt64 tries to convert x to an int64 and specifies if the conversion was successful or
// if x only can be expressed as a uint64
func asInt64(x any) (int64, bool) {
	if y, ok := x.(uint64); ok && y > math.MaxInt64 {
		return 0, false
	}

	switch x := x.(type) {
	case int:
		return int64(x), true
	case int8:
		return int64(x), true
	case int16:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case uint:
		return int64(x), true
	case uint8:
		return int64(x), true
	case uint16:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		return int64(x), true
	}

	return 0, false
}

// True errors if condition is false.
func True(t *testing.T, condition bool, msg ...any) {
	if condition {
		return
	}

	t.Errorf("condition is false")

	if len(msg) != 0 {
		t.Errorf(msg[0].(string), msg[1:]...)
	}

	t.FailNow()
}

// False errors if condition is true.
func False(t *testing.T, condition bool, msg ...any) {
	if !condition {
		return
	}

	t.Errorf("condition is true")

	if len(msg) != 0 {
		t.Errorf(msg[0].(string), msg[1:]...)
	}

	t.FailNow()
}
