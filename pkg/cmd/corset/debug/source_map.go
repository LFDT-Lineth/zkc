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
package debug

import (
	"fmt"
	"math"

	"github.com/LFDT-Lineth/zkc/pkg/corset"
)

// PrintSourceMap provides a suitable human-readable view of the internal source
// map attribute.
func PrintSourceMap(srcmap *corset.SourceMap) {
	printSourceMapModule(1, srcmap.Root)
}

func printSourceMapModule(indent uint, module corset.SourceModule) {
	//
	fmt.Println()
	printIndent(indent)
	//
	if module.Public {
		fmt.Printf("pub ")
	}
	//
	if module.Virtual {
		fmt.Printf("virtual ")
	}
	//
	fmt.Printf("module \"%s\"", module.Name)
	//
	if module.Selector.HasValue() {
		fmt.Printf(" when %s", module.Selector.Unwrap())
	}
	//
	fmt.Println(":")
	//
	indent++
	// Print constants
	for _, c := range module.Constants {
		printIndent(indent)
		//
		if c.Extern {
			fmt.Printf("extern\t")
		} else {
			fmt.Printf("const\t")
		}
		//
		if c.Bitwidth != math.MaxUint {
			fmt.Printf("u%d ", c.Bitwidth)
		}
		//
		fmt.Printf("%s = %s\n", c.Name, &c.Value)
	}
	// Print columns
	for _, c := range module.Columns {
		printIndent(indent)
		fmt.Printf("u%d\t%s\t[", c.Bitwidth, c.Name)
		//
		for i, a := range sourceColumnAttrs(c) {
			if i == 0 {
				fmt.Print(a)
			} else {
				fmt.Printf(", %s", a)
			}
		}

		fmt.Println("]")
	}
	// Print submodules
	for _, m := range module.Submodules {
		printSourceMapModule(indent, m)
	}
}

func sourceColumnAttrs(col corset.SourceColumn) []string {
	var attrs []string
	//
	attrs = append(attrs, fmt.Sprintf("r%d", col.Register.Column().Unwrap()))
	//
	if col.Multiplier != 1 {
		attrs = append(attrs, fmt.Sprintf("×%d", col.Multiplier))
	}
	//
	if col.Computed {
		attrs = append(attrs, "computed")
	}
	//
	if col.MustProve {
		attrs = append(attrs, "proved")
	}
	//
	switch col.Display {
	case corset.DISPLAY_HEX:
		attrs = append(attrs, "hex")
	case corset.DISPLAY_DEC:
		attrs = append(attrs, "dec")
	case corset.DISPLAY_BYTES:
		attrs = append(attrs, "bytes")
	}
	//
	return attrs
}
