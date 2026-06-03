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
package data

import (
	"fmt"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// DecodeAll decodes the given set of bytes as big integer values according to
// the given data type(s).  Observe that values are assumed to be packed tightly
// (i.e. without any padding).  Consider the following input byte array:
//
// |  00  |  01  |  02  |  03  |
// +------+------+------+------+
// | 0x31 | 0xf0 | 0x0e | 0x1d |
//
// Then, decoding this into a u4 array will produce the following:
//
// |  0  |  1  |  2  |  3  |  4  |  5  |  6  |  7  |
// +-----+-----+-----+-----+-----+-----+-----+-----+
// | 0x3 | 0x1 | 0xf | 0x0 | 0x0 | 0xe | 0x1 | 0xd |
//
// If the input array is not a multiple of the bitwidth
func DecodeAll[S symbol.Symbol[S]](datatype Type[S], bytes []byte, env Environment[S]) []vm.Uint {
	var (
		bitwidth, _ = BitWidthOf(datatype, env)
		// Initially empty buffer which is expanded as necessary to accommodate
		// reading bits of the given data types.
		buffer []byte
	)
	// Decode array into
	values, _ := bit.DecodeArray(bitwidth, bytes, func(bytes []byte) (ints []big.Int) {
		var reader = bit.NewReader(bytes)
		// Decode the type using the given buffer
		ints, buffer = decodeType(datatype, &reader, buffer, env)
		// Done
		return ints
	})
	// Flatten decoded tuples
	return array.FlatMap(values, func(ints []big.Int) []vm.Uint {
		var words = make([]vm.Uint, len(ints))
		//
		for i, v := range ints {
			var ith vm.Uint
			//
			words[i] = ith.SetBigInt(&v)
		}
		//
		return words
	})
}

func decodeType[S symbol.Symbol[S]](datatype Type[S], reader *bit.Reader, buffer []byte,
	env Environment[S]) ([]big.Int, []byte) {
	//
	switch dt := datatype.(type) {
	case *Alias[S]:
		return decodeType(dt.Resolve(env), reader, buffer, env)
	case *FieldElement[S]:
		panic(fmt.Sprintf("field element type cannot be decoded from bytes: %s", datatype.String(env)))
	case *UnsignedInt[S]:
		return vm.DecodeUnsignedInt(dt.bitwidth, reader, buffer)
	case *Tuple[S]:
		return decodeTuple(dt.elements, reader, buffer, env)
	default:
		panic(fmt.Sprintf("unknown type \"%s\"", datatype.String(env)))
	}
}

func decodeTuple[S symbol.Symbol[S]](types []Type[S], reader *bit.Reader, buffer []byte,
	env Environment[S]) ([]big.Int, []byte) {
	//
	var (
		vals []big.Int
	)
	//
	for _, t := range types {
		var vs []big.Int
		//
		vs, buffer = decodeType(t, reader, buffer, env)
		//
		vals = append(vals, vs...)
	}
	//
	return vals, buffer
}
