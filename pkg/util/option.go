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
package util

import (
	"bytes"
	"encoding/gob"
)

// Option provides a simple encoding for an optional value.  A key advantage
// over a pointer is that this serialises nicely.
type Option[T any] struct {
	// Indicates whether value present
	some bool
	// The value itself
	value T
}

// Some constructs an option which holds a value.
func Some[T any](val T) Option[T] {
	return Option[T]{true, val}
}

// None constructs an option which doesn't hold a value.
func None[T any]() Option[T] {
	var empty T
	return Option[T]{false, empty}
}

// HasValue indicates whether or not this option contains an actual value, or
// whether it is empty.
func (o Option[T]) HasValue() bool {
	return o.some
}

// IsEmpty indicates whether or not this option is empty (i.e. contains no value).
func (o Option[T]) IsEmpty() bool {
	return !o.some
}

// Unwrap returns the value contained, or panics if this option is empty.
func (o Option[T]) Unwrap() T {
	if o.some {
		return o.value
	}
	//
	panic("cannot unwrap an empty option")
}

// UnwrapOr returns the value contained, or a given default value is the option
// is empty.
func (o Option[T]) UnwrapOr(value T) T {
	if o.some {
		return o.value
	}
	//
	return value
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// GobEncode an option.  This allows it to be marshalled into a binary form.
func (o *Option[T]) GobEncode() (data []byte, err error) {
	var (
		buffer     bytes.Buffer
		gobEncoder = gob.NewEncoder(&buffer)
	)
	// Some
	if err := gobEncoder.Encode(&o.some); err != nil {
		return nil, err
	}
	// Decide whether need anything else.
	if o.some {
		// Value
		if err := gobEncoder.Encode(&o.value); err != nil {
			return nil, err
		}
	}
	// Success
	return buffer.Bytes(), nil
}

// GobDecode a previously encoded option
func (o *Option[T]) GobDecode(data []byte) error {
	buffer := bytes.NewBuffer(data)
	gobDecoder := gob.NewDecoder(buffer)
	// Some
	if err := gobDecoder.Decode(&o.some); err != nil {
		return err
	}
	// Check whether value provided
	if o.some {
		// Value
		if err := gobDecoder.Decode(&o.value); err != nil {
			return err
		}
	}
	// Success!
	return nil
}
