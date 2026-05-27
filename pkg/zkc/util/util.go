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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// ParseJsonInputFile parses a json input file into an input / output mapping,
// as suitable for booting a machine with.  This can fail with a parsing error.
func ParseJsonInputFile(bytes []byte) (map[string][]byte, error) {
	var (
		rawData map[string]string
		data    map[string][]byte
		err     error
	)
	// Unmarshall data
	if err = json.Unmarshal(bytes, &rawData); err == nil {
		// Parse data
		data = make(map[string][]byte)
		// Initialise data
		for k, v := range rawData {
			// Replace occurrences of "_"
			w := strings.ReplaceAll(v, "_", "")
			//
			if strings.HasPrefix(w, "0x") {
				data[k], err = hex.DecodeString(w[2:])
			} else if w != "" {
				var val big.Int
				if _, ok := val.SetString(w, 10); !ok {
					return nil, fmt.Errorf("malformed numeric literal \"%s\"", w)
				}
				//
				data[k] = val.Bytes()
			}
			//
			if err != nil {
				break
			}
		}
	}
	//
	return data, err
}
