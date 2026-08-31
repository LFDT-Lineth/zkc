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
package array

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Array provides a generice interface to an array of elements.  Typically, we
// are interested in arrays of field elements here.
type Array[T any] interface {
	fmt.Stringer
	// Return the number of bits required to store an element of this array.
	BitWidth() uint
	// Bytes returns (approximately) the number of bytes required to store the
	// data of this column.
	Bytes() uint
	// Get returns the element at the given index in this array.
	Get(uint) T
	// Returns the number of elements in this array.
	Len() uint
	// Pad returns a copy of this array padding with n zero values prepended.
	// The receiver is left unmodified.
	Pad(uint) Array[T]
}

// MutArray provides a generice interface to an array of elements.  Typically, we
// are interested in arrays of field elements here.
type MutArray[T any] interface {
	// Build the given array
	Build() Array[T]
	// Append new element onto the end of array producing an updated array.
	// This updates the array in place, and will panic if the given value is not
	// representable in the array.
	Append(T) MutArray[T]
	// Returns current height of array being built
	Height() uint
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
