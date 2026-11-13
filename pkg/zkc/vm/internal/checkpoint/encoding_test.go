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
package checkpoint_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/checkpoint"
)

func sampleCheckPoint() checkpoint.CheckPoint {
	return checkpoint.NewCheckPoint(7, []byte{0xde, 0xad, 0xbe, 0xef}, []checkpoint.Memory{
		// ROM-like: single unstamped page at address zero
		checkpoint.NewMemory(1, 0, checkpoint.NewPage(0, []byte{1, 2, 3})),
		// Timestamped RAM: two sparse pages
		checkpoint.NewMemory(2, 0x0102030405060708,
			checkpoint.NewStampedPage(16, []byte{9, 8, 7, 6}, []uint64{1, 1<<63 | 5}),
			checkpoint.NewStampedPage(1<<40, []byte{5}, []uint64{3}),
		),
		// Memory with no pages at all
		checkpoint.NewMemory(3, 42),
	})
}

func TestCheckPointRoundTrip(t *testing.T) {
	var (
		original = sampleCheckPoint()
		decoded  checkpoint.CheckPoint
	)
	//
	if err := decoded.Unmarshal(original.Marshal()); err != nil {
		t.Fatal(err)
	}
	//
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip mismatch:\n have %+v\n want %+v", decoded, original)
	}
}

func TestCheckPointEmptyFieldsDecodeAsNil(t *testing.T) {
	var (
		// Empty (but non-nil) arguments / data / stamps must decode as nil, so
		// that Stamps() == nil continues to identify an unstamped page.
		original = checkpoint.NewCheckPoint(0, []byte{}, []checkpoint.Memory{
			checkpoint.NewMemory(0, 0, checkpoint.NewStampedPage(0, []byte{}, []uint64{})),
		})
		decoded checkpoint.CheckPoint
	)
	//
	if err := decoded.Unmarshal(original.Marshal()); err != nil {
		t.Fatal(err)
	}
	//
	if decoded.ArgumentBytes() != nil {
		t.Errorf("arguments: want nil, got %v", decoded.ArgumentBytes())
	}
	//
	page := decoded.Memories()[0].Pages()[0]
	//
	if page.Bytes() != nil || page.Stamps() != nil {
		t.Errorf("page: want nil data and stamps, got %v / %v", page.Bytes(), page.Stamps())
	}
}

func TestCheckPointUnmarshalRejectsTruncation(t *testing.T) {
	encoded := sampleCheckPoint().Marshal()
	// Every proper prefix must be rejected (never panic, never accept).
	for n := range encoded {
		var cp checkpoint.CheckPoint
		//
		if err := cp.Unmarshal(encoded[:n]); !errors.Is(err, checkpoint.ErrMalformed) {
			t.Errorf("prefix of %d bytes: want ErrMalformed, got %v", n, err)
		}
	}
}

func TestCheckPointUnmarshalRejectsTrailingBytes(t *testing.T) {
	var (
		encoded = append(sampleCheckPoint().Marshal(), 0)
		cp      checkpoint.CheckPoint
	)
	//
	if err := cp.Unmarshal(encoded); !errors.Is(err, checkpoint.ErrMalformed) {
		t.Fatalf("want ErrMalformed, got %v", err)
	}
}

func TestCheckPointUnmarshalRejectsHugeCounts(t *testing.T) {
	// function=0, len(args)=0, len(memories)=0xffffffff with nothing following.
	// A naive decoder would attempt to allocate 4G memories here.
	encoded := []byte{0, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}
	//
	var cp checkpoint.CheckPoint
	//
	if err := cp.Unmarshal(encoded); !errors.Is(err, checkpoint.ErrMalformed) {
		t.Fatalf("want ErrMalformed, got %v", err)
	}
}
