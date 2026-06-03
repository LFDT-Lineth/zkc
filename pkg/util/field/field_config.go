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
package field

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
)

// GF_251 is teany tiny prime field used exclusively for testing.
var GF_251 = Config{"GF_251", 7, 4}

// GF_8209 is small prime field used exclusively for testing.
var GF_8209 = Config{"GF_8209", 13, 8}

// KOALABEAR_16 corresponds to the KoalaBear field with a 16bit register size.
var KOALABEAR_16 = Config{"KOALABEAR_16", 30, 16}

// BLS12_377 is the defacto default field at this time.
var BLS12_377 = Config{"BLS12_377", 252, 160}

// FIELD_CONFIGS determines the set of supported fields.
var FIELD_CONFIGS = []Config{
	GF_251,
	GF_8209,
	KOALABEAR_16,
	BLS12_377,
}

// Config provides a simple mechanism for configuring the field agnosticity
// pipeline.
type Config struct {
	// Name suitable for identifying the config.  This is only really used for
	// improving error reporting, etc.
	Name string
	// Maximum field bandwidth available in the field.
	BandWidth uint
	// Maximum register width to use with this field.
	RegisterWidth uint
}

// Modulus returns the (prime) modulus used for this field.
func (p Config) Modulus() *big.Int {
	switch p {
	case GF_251:
		return big.NewInt(251)
	case GF_8209:
		return big.NewInt(8209)
	case KOALABEAR_16:
		return big.NewInt(koalabear.Modulus)
	case BLS12_377:
		return bls12_377.Modulus
	default:
		panic("unsupported field configuration")
	}
}

// GetConfig returns the field configuration corresponding with the given
// name, or nil no such config exists.
func GetConfig(name string) *Config {
	for i := range FIELD_CONFIGS {
		if FIELD_CONFIGS[i].Name == name {
			return &FIELD_CONFIGS[i]
		}
	}
	//
	return nil
}
