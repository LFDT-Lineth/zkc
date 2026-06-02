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
	"fmt"
	"math/big"
	"strings"
)

// EscapeFormattedText takes a string and escapes any characters which need to
// be escaped in order to be printed.
func EscapeFormattedText(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "\n", "\\n")
	text = strings.ReplaceAll(text, "\t", "\\t")
	//
	return strings.ReplaceAll(text, "\r", "\\r")
}

const (
	// FORMAT_NONE indicates an empty format string
	FORMAT_NONE uint = iota
	// FORMAT_DEC indicates to format in decimal
	FORMAT_DEC
	// FORMAT_HEX indicates to format in hexadecimal
	FORMAT_HEX
	// FORMAT_BIN indicates to format in binary
	FORMAT_BIN
	// FORMAT_CHR indicates to format as a single ASCII character.  The
	// argument is required (at type-check time) to be a concrete u8; the
	// rendered output is the single byte interpreted as a character.
	FORMAT_CHR
)

// Formattable captures a numeric element which can be formatted in a particular
// base.
type Formattable interface {
	// Text returns the given word formated in the given base
	Text(base int) string
	// BigInt returns big integer representation of value.
	BigInt() *big.Int
}

// Format simply encodes the set of permitted formatting strings in a printf
// statement, such as "%d", "%x", "%08x", "%8x", etc.  Width specifies an
// optional minimum number of digits to render, and ZeroPad selects between
// zero-padding ('0' flag) and space-padding (the default).  Any base prefix
// ("0x", "0b") is rendered separately and does not count towards Width.
type Format struct {
	Code    uint
	Width   uint
	ZeroPad bool
}

// EMPTY_FORMAT indicates no formatted argument is required.
var EMPTY_FORMAT = Format{Code: FORMAT_NONE}

// DecimalFormat constructs a new decimal format.
func DecimalFormat() Format {
	return Format{Code: FORMAT_DEC}
}

// HexFormat constructs a new hexadecimal format.
func HexFormat() Format {
	return Format{Code: FORMAT_HEX}
}

// BinFormat constructs a new binary format.
func BinFormat() Format {
	return Format{Code: FORMAT_BIN}
}

// CharFormat constructs a new character format.  %c does not support
// width/zero-padding flags; the parser rejects them before this is reached.
func CharFormat() Format {
	return Format{Code: FORMAT_CHR}
}

// HasFormat checks whether this actually represents a format, or is empty.
func (p Format) HasFormat() bool {
	return p.Code != FORMAT_NONE
}

func (p Format) String() string {
	var (
		builder  strings.Builder
		typeChar byte
	)
	//
	switch p.Code {
	case FORMAT_DEC:
		typeChar = 'd'
	case FORMAT_HEX:
		typeChar = 'x'
	case FORMAT_BIN:
		typeChar = 'b'
	case FORMAT_CHR:
		typeChar = 'c'
	default:
		panic("invalid format")
	}
	//
	builder.WriteByte('%')
	//
	if p.ZeroPad {
		builder.WriteByte('0')
	}
	//
	if p.Width > 0 {
		fmt.Fprintf(&builder, "%d", p.Width)
	}
	//
	builder.WriteByte(typeChar)
	//
	return builder.String()
}
