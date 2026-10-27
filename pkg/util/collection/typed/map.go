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
package typed

import (
	"encoding/json"
	"slices"
)

// Map provides a wrapper around a map holding arbitrary values which supports
// introspection.  It just provides an easy way to work with structured data,
// such as from JSON files.
type Map struct {
	items map[string]any
}

// NewMap constructs a new wrapper around a given map of arbitrary data.
func NewMap(items map[string]any) Map {
	return Map{items}
}

// FromJsonBytes attempts to construct a map from an array of JSON formatted
// bytes.
func FromJsonBytes(js []byte) (Map, error) {
	var jsonMap map[string]any
	//
	if err := json.Unmarshal(js, &jsonMap); err != nil {
		return Map{nil}, err
	}
	//
	return NewMap(jsonMap), nil
}

// IsEmpty checks whether this typed map is empty or not.
func (p *Map) IsEmpty() bool {
	return len(p.items) == 0
}

// ToJsonBytes converts this map into an array of JSON formatted bytes, or
// returns an error.
func (p *Map) ToJsonBytes() ([]byte, error) {
	return json.Marshal(p.items)
}

// Keys returns the list of items in this map, but not their contents.
func (p *Map) Keys() []string {
	var keys []string
	//
	for k := range p.items {
		keys = append(keys, k)
	}
	// Sort keys for determinism
	slices.Sort(keys)
	//
	return keys
}

// String attempts to retrieve an item from the map as a string.  This can fail
// in two ways: either no such item exists; or, an item exists but has the wrong
// type.
func (p *Map) String(key string) (string, bool) {
	if val, ok := p.items[key]; ok {
		sval, ok := val.(string)
		return sval, ok
	}
	// Failure
	return "", false
}

// Map attempts to retrieve an item from the map as a string.  This can fail in
// two ways: either no such item exists; or, an item exists but has the wrong
// type.
func (p *Map) Map(key string) (Map, bool) {
	if val, ok := p.items[key]; ok {
		sval, ok := val.(map[string]any)
		return Map{sval}, ok
	}
	// Failure
	return Map{nil}, false
}

// Nil determines whether or not the given key is assigned the nil value.
func (p *Map) Nil(key string) bool {
	if val, ok := p.items[key]; ok {
		return val == nil
	}
	// Failure
	return false
}
