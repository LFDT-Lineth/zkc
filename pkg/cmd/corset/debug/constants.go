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

	"github.com/LFDT-Lineth/zkc/pkg/binfile"
	"github.com/LFDT-Lineth/zkc/pkg/corset"
)

// PrintExternalisedConstants is responsible for printing any externalised
// constants contained within the given binary file.
func PrintExternalisedConstants(binf *binfile.BinaryFile) {
	//
	fmt.Println("External constants:")
	// Sanity check debug information is available.
	srcmap, srcmap_ok := binfile.GetAttribute[*corset.SourceMap](binf)
	//
	if !srcmap_ok {
		fmt.Println("\t(no information available)")
		return
	}
	//
	printExternalisedModuleConstants(1, srcmap.Root)
}

func printExternalisedModuleConstants(indent uint, mod corset.SourceModule) {
	first := true
	// print constants in this module.
	for _, c := range mod.Constants {
		if c.Extern {
			if first && mod.Name != "" {
				printIndent(indent)
				fmt.Printf("%s:\n", mod.Name)
				//
				indent++
			}
			//
			printIndent(indent)
			//
			if c.Bitwidth != math.MaxUint {
				fmt.Printf("%s (%s): u%d\n", c.Name, c.Value.String(), c.Bitwidth)
			} else {
				fmt.Printf("%s: %s\n", c.Name, c.Value.String())
			}
			//
			first = false
		}
	}
	// traverse submodules
	for _, m := range mod.Submodules {
		printExternalisedModuleConstants(indent, m)
	}
}

func printIndent(indent uint) {
	for i := uint(0); i < indent; i++ {
		fmt.Print("\t")
	}
}
