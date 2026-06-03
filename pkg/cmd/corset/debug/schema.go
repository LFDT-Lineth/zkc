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

	"github.com/LFDT-Lineth/zkc/pkg/asm"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/macro"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/micro"
	cmd_util "github.com/LFDT-Lineth/zkc/pkg/cmd/corset/util"
	"github.com/LFDT-Lineth/zkc/pkg/ir/mir"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// PrintSchemas is responsible for printing out a human-readable description of
// a given schema.
func PrintSchemas[F field.Element[F]](stack cmd_util.SchemaStack[F], textwidth uint) {
	//
	for _, schema := range stack.AbstractSchemas() {
		PrintAnySchema(schema, textwidth)
	}
	//
	if stack.HasConcreteSchema() {
		PrintAnySchema(stack.ConcreteSchema(), textwidth)
	}
}

// PrintAnySchema prints out all declarations included in a given schema
func PrintAnySchema[F field.Element[F]](schema schema.AnySchema[F], width uint) {
	first := true
	// Print out each module, one by one.
	for i := schema.Modules(); i.HasNext(); {
		ith := i.Next()
		//
		if isEmptyModule(ith) {
			// Skip empty modules as they just clutter things up.  Typically,
			// for example, the root module is empty.
			continue
		} else if !first {
			fmt.Println()
		}
		//
		switch ith := ith.(type) {
		case *asm.MacroModule[F]:
			printAssemblyFunctionalUnit[macro.Instruction](ith.Function())
		case *asm.MicroModule[F]:
			printAssemblyFunctionalUnit[micro.Instruction](ith.Function())
		default:
			printModule(ith, schema, width)
		}
		//
		first = false
	}
}

// ==================================================================
// Legacy module
// ==================================================================

func printModule[F field.Element[F]](module schema.Module[F], sc schema.AnySchema[F], width uint) {
	var (
		name      = module.Name().String()
		formatter = sexp.NewFormatter(width, true)
		postfix   string
	)
	formatter.Add(&sexp.SFormatter{Head: "if", Priority: 0})
	formatter.Add(&sexp.SFormatter{Head: "ifnot", Priority: 0})
	formatter.Add(&sexp.LFormatter{Head: "begin", Priority: 0})
	formatter.Add(&sexp.LFormatter{Head: "∧", Priority: 1})
	formatter.Add(&sexp.LFormatter{Head: "∨", Priority: 1})
	formatter.Add(&sexp.IFormatter{Head: "==", Priority: 2})
	formatter.Add(&sexp.IFormatter{Head: "!=", Priority: 2})
	formatter.Add(&sexp.IFormatter{Head: "+", Priority: 3})
	formatter.Add(&sexp.IFormatter{Head: "*", Priority: 4})

	if name != "" {
		postfix = fmt.Sprintf(" %s", name)
	}

	if module.IsSynthetic() {
		postfix = fmt.Sprintf("%s :synthetic", postfix)
	}

	if module.IsNative() {
		postfix = fmt.Sprintf("%s :native", postfix)
	}

	if module.IsStatic() {
		postfix = fmt.Sprintf("%s :static", postfix)
	}
	//
	fmt.Printf("(module%s)\n", postfix)
	//
	fmt.Println()
	// Print inputs / outputs
	printRegisters(module, "inputs", func(r register.Register) bool { return r.IsInput() })
	printRegisters(module, "outputs", func(r register.Register) bool { return r.IsOutput() })
	printRegisters(module, "computed", func(r register.Register) bool { return r.IsComputed() })
	// Print static contents (if applicable)
	printStaticContents(module)
	// Print computations
	for i := module.Assignments(); i.HasNext(); {
		ith := i.Next()
		text := formatter.Format(ith.Lisp(sc))
		fmt.Print(text)
	}
	// Print constraints
	for i := module.Constraints(); i.HasNext(); {
		ith := i.Next()
		text := formatter.Format(ith.Lisp(sc))
		//
		if requiresSpacing(ith) {
			fmt.Println()
		}
		//
		fmt.Print(text)
	}
}

func printStaticContents[F field.Element[F]](module schema.Module[F]) {
	if module.IsStatic() {
		var contents = module.StaticContents()
		//
		fmt.Println("(contents")
		//
		for i, row := range contents {
			fmt.Print("   (")
			//
			for j, v := range row {
				if j != 0 {
					fmt.Print(", ")
				}
				//
				fmt.Printf("0x%s", v.Text(16))
			}
			//
			if i+1 != len(contents) {
				fmt.Println("),")
			} else {
				fmt.Println(")")
			}
		}
		//
		fmt.Println(")")
	}
}

func printRegisters[F any](module schema.Module[F], prefix string, filter func(register.Register) bool) {
	var (
		regT string
	)

	if countRegisters(module, filter) != 0 {
		//
		fmt.Printf("(%s\n", prefix)
		//
		for _, r := range module.Registers() {
			if filter(r) {
				if r.IsNative() {
					regT = "𝔽"
				} else {
					regT = fmt.Sprintf("u%d", r.Width())
				}
				// construct name string whilst applying quotes when necessary.
				name := sexp.NewSymbol(r.Name()).String(true)
				//
				fmt.Printf("   (%s %s", name, regT)
				// Print padding
				fmt.Printf(" 0x%s)\n", r.Padding().Text(16))
			}
		}
		//
		fmt.Println(")")
		fmt.Println("")
	}
}

func countRegisters[F any](module schema.Module[F], filter func(register.Register) bool) uint {
	var count = uint(0)
	//
	for _, r := range module.Registers() {
		if filter(r) {
			count++
		}
	}
	//
	return count
}

func requiresSpacing[F field.Element[F]](c schema.Constraint[F]) bool {
	if c, ok := c.(mir.Constraint[F]); ok {
		if _, ok := c.Unwrap().(mir.VanishingConstraint[F]); ok {
			return ok
		}
	}
	//
	return false
}

func isEmptyModule[F any](module schema.Module[F]) bool {
	return len(module.Registers()) == 0 &&
		module.Constraints().Count() == 0 &&
		module.Assignments().Count() == 0
}

// ==================================================================
// Assembly Function
// ==================================================================

func printAssemblyFunctionalUnit[T io.Instruction](f io.Component[T]) {
	printAssemblySignature[T](f)
	printAssemblyRegisters[T](f)
	//
	switch f := f.(type) {
	case *io.Function[T]:
		for pc, insn := range f.Code() {
			fmt.Printf("[%d]\t%s\n", pc, insn.String(f))
		}
	default:
		panic("unknown component")
	}
	//
	fmt.Println("}")
}

func printAssemblySignature[T io.Instruction](f io.Component[T]) {
	first := true
	//
	fmt.Printf("fn %s(", f.Name())
	//
	for _, r := range f.Registers() {
		if r.IsInput() {
			if !first {
				fmt.Printf(", ")
			} else {
				first = false
			}
			//
			fmt.Printf("%s u%d", r.Name(), r.Width())
		}
	}
	//
	fmt.Printf(") -> (")
	// reset
	first = true
	//
	for _, r := range f.Registers() {
		if r.IsOutput() {
			if !first {
				fmt.Printf(", ")
			} else {
				first = false
			}
			//
			fmt.Printf("%s u%d", r.Name(), r.Width())
		}
	}
	//
	fmt.Println(") {")
}

func printAssemblyRegisters[T io.Instruction](f io.Component[T]) {
	for _, r := range f.Registers() {
		if !r.IsInput() && !r.IsOutput() {
			fmt.Printf("\tvar %s u%d\n", r.Name(), r.Width())
		}
	}
}
