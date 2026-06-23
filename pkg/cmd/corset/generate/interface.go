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
	"path/filepath"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/binfile"
	cmd_util "github.com/LFDT-Lineth/zkc/pkg/cmd/corset/util"
	"github.com/LFDT-Lineth/zkc/pkg/corset"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// JavaTraceInterfaceUnion generates a suitable interface capturing the given schema,
// as outlined in the source map.
func JavaTraceInterfaceUnion[F field.Element[F]](filename string, pkgname string,
	stacks []cmd_util.SchemaStacker[F]) (string, error) {
	//
	return javaTraceInterface(filename, pkgname, true, stacks)
}

func javaTraceInterface[F field.Element[F]](filename string, pkgname string, union bool,
	stacks []cmd_util.SchemaStacker[F]) (string, error) {
	//
	var root corset.SourceModule
	// Combine roots to determine set of common functionality.
	for i, stack := range stacks {
		// Extract source map (which we assume is present)
		srcmap, _ := binfile.GetAttribute[*corset.SourceMap](stack.BinaryFile())
		//
		if i == 0 {
			root = srcmap.Root
		} else if union {
			root = *unionModules(root, srcmap.Root)
		} else {
			root = *intersectModules(root, srcmap.Root)
		}
	}
	// Finally, generate the interface
	return generateInterface(filename, pkgname, root)
}

func generateInterface(filename string, pkgname string, root corset.SourceModule) (string, error) {
	//
	var (
		builder strings.Builder
		// Extract base of filename
		basename = filepath.Base(filename)
	)
	// Sanity check a request is made to generate a java source file.
	if !strings.HasSuffix(basename, ".java") {
		return "", errors.New("invalid Java classname")
	}
	// Strip suffix to determine classname
	classname := strings.TrimSuffix(basename, ".java")
	// begin generation
	generateInterfaceHeader(pkgname, &builder)
	generateInterfaceContents(classname, root, indentBuilder{0, &builder})

	return builder.String(), nil
}

func generateInterfaceHeader(pkgname string, builder *strings.Builder) {
	builder.WriteString(license)
	// Write package line
	if pkgname != "" {
		fmt.Fprintf(builder, "package %s;\n", pkgname)
	}
	//
	builder.WriteString(javaImports)
	builder.WriteString(javaWarning)
	//
	builder.WriteString(" */\n")
}

func generateInterfaceContents(className string, mod corset.SourceModule, builder indentBuilder) {
	builder.WriteIndentedString("public interface ", className, " {\n")
	//
	generateInterfaceConstants(mod.Constants, builder.Indent())
	generateInterfaceSubmoduleAccessors(mod.Submodules, builder.Indent())
	generateInterfaceHeaders(builder.Indent())
	generateInterfaceColumnSetters(className, mod, builder.Indent())
	generateInterfaceValidateRow(className, builder.Indent())
	// Generate any submodules
	for _, submod := range mod.Submodules {
		if submod.Public && !submod.Virtual {
			generateInterfaceContents(toPascalCase(submod.Name), submod, builder.Indent())
		} else if submod.Public {
			generateInterfaceColumnSetters(className, submod, builder.Indent())
		}
	}
	//
	if mod.Name == "" {
		builder.WriteString(javaColumnHeader)
		builder.WriteString(javaColumn)
		builder.WriteString(javaAddMetadataSignature)
		builder.WriteString(javaOpenSignature)
	}
	//
	builder.WriteIndentedString("}\n")
}

func generateInterfaceSubmoduleAccessors(submodules []corset.SourceModule, builder indentBuilder) {
	first := true
	//
	for _, m := range submodules {
		// Only consider non-virtual, public modules (for now)
		if !m.Virtual && m.Public {
			className := toPascalCase(m.Name)
			// Determine suitable name for field
			fieldName := toCamelCase(m.Name)
			// Start submodules section
			if first {
				builder.WriteIndentedString("// Submodules\n")
			}
			// Yes, it is.
			builder.WriteIndentedString("default ", className, " ", fieldName, "() {")
			builder.WriteString(fmt.Sprintf("throw new IllegalArgumentException(\"missing module %s\"); }\n", m.Name))
			//
			first = false
		}
	}
	//
	if !first {
		builder.WriteString("\n")
	}
}

func generateInterfaceHeaders(builder indentBuilder) {
	builder.WriteIndentedString("List<ColumnHeader> headers(int length);\n")
}

func generateInterfaceConstants(constants []corset.SourceConstant, builder indentBuilder) {
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
		builder.WriteIndentedString("final ", javaType, " ", fieldName, " = ", constructor, ";\n")
	}
	//
	builder.WriteIndentedString("int spillage();\n")
}

func generateInterfaceColumnSetters(className string, mod corset.SourceModule,
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
			generateInterfaceColumnSetter(className, methodName, mod, column, builder)
		}
	}
}

func generateInterfaceColumnSetter(className string, methodName string, mod corset.SourceModule,
	col corset.SourceColumn, builder indentBuilder) {
	//
	handle := fmt.Sprintf("%s.%s", mod.Name, col.Name)
	methodName = toCamelCase(methodName)
	typeStr := getJavaType(col.Bitwidth)
	errorMsg := fmt.Sprintf("throw new IllegalArgumentException(\"missing column %s (u%d)\"); }\n", handle, col.Bitwidth)
	//
	builder.WriteIndentedString("default ", className, " ", methodName, "(final ", typeStr, " val) {")
	builder.WriteString(errorMsg)
	// Legacy case for bytes
	if col.Bitwidth == 8 {
		builder.WriteIndentedString("default ", className, " ", methodName, "(final UnsignedByte val) {")
		builder.WriteString(errorMsg)
	}
}

func generateInterfaceValidateRow(className string, builder indentBuilder) {
	//
	builder.WriteIndentedString(className, " validateRow();\n")
	builder.WriteIndentedString(className, " fillAndValidateRow();\n")
}
