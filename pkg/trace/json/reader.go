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
package json

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// FromBytes parses a sharded trace expressed in JSON notation.  For example,
// {"X": [0], "Y": [1]} is a trace containing one row of data each for two
// columns "X" and "Y".
func FromBytes[F field.Element[F]](data []byte) (tr trace.Trace[F], err error) {
	var (
		shard           trace.Shard[F]
		unsharded       map[string]map[string][]big.Int
		sharded         []map[string]map[string][]big.Int
		legacyUnsharded map[string][]big.Int
		legacySharded   []map[string][]big.Int
	)
	// Attempt to unmarshall
	if err = json.Unmarshal(data, &sharded); err == nil {
		for _, data := range sharded {
			if shard, err = fromBytesInternal[F](data); err != nil {
				return nil, err
			}
			//
			tr = append(tr, shard)
		}
	} else if err = json.Unmarshal(data, &legacySharded); err == nil {
		for _, data := range legacySharded {
			if shard, err = fromBytesLegacy[F](data); err != nil {
				return nil, err
			}
			//
			tr = append(tr, shard)
		}
	} else if err = json.Unmarshal(data, &unsharded); err == nil {
		shard, err = fromBytesInternal[F](unsharded)
		tr = trace.Trace[F]{shard}
	} else if err = json.Unmarshal(data, &legacyUnsharded); err == nil {
		shard, err = fromBytesLegacy[F](legacyUnsharded)
		tr = trace.Trace[F]{shard}
	}
	//
	return tr, err
}

// FromBytesLegacy parses a trace expressed in JSON notation.  For example, {"X":
// [0], "Y": [1]} is a trace containing one row of data each for two columns "X"
// and "Y".
func fromBytesLegacy[F field.Element[F]](rawData map[string][]big.Int) (trace.Shard[F], error) {
	var (
		strData = make(map[string]map[string][]big.Int, 0)
	)
	//
	for name, rawInts := range rawData {
		// Translate raw bigints into raw field elements
		mod, col, error := splitQualifiedColumnName(name)
		// error check
		if error != nil {
			return nil, error
		}
		// Sanity check existing module data
		if strData[mod] == nil {
			strData[mod] = make(map[string][]big.Int)
		} else if _, ok := strData[mod][col]; ok {
			return nil, fmt.Errorf("duplicate column %s encountered", trace.QualifiedColumnName(mod, col))
		}
		// Assign values
		strData[mod][col] = rawInts
	}
	// Done.
	return fromBytesInternal[F](strData)
}

func fromBytesInternal[F field.Element[F]](rawData map[string]map[string][]big.Int) (trace.Shard[F], error) {
	var modules []*trace.CompactModule[F]
	//
	for mod, modData := range rawData {
		var (
			columns     []array.MutArray[F]
			descriptors []trace.ColumnDescriptor
		)
		//
		for name, rawInts := range modData {
			col, bitwidth, error := splitColumnBitwidth(name)
			// error check
			if error != nil {
				return nil, error
			}
			// Validate data array
			if row := validateBigInts(bitwidth, rawInts); row != math.MaxUint {
				return nil, fmt.Errorf("column %s out-of-bounds (row %d, value %s)",
					name, row, rawInts[row].String())
			}
			// Construct column
			columns = append(columns, newArrayFromBigInts[F](bitwidth, rawInts))
			descriptors = append(descriptors, trace.NewColumnDescriptor(col, bitwidth))
		}
		// construct module descriptor
		descriptor := trace.NewModuleDescriptor(mod, descriptors)
		// append new module
		modules = append(modules, trace.NewCompactModule[F](descriptor, columns...))
	}
	//
	return trace.NewArray(modules), nil
}

func newArrayFromBigInts[F field.Element[F]](bitwidth util.Option[uint], data []big.Int) array.MutArray[F] {
	//
	var (
		n   = uint(len(data))
		arr = array.Alloc[F](bitwidth.UnwrapOr(math.MaxUint), n)
	)
	//
	for i := range n {
		var val F
		//
		arr.Set(i, val.SetBytes(data[i].Bytes()))
	}
	//
	return arr
}

// SplitQualifiedColumnName splits a qualified column name into its module and
// column components.
func splitQualifiedColumnName(name string) (string, string, error) {
	// Now look for qualified name
	i := strings.Index(name, ".")
	if i >= 0 {
		// Split on "."
		return name[0:i], name[i+1:], nil
	}
	// No module name given, therefore its in the prelude.
	return "", name, nil
}

func splitColumnBitwidth(name string) (string, util.Option[uint], error) {
	var (
		err      error
		bitwidth uint64
		bits     = strings.Split(name, "@")
	)
	//
	if len(bits) == 1 {
		// no bitwidth given
		return bits[0], util.None[uint](), nil
	} else if len(bits) > 2 || len(bits[1]) < 2 {
		return "", util.None[uint](), fmt.Errorf("malformed column name \"%s\"", name)
	} else if bits[1][0] != 'u' {
		return "", util.None[uint](), fmt.Errorf("malformed column type \"%s\"", bits[1])
	}
	// Extract colwidth, whilst ignoring column type (for now)
	colwidth := bits[1][1:]
	//
	if bitwidth, err = strconv.ParseUint(colwidth, 10, 17); err != nil {
		// failure
		return "", util.None[uint](), err
	}
	//
	return bits[0], util.Some(uint(bitwidth)), nil
}

func validateBigInts(bitwidth util.Option[uint], data []big.Int) uint {
	if bitwidth.HasValue() {
		var zero = big.NewInt(0)
		//
		for i, val := range data {
			if val.Cmp(zero) < 0 {
				return uint(i)
			} else if uint(val.BitLen()) > bitwidth.Unwrap() {
				return uint(i)
			}
		}
	}
	//
	return math.MaxUint
}
