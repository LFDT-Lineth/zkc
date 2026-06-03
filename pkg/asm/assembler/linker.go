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
package assembler

import (
	"fmt"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/asm/io"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/macro"
	"github.com/LFDT-Lineth/zkc/pkg/asm/io/macro/expr"
	"github.com/LFDT-Lineth/zkc/pkg/schema/module"
	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
)

// AssemblyComponent represents something declared within an assembly file, such
// as a function or constant, etc.
type AssemblyComponent interface {
	// Return name of this component
	Name() module.Name
}

// AssemblyConstant represents a named constant at the assembly level.
type AssemblyConstant struct {
	name     module.Name
	constant big.Int
	base     uint
}

// Name implementation for AssemblyComponent interface
func (p *AssemblyConstant) Name() module.Name {
	return p.name
}

// AssemblyItem represents an intermediate artifact producing during the
// assembly process.  The key is that, as this stage, the bus identifiers are
// not fully known.  Hence, when multiple assembly items come together we must
// "align buses appropriately between them.
type AssemblyItem struct {
	Includes []*string
	// Components making up this assembly item.
	Components []AssemblyComponent
	// Mapping of instructions back to the source file.
	SourceMap source.Map[any]
}

// ConstMap is a convenient alias
type ConstMap map[module.Name]util.Pair[big.Int, uint]

// Link a set of one or more assembly items together to produce a complete
// program, or one or more errors.  Linking is the process of connecting buses
// which are used (e.g. by a call instruction) with their definitions (e.g. a
// function declaration).
func Link(items ...AssemblyItem) ([]MacroComponent, source.Maps[any], []source.SyntaxError) {
	var (
		linker = NewLinker()
		errors []source.SyntaxError
	)
	// Constuct bus and source mappings
	for _, item := range items {
		linker.Join(item.SourceMap)
		//
		for _, component := range item.Components {
			// Check whether component of same name already exists.
			if linker.Exists(component.Name()) {
				// Indicates component of same name already exists.  It would be
				// good to report a source error here, but the problem is that
				// our source map doesn't contain the right information.
				msg := fmt.Sprintf("duplicate component %s", component.Name())
				errors = append(errors, *linker.srcmap.SyntaxError(component, msg))
			} else {
				linker.Register(component)
			}
		}
	}
	// Link all assembly items
	if len(errors) == 0 {
		errors = linker.Link()
	}
	//
	return linker.components, linker.srcmap, errors
}

// Linker packages together the various bits of information required for linking
// the assembly files.
type Linker struct {
	srcmap     source.Maps[any]
	busmap     map[module.Name]uint
	constmap   ConstMap
	components []MacroComponent
	names      map[module.Name]bool
}

// NewLinker constructs a new linker
func NewLinker() *Linker {
	return &Linker{
		srcmap:     *source.NewSourceMaps[any](),
		busmap:     make(map[module.Name]uint),
		constmap:   make(ConstMap),
		components: nil,
		names:      make(map[module.Name]bool),
	}
}

// Exists checks whether or not a component of the given name already exists.
func (p *Linker) Exists(name module.Name) bool {
	_, ok := p.names[name]
	//
	return ok
}

// Join a source map into this linker
func (p *Linker) Join(srcmap source.Map[any]) {
	p.srcmap.Join(&srcmap)
}

// Register a new components with this linker.
func (p *Linker) Register(component AssemblyComponent) {
	// First, record name
	p.names[component.Name()] = true
	// Second, act on component type
	switch c := component.(type) {
	case *AssemblyConstant:
		p.constmap[c.Name()] = util.NewPair(c.constant, c.base)
	case *MacroFunction:
		// Allocate bus entry
		p.busmap[c.Name()] = uint(len(p.busmap))
		//
		p.components = append(p.components, c)
	default:
		// Should be unreachable
		panic(fmt.Sprintf("unknown component %s", component.Name()))
	}
}

// Link all components register with this linker
func (p *Linker) Link() []source.SyntaxError {
	var errors []source.SyntaxError
	//
	for index := range p.components {
		errs := p.linkComponent(uint(index))
		errors = append(errors, errs...)
	}
	//
	return errors
}

// Link all buses used within this function to their intended targets.  This
// means, for every bus used locally, settings the global bus identifier and
// also allocated regisers for the address/data lines.
func (p *Linker) linkComponent(index uint) []source.SyntaxError {
	switch p.components[index].(type) {
	case *MacroFunction:
		return p.linkFunction(index)
	default:
		panic("unknown component")
	}
}

// Link all buses used within this function to their intended targets.  This
// means, for every bus used locally, settings the global bus identifier and
// also allocated regisers for the address/data lines.
func (p *Linker) linkFunction(index uint) []source.SyntaxError {
	// Mapping of bus names to allocated buses
	var (
		fn         = p.components[index].(*MacroFunction)
		buses      = fn.Buses()
		code       = fn.Code()
		localBuses = make(map[uint]io.Bus, 0)
		errors     []source.SyntaxError
	)
	// Allocate buses
	for i, bus := range buses {
		busId := p.busmap[bus.Name]
		// Allocate bus
		buses[i] = allocateBus(busId, localBuses, index, p.components)
	}
	//
	for i := range code {
		if err := p.linkInstruction(code[i], localBuses); err != nil {
			errors = append(errors, *err)
		}
	}
	//
	return errors
}

func (p *Linker) linkInstruction(insn macro.Instruction, buses map[uint]io.Bus) *source.SyntaxError {
	switch insn := insn.(type) {
	case *macro.Assign:
		return p.linkExpr(insn.Source)
	case *macro.Division:
		return p.linkExprs(insn.Dividend, insn.Divisor)
	case *macro.Call:
		// Determine global bus identifier
		busId, ok := p.busmap[insn.Bus().Name]
		// sanity check
		if !ok {
			msg := fmt.Sprintf("unknown function \"%s\"", insn.Bus().Name)
			return p.srcmap.SyntaxError(insn, msg)
		}
		// allocate & link bus
		insn.Link(buses[busId])
		//
		return p.linkExprs(insn.Sources...)
	case *macro.IfGoto:
		return p.linkExprs(insn.Left, insn.Right)
	case *macro.IfThenElse:
		return p.linkExprs(insn.Left, insn.Right, insn.Then, insn.Else)
	default:
		// continue
	}
	//
	return nil
}

func (p *Linker) linkExpr(e macro.Expr) *source.SyntaxError {
	switch e := e.(type) {
	case *expr.Add:
		return p.linkExprs(e.Exprs...)
	case *expr.Const:
		if e.Label != "" {
			deats, ok := p.constmap[module.NewName(e.Label, 1)]
			//
			if !ok {
				return p.srcmap.SyntaxError(e, "unknown register or constant")
			}
			//
			e.Base = deats.Right
			e.Constant = deats.Left
		}
	case *expr.Mul:
		return p.linkExprs(e.Exprs...)
	case *expr.RegAccess:
		// Nothing to do
	case *expr.Sub:
		return p.linkExprs(e.Exprs...)
	default:
		panic("unreachable")
	}
	//
	return nil
}

func (p *Linker) linkExprs(es ...macro.Expr) *source.SyntaxError {
	for _, e := range es {
		if err := p.linkExpr(e); err != nil {
			return err
		}
	}
	//
	return nil
}

// Get the local bus declared for the given function, either by allocating a new
// bus (if was not already allocated) or returning the existing bus (if it was
// previously allocated).  Allocating a new bus requires allocating
// corresponding I/O registers within the given function.
func allocateBus(busId uint, localBuses map[uint]io.Bus, index uint, components []MacroComponent) io.Bus {
	//
	var (
		fn      = components[index].(*MacroFunction)
		busName = components[busId].Name()
		inputs  = components[busId].Inputs()
		outputs = components[busId].Outputs()
	)
	// Create new bus.
	addressLines := allocateIoRegisters(busName, inputs, fn)
	dataLines := allocateIoRegisters(busName, outputs, fn)
	bus := io.NewBus(busName, busId, addressLines, dataLines)
	// Update local bus map
	localBuses[busId] = bus
	//
	return bus
}

func allocateIoRegisters(busName module.Name, registers []io.Register, fn *MacroFunction) []io.RegisterId {
	//
	var lines []io.RegisterId
	//
	for _, reg := range registers {
		var regName string
		// Determine suitable register name
		if reg.IsInput() {
			regName = fmt.Sprintf("%s>%s[%s]", fn.Name().String(), busName.String(), reg.Name())
		} else if reg.IsOutput() {
			regName = fmt.Sprintf("%s<%s[%s]", fn.Name().String(), busName.String(), reg.Name())
		} else {
			panic("unreachable")
		}
		// Allocate register
		lines = append(lines, fn.AllocateRegister(register.COMPUTED_REGISTER, regName, reg.Width(), *reg.Padding()))
	}
	//
	return lines
}
