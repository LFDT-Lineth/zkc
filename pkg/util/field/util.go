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
package field

import (
	"slices"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// Pow takes a given value to the power n.
func Pow[F Element[F]](val F, n uint64) F {
	if n == 0 {
		val = val.SetUint64(1)
	} else if n > 1 {
		m := n / 2
		// Check for odd case
		if n%2 == 1 {
			tmp := val
			val = Pow(val, m)
			val = val.Mul(val).Mul(tmp)
		} else {
			// Even case is easy
			val = Pow(val, m)
			val = val.Mul(val)
		}
	}
	//
	return val
}

// SplitWord splits a BigEndian word into one or more limbs in a given field F,
// where each has a given width.  If the given value cannot be split into the
// given widths (i.e. because it overflows their combined width), false is
// returned to signal a splitting failure.
func SplitWord[F Element[F]](val word.BigEndian, widths []uint) ([]F, bool) {
	var (
		bitwidth = sum(widths...)
		// Determine bytewidth
		bytewidth = word.ByteWidth(bitwidth)
		// Extract bytes whilst ensuring they are in little endian form, and
		// that they match the expected bitwidth.
		bytes = padAndReverse(val.Bytes(), bytewidth)
		//
		bits     = bit.NewReader(bytes[:])
		elements = make([]F, len(widths))
		// FIXME: this should not be hardcoded
		buf [32]byte
	)
	// sanity check input value
	if val.Cmp(TwoPowN[word.BigEndian](bitwidth)) >= 0 {
		return nil, false
	}
	// read actual bits
	for i, w := range widths {
		// Read bits
		m := bits.LittleEndianReadInto(w, buf[:])
		// Convert back to big endian
		array.ReverseInPlace(buf[:m])
		// Done
		elements[i] = FromBigEndianBytes[F](buf[:m])
	}
	//
	return elements, true
}

func padAndReverse(bytes []byte, n uint) []byte {
	// Make sure bytes is both padded and cloned.
	switch {
	case n > uint(len(bytes)):
		bytes = array.FrontPad(bytes, n, 0)
	default:
		bytes = slices.Clone(bytes)
	}
	// In place reversal
	array.ReverseInPlace(bytes)
	//
	return bytes
}

func sum(vals ...uint) uint {
	val := uint(0)
	//
	for _, v := range vals {
		val += v
	}
	//
	return val
}
