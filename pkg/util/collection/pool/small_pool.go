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
package pool

import (
	"fmt"
	"reflect"

	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// SmallPool is a pool implementation for handling small domains (i.e. where all
// elements can be enumerated).
type SmallPool[K uint8 | uint16, W word.Word[W]] struct {
	index []W
}

// NewBytePool constructs a new pool which uses a given key type for
// indexing.
func NewBytePool[W word.Word[W]]() SmallPool[uint8, W] {
	index := getIndex[W]()
	//
	return SmallPool[uint8, W]{index}
}

// NewWordPool constructs a new pool which uses a given key type for
// indexing.
func NewWordPool[W word.Word[W]]() SmallPool[uint16, W] {
	index := getIndex[W]()
	//
	return SmallPool[uint16, W]{index}
}

// Get implementation for Pool interface.
func (p SmallPool[K, F]) Get(index K) F {
	return p.index[index]
}

// Put implementation for Pool interface.
func (p SmallPool[K, F]) Put(element F) K {
	return K(element.Uint64())
}

// Clone implementation for Pool interface.
func (p SmallPool[K, F]) Clone() Pool[K, F] {
	return p
}

// ============================================================================
// Custom implementations
// ============================================================================

// Index for the GF251 field
var gf251_index []gf251.Element

// Index for the GF8209 field
var gf8209_index []gf8209.Element

// Index for the koalabear field
var koalabear_index []koalabear.Element

// Index for the bls12_377 field
var bls12_377_index []bls12_377.Element

// Index for the bigendian word
var bigendian_index []word.BigEndian

func getIndex[W word.Word[W]]() []W {
	var (
		dummy W
		index []W
	)
	//
	switch any(dummy).(type) {
	case gf251.Element:
		index = any(gf251_index).([]W)
	case gf8209.Element:
		index = any(gf8209_index).([]W)
	case koalabear.Element:
		index = any(koalabear_index).([]W)
	case bls12_377.Element:
		index = any(bls12_377_index).([]W)
	case word.BigEndian:
		index = any(bigendian_index).([]W)
	default:
		panic(fmt.Sprintf("small pool not supported for %s", reflect.TypeOf(dummy).String()))
	}
	//
	return index
}

func initIndex[W word.Word[W]]() []W {
	var index = make([]W, 65536)
	//
	for i := range uint64(65536) {
		var ith W
		//
		index[i] = ith.SetUint64(i)
	}
	//
	return index
}

func init() {
	gf251_index = initIndex[gf251.Element]()
	gf8209_index = initIndex[gf8209.Element]()
	koalabear_index = initIndex[koalabear.Element]()
	bls12_377_index = initIndex[bls12_377.Element]()
	bigendian_index = initIndex[word.BigEndian]()
}
