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
package byte_test

import (
	"errors"
	"testing"

	bytedec "github.com/LFDT-Lineth/zkc/pkg/util/collection/byte"
)

func TestDecoderSequence(t *testing.T) {
	input := []byte{
		0xab,       // u8
		0x01, 0x02, // u16
		0x01, 0x02, 0x03, 0x04, // u32
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, // u64
		0x00, 0x00, 0x00, 0x02, 0xaa, 0xbb, // bytes (len 2)
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07, // u64s (len 1)
	}
	d := bytedec.NewDecoder(input)
	//
	if v, err := d.Uint8(); err != nil || v != 0xab {
		t.Fatalf("u8: %x %v", v, err)
	}
	//
	if v, err := d.Uint16(); err != nil || v != 0x0102 {
		t.Fatalf("u16: %x %v", v, err)
	}
	//
	if v, err := d.Uint32(); err != nil || v != 0x01020304 {
		t.Fatalf("u32: %x %v", v, err)
	}
	//
	if v, err := d.Uint64(); err != nil || v != 0xff {
		t.Fatalf("u64: %x %v", v, err)
	}
	//
	if v, err := d.Bytes(); err != nil || len(v) != 2 || v[0] != 0xaa || v[1] != 0xbb || cap(v) != 2 {
		t.Fatalf("bytes: %x (cap %d) %v", v, cap(v), err)
	}
	//
	if v, err := d.Uint64s(); err != nil || len(v) != 1 || v[0] != 7 {
		t.Fatalf("u64s: %v %v", v, err)
	}
	//
	if d.Remaining() != 0 {
		t.Fatalf("remaining: %d", d.Remaining())
	}
	// Reading past the end must fail, not panic.
	if _, err := d.Uint8(); !errors.Is(err, bytedec.ErrTruncated) {
		t.Fatalf("eof: %v", err)
	}
}

func TestDecoderZeroLengthIsNil(t *testing.T) {
	d := bytedec.NewDecoder([]byte{0, 0, 0, 0, 0, 0, 0, 0})
	//
	if v, err := d.Bytes(); err != nil || v != nil {
		t.Fatalf("bytes: %v %v", v, err)
	}
	//
	if v, err := d.Uint64s(); err != nil || v != nil {
		t.Fatalf("u64s: %v %v", v, err)
	}
}

func TestDecoderCountGuardsAllocation(t *testing.T) {
	// Count of 0xffffffff u64 elements with no data following.
	d := bytedec.NewDecoder([]byte{0xff, 0xff, 0xff, 0xff})
	//
	if _, err := d.Count(8); !errors.Is(err, bytedec.ErrTruncated) {
		t.Fatalf("count: %v", err)
	}
	// A count which fits exactly is accepted.
	d = bytedec.NewDecoder([]byte{0, 0, 0, 2, 1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8})
	//
	if n, err := d.Count(8); err != nil || n != 2 {
		t.Fatalf("count: %d %v", n, err)
	}
}

func TestDecoderTruncatedMultiByte(t *testing.T) {
	// Three bytes available, but a u32 is requested.
	d := bytedec.NewDecoder([]byte{1, 2, 3})
	//
	if _, err := d.Uint32(); !errors.Is(err, bytedec.ErrTruncated) {
		t.Fatalf("u32: %v", err)
	}
	// Nothing was consumed by the failed read.
	if d.Remaining() != 3 {
		t.Fatalf("remaining: %d", d.Remaining())
	}
}
