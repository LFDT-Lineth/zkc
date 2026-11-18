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
package byte

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrTruncated signals that the input ended before a complete value could be
// decoded.  All errors returned by Decoder wrap this.
var ErrTruncated = errors.New("truncated input")

// Decoder is a cursor over a byte array which consumes big-endian encoded
// values from the front.  Every read is bounds checked, returning an error
// (rather than panicking) on malformed input.  Variable-length values are
// returned as subslices of the underlying input, hence that input must not be
// modified whilst the decoded values are in use.
type Decoder struct {
	buf []byte
}

// NewDecoder constructs a decoder positioned at the start of the given bytes.
func NewDecoder(buf []byte) Decoder {
	return Decoder{buf}
}

// Remaining returns the number of bytes not yet consumed.
func (d *Decoder) Remaining() int {
	return len(d.buf)
}

// need checks that at least n bytes remain.
func (d *Decoder) need(n int) error {
	if len(d.buf) < n {
		return fmt.Errorf("%w: need %d bytes, have %d", ErrTruncated, n, len(d.buf))
	}
	//
	return nil
}

// Uint8 consumes a single byte.
func (d *Decoder) Uint8() (uint8, error) {
	if err := d.need(1); err != nil {
		return 0, err
	}
	//
	v := d.buf[0]
	d.buf = d.buf[1:]
	//
	return v, nil
}

// Uint16 consumes a big-endian 16-bit unsigned integer.
func (d *Decoder) Uint16() (uint16, error) {
	if err := d.need(2); err != nil {
		return 0, err
	}
	//
	v := binary.BigEndian.Uint16(d.buf)
	d.buf = d.buf[2:]
	//
	return v, nil
}

// Uint32 consumes a big-endian 32-bit unsigned integer.
func (d *Decoder) Uint32() (uint32, error) {
	if err := d.need(4); err != nil {
		return 0, err
	}
	//
	v := binary.BigEndian.Uint32(d.buf)
	d.buf = d.buf[4:]
	//
	return v, nil
}

// Uint64 consumes a big-endian 64-bit unsigned integer.
func (d *Decoder) Uint64() (uint64, error) {
	if err := d.need(8); err != nil {
		return 0, err
	}
	//
	v := binary.BigEndian.Uint64(d.buf)
	d.buf = d.buf[8:]
	//
	return v, nil
}

// Count consumes a 32-bit element count, checking that the remaining input
// could hold that many elements of (at least) elemSize bytes each.  This
// guards against allocating huge slices on corrupt input, and so should be
// used (rather than Uint32) wherever a count drives an allocation.
func (d *Decoder) Count(elemSize int) (int, error) {
	n, err := d.Uint32()
	//
	if err != nil {
		return 0, err
	} else if uint64(n)*uint64(elemSize) > uint64(len(d.buf)) {
		return 0, fmt.Errorf("%w: count %d exceeds remaining %d bytes", ErrTruncated, n, len(d.buf))
	}
	//
	return int(n), nil
}

// Bytes consumes a 32-bit length-prefixed byte array, returned as a subslice
// of the underlying input (nil when the length is zero).
func (d *Decoder) Bytes() ([]byte, error) {
	n, err := d.Count(1)
	//
	if err != nil {
		return nil, err
	} else if n == 0 {
		return nil, nil
	}
	//
	v := d.buf[:n:n]
	d.buf = d.buf[n:]
	//
	return v, nil
}

// Uint64s consumes a 32-bit length-prefixed array of big-endian 64-bit
// unsigned integers (nil when the length is zero).
func (d *Decoder) Uint64s() ([]uint64, error) {
	n, err := d.Count(8)
	//
	if err != nil {
		return nil, err
	} else if n == 0 {
		return nil, nil
	}
	//
	v := make([]uint64, n)
	//
	for i := range v {
		v[i] = binary.BigEndian.Uint64(d.buf[8*i:])
	}
	//
	d.buf = d.buf[8*n:]
	//
	return v, nil
}
