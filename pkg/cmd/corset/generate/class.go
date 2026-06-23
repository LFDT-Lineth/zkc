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
package generate

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"path/filepath"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/binfile"
	"github.com/LFDT-Lineth/zkc/pkg/corset"
	sc "github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/typed"
)

// JavaTraceClass generates a suitable trace class for Java integration.
func JavaTraceClass(filename string, pkgname string, super string, spillage []uint,
	binf *binfile.BinaryFile) (string, error) {
	var (
		builder strings.Builder
		// Extract source map (which we assume is present)
		srcmap, _ = binfile.GetAttribute[*corset.SourceMap](binf)
		// Extract base of filename
		basename = filepath.Base(filename)
	)
	// Sanity check a request is made to generate a java source file.
	if !strings.HasSuffix(basename, ".java") {
		return "", errors.New("invalid Java classname")
	}
	// Strip suffix to determine classname
	classname := strings.TrimSuffix(basename, ".java")
	metadata, err := binf.Header.GetMetaData()
	// Error check
	if err != nil {
		return "", err
	}
	// begin generation
	generateClassHeader(pkgname, metadata, &builder)
	generateClassContents(classname, super, srcmap.Root, metadata, spillage,
		&binf.Schema, indentBuilder{0, &builder})
	//
	return builder.String(), nil
}

func generateClassHeader(pkgname string, metadata typed.Map, builder *strings.Builder) {
	builder.WriteString(license)
	// Write package line
	if pkgname != "" {
		fmt.Fprintf(builder, "package %s;\n", pkgname)
	}
	//
	builder.WriteString(javaImports)
	builder.WriteString(javaWarning)
	//
	if !metadata.IsEmpty() {
		// Write embedded metadata for the record.
		builder.WriteString(" * <p>Embedded Metata</p>\n")
		builder.WriteString(" * <ul>\n")
		//
		for _, k := range metadata.Keys() {
			builder.WriteString(" * <li>")
			builder.WriteString(k)
			builder.WriteString(": ")

			if v, ok := metadata.String(k); ok {
				builder.WriteString(v)
			} else {
				// NOTE: for now, we don't support nested maps.  This could be added
				// relatively straightforwardly.
				builder.WriteString("???")
			}

			builder.WriteString("</li>\n")
		}
		//
		builder.WriteString(" * </ul>\n")
	}
	//
	builder.WriteString(" */\n")
}

func generateClassContents[F any](className string, super string, mod corset.SourceModule, metadata typed.Map,
	spillage []uint, schema sc.AnySchema[F], builder indentBuilder) {
	// Attempt to find module
	mid, ok := schema.Modules().Find(func(m sc.Module[F]) bool { return m.Name().Name == mod.Name })
	// Sanity check we found it
	if !ok {
		panic(fmt.Sprintf("unable to find module \"%s\"", mod.Name))
	}
	// Generate what we need
	generateJavaClassHeader(mod.Name == "", className, super, builder)
	generateJavaModuleConstants(spillage[mid], mod.Constants, builder.Indent())
	generateJavaModuleSubmoduleFields(mod.Submodules, builder.Indent())
	//
	if mod.Name == "" {
		generateJavaModuleMetadata(metadata, builder.Indent())
	}
	//
	generateJavaModuleHeaders(mod, schema, builder.Indent())
	//
	generateJavaModuleRegisterFields(mod, schema, builder.Indent())
	generateJavaModuleHeader(builder.Indent())
	generateJavaModuleConstructor(className, mod, builder.Indent())
	generateJavaModuleOpen(mod, schema, builder.Indent())
	generateJavaModuleColumnSetters(className, mod, schema, builder.Indent())
	generateJavaModuleSize(builder.Indent())
	generateJavaModuleValidateRow(className, mod, schema, builder.Indent())
	generateJavaModuleFillAndValidateRow(className, mod, schema, builder.Indent())
	// Generate any submodules
	for _, submod := range mod.Submodules {
		if submod.Public && !submod.Virtual {
			name := toPascalCase(submod.Name)
			superSub := fmt.Sprintf("%s.%s", super, name)
			generateClassContents(name, superSub, submod, metadata, spillage, schema, builder.Indent())
		} else if submod.Public {
			generateJavaModuleColumnSetters(className, submod, schema, builder.Indent())
		}
	}
	//
	generateJavaClassFooter(builder)
}

func generateJavaClassHeader(root bool, classname string, super string, builder indentBuilder) {
	var extends string
	if super != "" {
		extends = fmt.Sprintf(" implements %s", super)
	}
	//
	if root {
		builder.WriteIndentedString("public class ", classname, extends, " {\n")
	} else {
		builder.WriteIndentedString("public static class ", classname, extends, " {\n")
	}
}

func generateJavaClassFooter(builder indentBuilder) {
	builder.WriteIndentedString("}\n")
}

func generateJavaModuleHeaders[F any](mod corset.SourceModule, schema sc.AnySchema[F], builder indentBuilder) {
	//
	i1Builder := builder.Indent()
	// Count of created registers
	count := uint(0)
	//
	builder.WriteIndentedString("public List<ColumnHeader> headers(int length) {\n")
	i1Builder.WriteIndentedString("List<ColumnHeader> headers = new ArrayList<>();\n")
	//
	// Write register initialisers
	for _, col := range mod.Registers(schema.Width()) {
		reg := schema.Register(col.Register)
		// Check whether this is part of our module
		if reg.IsInputOutput() {
			bitWidth := fmt.Sprintf("%d", reg.Width())
			name := fmt.Sprintf("%s.%s", mod.Name, reg.Name())
			regStr := fmt.Sprintf("%d", col.Register.Index(schema.Width()))
			i1Builder.WriteIndentedString(
				"headers.add(new ColumnHeader(\"", name, "\",", regStr, ",", bitWidth, ",length));\n")
			//
			count++
		}
	}
	//
	i1Builder.WriteIndentedString("return headers;\n")
	builder.WriteIndentedString("}\n\n")
}

func generateJavaModuleHeader(builder indentBuilder) {
	builder.WriteIndentedString("private final BitSet filled = new BitSet();\n")
	builder.WriteIndentedString("private int currentLine = 0;\n\n")
}

func generateJavaModuleConstants(spillage uint, constants []corset.SourceConstant, builder indentBuilder) {
	for _, constant := range constants {
		var (
			javaType    string
			constructor string
			fieldName   string = strings.ReplaceAll(constant.Name, "-", "_")
		)
		// Determine suitable Java type
		if constant.Value.Sign() < 0 {
			// TODO: for now, we always skip negative constants since it is
			// entirely unclear how they should be interpreted.
			continue
		} else if constant.Bitwidth != math.MaxUint {
			constructor, javaType = translateJavaType(constant.Bitwidth, constant.Value)
		} else {
			constructor, javaType = inferJavaType(constant.Value)
		}
		//
		builder.WriteIndentedString("public static final ", javaType, " ", fieldName, " = ", constructor, ";\n")
	}
	//
	builder.WriteIndentedString("public static final int SPILLAGE = ", fmt.Sprintf("%d", spillage), ";\n")
	builder.WriteIndentedString("public int spillage() { return SPILLAGE; }\n")
}

func translateJavaType(bitwidth uint, value big.Int) (string, string) {
	var (
		constructor string
		javaType    string
	)
	//
	switch {
	case bitwidth < 32:
		// NOTE: cannot embed arbitrary unsigned 32bit constant into a Java
		// "int" because this type is signed.
		constructor = fmt.Sprintf("0x%s", value.Text(16))
		javaType = "int"
	case bitwidth < 64:
		// NOTE: cannot embed arbitrary unsigned 64bit constant into a Java
		// "long" because this type is signed.
		constructor = fmt.Sprintf("0x%sL", value.Text(16))
		javaType = "long"
	default:
		constructor = fmt.Sprintf("new BigInteger(\"%s\")", value.String())
		javaType = "BigInteger"
	}
	//
	return constructor, javaType
}

func inferJavaType(value big.Int) (string, string) {
	var (
		maxInt      = big.NewInt(math.MaxInt32)
		constructor string
		javaType    string
	)
	//
	switch {
	case value.Cmp(maxInt) <= 0:
		constructor = fmt.Sprintf("0x%s", value.Text(16))
		javaType = "int"
	case value.IsInt64():
		constructor = fmt.Sprintf("0x%sL", value.Text(16))
		javaType = "long"
	default:
		constructor = fmt.Sprintf("new BigInteger(\"%s\")", value.String())
		javaType = "BigInteger"
	}
	//
	return constructor, javaType
}

func generateJavaModuleRegisterFields[F any](mod corset.SourceModule, schema sc.AnySchema[F], builder indentBuilder) {
	// Count of created registers
	count := uint(0)
	// Write register initialisers
	for _, col := range mod.Registers(schema.Width()) {
		// Determine underlying register
		reg := schema.Register(col.Register)
		// Check whether this is part of our module
		if reg.IsInputOutput() {
			// Yes, include register
			if count == 0 {
				builder.WriteIndentedString(fmt.Sprintf("// Registers from %s\n", mod.Name))
			}
			// Determine suitable name for field
			fieldName := toRegisterName(col.Register, reg.Name())
			//
			builder.WriteIndentedString("private Column ", fieldName, ";\n")
			// increase count
			count++
		}
	}
}

func generateJavaModuleSubmoduleFields(submodules []corset.SourceModule, builder indentBuilder) {
	first := true
	//
	for _, m := range submodules {
		// Only consider public, non-virtual modules (for now)
		if m.Public && !m.Virtual {
			className := toPascalCase(m.Name)
			// Determine suitable name for field
			fieldName := toCamelCase(m.Name)
			// Start submodules section
			if first {
				builder.WriteIndentedString("// Submodules\n")
			}
			// Yes, it is.
			builder.WriteIndentedString("public final ", className, " ", fieldName, ";\n")
			builder.WriteIndentedString("public ", className, " ", fieldName, "() { return "+fieldName, "; }\n")
			//
			first = false
		}
	}
	//
	if !first {
		builder.WriteString("\n")
	}
}

func generateJavaModuleMetadata(metadata typed.Map, builder indentBuilder) {
	// Write field declaration
	builder.WriteIndentedString("private final Map<String,Object> metadata = new HashMap<>();\n")
	// Initialise map using Java static initialiser
	if !metadata.IsEmpty() {
		i1Builder := builder.Indent()
		builder.WriteIndentedString("{\n")
		i1Builder.WriteIndentedString("Map<String,String> constraints = new HashMap<>();\n")

		for _, k := range metadata.Keys() {
			val, ok := metadata.String(k)

			if !ok {
				// NOTE: for now, we don't support nested maps.  This could be added
				// relatively straightforwardly.
				panic("nested metadata not currently supported")
			}
			//
			i1Builder.WriteIndentedString("constraints.put(\"", k, "\",\"", val, "\");\n")
		}
		//
		i1Builder.WriteIndentedString("metadata.put(\"constraints\",constraints);\n")
		builder.WriteIndentedString("}\n\n")
	}

	builder.WriteIndentedString("public Map<String,Object> getMetaData() { return metadata; }\n")
}

func generateJavaModuleConstructor(classname string, mod corset.SourceModule, builder indentBuilder) {
	//
	innerBuilder := builder.Indent()
	//
	builder.WriteIndentedString("public ", classname, "() {\n")
	//
	innerBuilder.WriteIndentedString("// initialise submodule(s)\n")
	// Write submodule initialisers
	for _, m := range mod.Submodules {
		className := toPascalCase(m.Name)
		// Determine suitable name for field
		fieldName := toCamelCase(m.Name)
		// Only support non-virtual modules for now
		if m.Public && !m.Virtual {
			innerBuilder.WriteIndentedString("this.", fieldName, " = new ", className, "();\n")
		}
	}
	//
	builder.WriteIndentedString("}\n\n")
}

func generateJavaModuleOpen[F any](mod corset.SourceModule, schema sc.AnySchema[F], builder indentBuilder) {
	//
	innerBuilder := builder.Indent()
	//
	builder.WriteIndentedString("public void open(Column[] registers) {\n")
	innerBuilder.WriteIndentedString("// initialise register(s)\n")
	// Write register initialisers
	for _, col := range mod.Registers(schema.Width()) {
		reg := schema.Register(col.Register)
		// Check whether this is part of our module
		if reg.IsInputOutput() {
			// Yes, it is.
			fieldName := toRegisterName(col.Register, reg.Name())
			registerStr := fmt.Sprintf("%d", col.Register.Index(schema.Width()))
			innerBuilder.WriteIndentedString("this.", fieldName, " = registers[", registerStr, "];\n")
		}
	}
	//
	innerBuilder.WriteIndentedString("// initialise submodule(s)\n")
	// Write submodule initialisers
	for _, m := range mod.Submodules {
		// Determine suitable name for field
		fieldName := toCamelCase(m.Name)
		// Only support non-virtual modules for now
		if m.Public && !m.Virtual {
			innerBuilder.WriteIndentedString("this.", fieldName, "().open(registers);\n")
		}
	}
	//
	builder.WriteIndentedString("}\n\n")
}

func generateJavaModuleSize(builder indentBuilder) {
	innerBuilder := builder.Indent()
	//
	builder.WriteIndentedString("public int size() {\n")
	innerBuilder.WriteIndentedString("if(!filled.isEmpty()) {\n")
	innerBuilder.WriteIndentedString(
		"   throw new RuntimeException(\"Cannot measure a trace with a non-validated row.\");\n")
	innerBuilder.WriteIndentedString("}\n")
	innerBuilder.WriteIndentedString("\n")
	innerBuilder.WriteIndentedString("return this.currentLine;\n")
	builder.WriteIndentedString("}\n\n")
}

func generateJavaModuleColumnSetters[F any](className string, mod corset.SourceModule, schema sc.AnySchema[F],
	builder indentBuilder) {
	//
	for _, column := range mod.Columns {
		var methodName string = column.Name
		//
		if !column.Computed {
			if mod.Virtual {
				methodName = toCamelCase(fmt.Sprintf("p_%s_%s", mod.Name, methodName))
			}
			//
			generateJavaModuleColumnSetter(className, methodName, column, schema, builder)
		}
	}
}

func generateJavaModuleColumnSetter[F any](className string, methodName string, col corset.SourceColumn,
	schema sc.AnySchema[F], builder indentBuilder) {
	//
	methodName = toCamelCase(methodName)
	bitwidth := col.Bitwidth
	fieldName := toRegisterName(col.Register, schema.Register(col.Register).Name())
	//
	indexStr := fmt.Sprintf("%d", col.Register.Index(schema.Width()))
	typeStr := getJavaType(bitwidth)
	i1Builder := builder.Indent()
	i2Builder := i1Builder.Indent()
	//
	builder.WriteIndentedString("public ", className, " ", methodName, "(final ", typeStr, " val) {\n")
	i1Builder.WriteIndentedString("if(filled.get(", indexStr, ")) {\n")
	i2Builder.WriteIndentedString("throw new IllegalStateException(\"", col.Name, " already set\");\n")
	i1Builder.WriteIndentedString("} else {\n")
	i2Builder.WriteIndentedString("filled.set(", indexStr, ");\n")
	i1Builder.WriteIndentedString("}\n\n")
	//
	switch {
	case bitwidth == 1:
		i1Builder.WriteIndentedString(fieldName, ".write(val);\n")
	case bitwidth <= 8:
		i1Builder.WriteIndentedString(fieldName, ".write((byte) val);\n")
	case bitwidth <= 63:
		i1Builder.WriteIndentedString(fieldName, ".write(val);\n")
	default:
		i1Builder.WriteIndentedString(fieldName, ".write(val.trimLeadingZeros().toArray());\n")
	}
	//
	i1Builder.WriteIndentedString("\n")
	i1Builder.WriteIndentedString("return this;\n")
	// Done
	builder.WriteIndentedString("}\n\n")
	// Legacy case for bytes
	if bitwidth == 8 {
		generateJavaModuleLegacyColumnSetter(className, methodName, builder)
	}
}

// legacy setter to support UnsignedByte.
func generateJavaModuleLegacyColumnSetter(className string, methodName string, builder indentBuilder) {
	i1Builder := builder.Indent()
	builder.WriteIndentedString("public ", className, " ", methodName, "(final UnsignedByte val) {\n")
	i1Builder.WriteIndentedString("return ", methodName, "(val.toByte());\n")
	builder.WriteIndentedString("}\n\n")
}

func generateJavaModuleValidateRow[F any](className string, mod corset.SourceModule, schema sc.AnySchema[F],
	builder indentBuilder) {
	//
	i1Builder := builder.Indent()
	i2Builder := i1Builder.Indent()
	//
	builder.WriteIndentedString("public ", className, " validateRow() {\n")
	// Write register initialisers
	for _, col := range mod.Registers(schema.Width()) {
		reg := schema.Register(col.Register)
		// Check whether this is part of our module
		if reg.IsInputOutput() {
			name := fmt.Sprintf("%s.%s", mod.Name, reg.Name())
			regstr := fmt.Sprintf("%d", col.Register.Index(schema.Width()))
			// Yes, include register
			i1Builder.WriteIndentedString("if(!filled.get(", regstr, ")) {\n")
			i2Builder.WriteIndentedString("throw new IllegalStateException(\"", name, " has not been filled.\");\n")
			i1Builder.WriteIndentedString("}\n")
		}
	}
	//
	i1Builder.WriteIndentedString("this.filled.clear();\n")
	i1Builder.WriteIndentedString("this.currentLine++;\n")
	i1Builder.WriteIndentedString("return this;\n")
	builder.WriteIndentedString("}\n\n")
}

func generateJavaModuleFillAndValidateRow[F any](className string, mod corset.SourceModule, schema sc.AnySchema[F],
	builder indentBuilder) {
	//
	i1Builder := builder.Indent()
	i2Builder := i1Builder.Indent()
	//
	builder.WriteIndentedString("public ", className, " fillAndValidateRow() {\n")
	//
	for _, col := range mod.Registers(schema.Width()) {
		reg := schema.Register(col.Register)
		// Check whether this is part of our module
		if reg.IsInputOutput() {
			name := toRegisterName(col.Register, reg.Name())
			regstr := fmt.Sprintf("%d", col.Register.Index(schema.Width()))
			// Yes, include register
			i1Builder.WriteIndentedString("if(!filled.get(", regstr, ")) {\n")
			//
			switch {
			case reg.Width() == 1:
				i2Builder.WriteIndentedString(name, ".write(false);\n")
			case reg.Width() <= 63:
				i2Builder.WriteIndentedString(name, ".write(0L);\n")
			default:
				i2Builder.WriteIndentedString(name, ".write(new byte[0]);\n")
			}
			//
			i1Builder.WriteIndentedString("}\n")
		}
	}
	//
	i1Builder.WriteIndentedString("this.filled.clear();\n")
	i1Builder.WriteIndentedString("this.currentLine++;\n")
	i1Builder.WriteIndentedString("return this;\n")
	builder.WriteIndentedString("}\n\n")
}
