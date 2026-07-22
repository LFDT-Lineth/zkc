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
package descriptor

import (
	"bytes"
	"encoding/gob"
)

// FunctionKind captures the execution-relevant properties of a function.
type FunctionKind struct {
	native, unsafeArgs bool
}

// IsNative reports whether this function is backed by a native circuit rather
// than bytecode instructions.
func (p FunctionKind) IsNative() bool {
	return p.native
}

// AllowsUnsafeArgs reports whether calls may supply arguments which are undefined
// on some paths reaching the call.
func (p FunctionKind) AllowsUnsafeArgs() bool {
	return p.unsafeArgs
}

// GobEncode marshals this function kind.
//
// nolint
func (p *FunctionKind) GobEncode() ([]byte, error) {
	var buffer bytes.Buffer
	gobEncoder := gob.NewEncoder(&buffer)
	//
	if err := gobEncoder.Encode(p.native); err != nil {
		return nil, err
	}
	//
	if err := gobEncoder.Encode(p.unsafeArgs); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

// GobDecode unmarshals this function kind.
//
// nolint
func (p *FunctionKind) GobDecode(data []byte) error {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
	)
	//
	if err := gobDecoder.Decode(&p.native); err != nil {
		return err
	}
	//
	return gobDecoder.Decode(&p.unsafeArgs)
}

var (
	// BYTECODE_FUNCTION represents a safe function implemented by bytecode.
	BYTECODE_FUNCTION = FunctionKind{false, false}
	// NATIVE_FUNCTION represents a safe function backed by a native circuit.
	NATIVE_FUNCTION = FunctionKind{true, false}
	// UNSAFE_ARGS_FUNCTION represents a bytecode function which may receive undefined arguments.
	UNSAFE_ARGS_FUNCTION = FunctionKind{false, true}
	// NATIVE_UNSAFE_ARGS_FUNCTION represents a native function which may receive undefined arguments.
	NATIVE_UNSAFE_ARGS_FUNCTION = FunctionKind{true, true}
)
