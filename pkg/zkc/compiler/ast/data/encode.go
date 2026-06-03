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

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/ast/symbol"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// EncodeAll encodes the given set of word values as packed bytes according to
// the given data type(s).  This is the inverse of DecodeAll.  Consider the
// following input array of u4 values:
//
// |  0  |  1  |  2  |  3  |  4  |  5  |  6  |  7  |
// +-----+-----+-----+-----+-----+-----+-----+-----+
// | 0x3 | 0x1 | 0xf | 0x0 | 0x0 | 0xe | 0x1 | 0xd |
//
// Then, encoding this as a u4 array will produce the following bytes:
//
// |  00  |  01  |  02  |  03  |
// +------+------+------+------+
// | 0x31 | 0xf0 | 0x0e | 0x1d |
func EncodeAll[S symbol.Symbol[S]](datatype Type[S], values []vm.Uint, env Environment[S]) []byte {
	var (
		bitwidth, _ = BitWidthOf(datatype, env)
		nElems      = uint(len(values))
		totalBits   = nElems * bitwidth
		totalBytes  = (totalBits + 7) / 8
		result      = make([]byte, totalBytes)
		n           = bit.BytesRequiredFor(bitwidth)
		buf         = make([]byte, n)
	)

	for i, v := range values {
		encodeType(datatype, bitwidth, v, buf, env)
		bit.BigEndianCopy(buf, 0, result, uint(i)*bitwidth, bitwidth)
	}

	return result
}

func encodeType[S symbol.Symbol[S]](datatype Type[S], bitwidth uint, v vm.Uint, buf []byte, env Environment[S]) {
	switch datatype.(type) {
	case *UnsignedInt[S], *Alias[S]:
		vm.EncodeUnsignedInt(bitwidth, v.BigInt(), buf)
	case *FieldElement[S]:
		panic(fmt.Sprintf("field element type cannot be encoded to bytes: %s", datatype.String(env)))
	default:
		panic(fmt.Sprintf("unknown type \"%s\"", datatype.String(env)))
	}
}
