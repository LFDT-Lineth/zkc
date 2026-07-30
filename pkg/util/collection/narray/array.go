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
package narray

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
)

// Predicate abstracts the notion of a function which identifies something.
type Predicate[T any] = func(T) bool

// Array provides a generice interface to an array of elements.  Typically, we
// are interested in arrays of field elements here.
type Array[T any] interface {
	fmt.Stringer
	// Return the number of bits required to store an element of this array.
	BitWidth() uint
	// Clone this array producing a mutable copy
	Clone() MutArray[T]
	// Encode writes the contents of this array into the given buffer, using
	// the natural encoding for this array representation.  The encoding is
	// self-delimiting given the length of the array (i.e. Decode can determine
	// how many bytes to read given the number of encoded elements).
	Encode(*bytes.Buffer)
	// Get returns the element at the given index in this array.
	Get(uint) T
	// Returns the number of elements in this array.
	Len() uint
}

// MutArray provides a generice interface to an array of elements.  Typically, we
// are interested in arrays of field elements here.
type MutArray[T any] interface {
	Array[T]
	// Append new element onto the end of array producing an updated array.
	// This updates the array in place, and will panic if the given value is not
	// representable in the array.
	Append(T)
	// Decode reads a given number of elements from the given buffer into this
	// array, replacing any existing contents.  The data is expected to be in
	// the natural encoding for this array representation (i.e. as produced by
	// Encode).
	Decode(uint, *bytes.Buffer) error
	// Set the element at the given index in this array, overwriting the
	// original value. This updates the array in place, and will panic if the
	// given value is not representable in the array.
	Set(uint, T)
	// ToLegacy (efficiently) constructs an equivalent legacy array from this
	// array.  This should be considered a destructive operation and, hence,
	// once done this array should no longer be used.
	ToLegacy() array.MutArray[T]
}

// writeUvarint writes an unsigned varint into the given buffer.
func writeUvarint(buffer *bytes.Buffer, value uint64) {
	var bytes [binary.MaxVarintLen64]byte
	//
	n := binary.PutUvarint(bytes[:], value)
	buffer.Write(bytes[:n])
}

// readUvarint reads an unsigned varint from the given buffer.
func readUvarint(buffer *bytes.Buffer) (uint64, error) {
	return binary.ReadUvarint(buffer)
}

// writeWordBytes writes a word into the given buffer as a length-prefixed
// sequence of raw bytes.
func writeWordBytes(buffer *bytes.Buffer, bytes []byte) {
	writeUvarint(buffer, uint64(len(bytes)))
	buffer.Write(bytes)
}

// readWordBytes reads a length-prefixed sequence of raw bytes (as written by
// writeWordBytes) from the given buffer.
func readWordBytes(buffer *bytes.Buffer) ([]byte, error) {
	length, err := readUvarint(buffer)
	//
	if err != nil {
		return nil, err
	} else if length > uint64(buffer.Len()) {
		return nil, fmt.Errorf("word length %d exceeds remaining bytes %d", length, buffer.Len())
	}
	// Observe bytes must be cloned, since the slice returned by Next is only
	// valid until the next buffer operation.
	return bytes.Clone(buffer.Next(int(length))), nil
}
