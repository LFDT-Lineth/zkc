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
	"testing"
)

func Test_TypedMap_01(t *testing.T) {
	tmap := NewMap(nil)
	// Check nil dereference doesn't occur.
	if val, ok := tmap.String("x"); ok {
		t.Errorf("unexpected value: %v (%t)", val, ok)
	}
}

func Test_TypedMap_02(t *testing.T) {
	tmap := jsonToTypedMap("{\"x\": \"1\"}")
	if val, ok := tmap.String("x"); !ok || val != "1" {
		t.Errorf("unexpected value: %v (%t)", val, ok)
	}
}

func Test_TypedMap_03(t *testing.T) {
	tmap := jsonToTypedMap("{\"x\": {\"y\": \"1\"}}")
	//
	if xval, ok := tmap.Map("x"); !ok {
		t.Errorf("unexpected value: %v (%t)", xval, ok)
	} else if yval, ok := xval.String("y"); !ok || yval != "1" {
		t.Errorf("unexpected value: %v (%t)", yval, ok)
	}
}

func Test_TypedMap_04(t *testing.T) {
	tmap := jsonToTypedMap("{\"x\": \"1\"}")
	if !slices.Equal(tmap.Keys(), []string{"x"}) {
		t.Errorf("unexpected keys: %v", tmap.Keys())
	}
}

func Test_TypedMap_05(t *testing.T) {
	tmap := jsonToTypedMap("{\"x\": \"1\", \"y\": \"2\"}")
	if !slices.Equal(tmap.Keys(), []string{"x", "y"}) {
		t.Errorf("unexpected keys: %v", tmap.Keys())
	}
}

func jsonToTypedMap(js string) Map {
	var jsonMap map[string]any
	//
	if err := json.Unmarshal([]byte(js), &jsonMap); err != nil {
		panic(err)
	}
	//
	return NewMap(jsonMap)
}
