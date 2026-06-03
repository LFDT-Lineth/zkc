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
package term

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/schema/register"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	util_math "github.com/LFDT-Lineth/zkc/pkg/util/math"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/sexp"
)

// RegisterAccess represents reading the value held at a given register in the
// tabular context.  Furthermore, the current row maybe shifted up (or down) by
// a given amount. Suppose we are evaluating a constraint on row k=5 which
// contains the register accesses "STAMP(0)" and "CT(-1)".  Then, STAMP(0)
// accesses the STAMP register at row 5, whilst CT(-1) accesses the CT register at
// row 4.
type RegisterAccess[F field.Element[F], T Expr[F, T]] struct {
	// Id for register being accessed
	register register.Id
	// Bitwidth of register being accessed.  This can be MaxUint to signal it
	// has "field width".
	bitwidth uint
	// Mask determines what subset of bitwidth is actively used.
	maskwidth uint
	// Relative shift of access.  This indicates on which row (relative to the
	// current row) the given register is being read.
	shift int
}

// NewRegisterAccess constructs an AIR expression representing the value of a
// given register on the current row.
func NewRegisterAccess[F field.Element[F], T Expr[F, T]](register register.Id, bitwidth uint, shift int) T {
	var term Expr[F, T] = RawRegisterAccess[F, T](register, bitwidth, shift)
	return term.(T)
}

// RawRegisterAccess constructs an AIR expression representing the value of a given
// register on the current row.
func RawRegisterAccess[F field.Element[F], T Expr[F, T]](register register.Id, bitwidth uint, shift int,
) *RegisterAccess[F, T] {
	//
	return &RegisterAccess[F, T]{register, bitwidth, bitwidth, shift}
}

// FieldAccess constructs an AIR expression representing the value of a given
// register on the current row.  There is an assumption here that the register
// being read has "field type".  That is, it does not represent fixed width
// value in the usual sense.  Such registers should only occur lower down in the
// pipeling for e.g. handling inverses, etc.
func FieldAccess[F field.Element[F], T Expr[F, T]](register register.Id, shift int,
) *RegisterAccess[F, T] {
	//
	return &RegisterAccess[F, T]{register, math.MaxUint, math.MaxUint, shift}
}

// Air indicates this term can be used at the AIR level.
func (p *RegisterAccess[F, T]) Air() {}

// Register returns the id of the register being accessed.
func (p *RegisterAccess[F, T]) Register() register.Id {
	return p.register
}

// HasFieldType checks whether or not this register access is for a register
// which has "field width".  That is, it does not have a true bitwidth per se.
func (p *RegisterAccess[F, T]) HasFieldType() bool {
	return p.bitwidth == math.MaxUint
}

// Mask constructs a variation on this register access which only uses the
// "masked" portion of the given register.  For example, this can be used to
// implement a cast.
func (p *RegisterAccess[F, T]) Mask(maskwidth uint) *RegisterAccess[F, T] {
	// Sanity check mask
	if maskwidth > p.bitwidth {
		panic(fmt.Sprintf("invalid mask (u%d > u%d)", maskwidth, p.bitwidth))
	} else if p.HasFieldType() {
		panic("cannot mask a register of field type")
	}
	//
	return &RegisterAccess[F, T]{p.register, p.bitwidth, maskwidth, p.shift}
}

// MaskWidth returns the portion of the underlying column / register actually
// read by this access.  For example, given a register of type u16 we might only
// be accessing the first u8 portion.  In such case, the access is acting like a
// cast.
func (p *RegisterAccess[F, T]) MaskWidth() uint {
	return p.maskwidth
}

// BitWidth returns the declared bitwidth of the variable being accessed.
// Observe that the actual width of this access may be smaller than this if a
// mask is being applied.
func (p *RegisterAccess[F, T]) BitWidth() uint {
	return p.bitwidth
}

// RelativeShift returns the relative shift of this access.
func (p *RegisterAccess[F, T]) RelativeShift() int {
	return p.shift
}

// ApplyShift implementation for Term interface.
func (p *RegisterAccess[F, T]) ApplyShift(shift int) T {
	var reg Expr[F, T] = &RegisterAccess[F, T]{
		p.register, p.bitwidth, p.maskwidth, p.shift + shift,
	}
	//
	return reg.(T)
}

// Bounds implementation for Boundable interface.
func (p *RegisterAccess[F, T]) Bounds() util.Bounds {
	if p.shift >= 0 {
		// Positive shift
		return util.NewBounds(0, uint(p.shift))
	}
	// Negative shift
	return util.NewBounds(uint(-p.shift), 0)
}

// EvalAt implementation for Evaluable interface.
func (p *RegisterAccess[F, T]) EvalAt(k int, module trace.Module[F], _ register.Map) (F, error) {
	var (
		val = module.Column(p.register.Unwrap()).Get(k + p.shift)
		err error
	)
	// // Dynamic cast
	// if p.bitwidth != math.MaxUint && val.Cmp(p.bound) >= 0 {
	// 	// Construct error
	// 	err = fmt.Errorf("read failure (value %s not u%d)", val.String(), p.bitwidth)
	// }
	//
	return val, err
}

// IsDefined implementation for Evaluable interface.
func (p *RegisterAccess[F, T]) IsDefined() bool {
	return p.register.IsUsed()
}

// Lisp implementation for Lispifiable interface.
func (p *RegisterAccess[F, T]) Lisp(global bool, mapping register.Map) sexp.SExp {
	var name string
	// Generate name, whilst allowing for schema to be nil.
	if mapping != nil && global {
		name = mapping.Register(p.register).QualifiedName(mapping)
	} else if mapping != nil {
		name = mapping.Register(p.register).Name()
	} else {
		name = fmt.Sprintf("#%d", p.register)
	}
	//
	var access sexp.SExp = sexp.NewSymbol(name)
	// Check whether shifted (or not)
	if p.shift != 0 {
		// Shifted
		shift := sexp.NewSymbol(fmt.Sprintf("%d", p.shift))
		//
		access = sexp.NewList([]sexp.SExp{sexp.NewSymbol("shift"), access, shift})
	}
	//
	if p.maskwidth != p.bitwidth {
		tw := fmt.Sprintf("u%d", p.maskwidth)
		access = sexp.NewList([]sexp.SExp{sexp.NewSymbol(tw), access})
	}
	//
	return access
}

// RequiredRegisters implementation for Contextual interface.
func (p *RegisterAccess[F, T]) RequiredRegisters() *set.SortedSet[uint] {
	r := set.NewSortedSet[uint]()
	r.Insert(p.register.Unwrap())
	// Done
	return r
}

// RequiredCells implementation for Contextual interface
func (p *RegisterAccess[F, T]) RequiredCells(row int, mid trace.ModuleId) *set.AnySortedSet[trace.CellRef] {
	var (
		set = set.NewAnySortedSet[trace.CellRef]()
		ref = trace.NewColumnRef(mid, p.register)
	)
	//
	set.Insert(trace.NewCellRef(ref, row+p.shift))
	//
	return set
}

// ShiftRange implementation for Term interface.
func (p *RegisterAccess[F, T]) ShiftRange() (int, int) {
	return p.shift, p.shift
}

// Simplify implementation for Term interface.
func (p *RegisterAccess[F, T]) Simplify(casts bool) T {
	var tmp Expr[F, T] = p
	return tmp.(T)
}

// Substitute implementation for Substitutable interface.
func (p *RegisterAccess[F, T]) Substitute(mapping map[string]F) {

}

// ValueRange implementation for Term interface.
func (p *RegisterAccess[F, T]) ValueRange() util_math.Interval {
	// NOTE: the following is necessary because MaxUint is permitted as a signal
	// that the given register has no fixed bitwidth.  Rather, it can consume
	// all possible values of the underlying field element.
	if p.bitwidth == math.MaxUint {
		return util_math.INFINITY
	}
	//
	return valueRangeOfBits(p.maskwidth)
}

func valueRangeOfBits(bitwidth uint) util_math.Interval {
	var bound = big.NewInt(2)
	//
	bound.Exp(bound, big.NewInt(int64(bitwidth)), nil)
	// Subtract 1 because interval is inclusive.
	bound.Sub(bound, &biONE)
	// Done
	return util_math.NewInterval(biZERO, *bound)
}

// ============================================================================
// Encoding / Decoding
// ============================================================================

// MarshalBinary converts the RegisterAccess into a sequence of bytes.
func (p *RegisterAccess[F, T]) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	// Register Index
	if err := binary.Write(&buf, binary.BigEndian, uint16(p.register.Unwrap())); err != nil {
		return nil, err
	}
	// Bitwidth
	if err := binary.Write(&buf, binary.BigEndian, uint16(p.bitwidth)); err != nil {
		return nil, err
	}
	// Maskwidth
	if err := binary.Write(&buf, binary.BigEndian, uint16(p.maskwidth)); err != nil {
		return nil, err
	}
	// Shift
	if err := binary.Write(&buf, binary.BigEndian, int16(p.RelativeShift())); err != nil {
		return nil, err
	}
	//
	return buf.Bytes(), nil
}

// UnmarshalBinary initialises this RegisterAccess from a given set of data
// bytes. This should match exactly the encoding above.
func (p *RegisterAccess[F, T]) UnmarshalBinary(data []byte) error {
	return p.UnmarshalBuffer(bytes.NewBuffer(data))
}

// UnmarshalBuffer initialises this RegisterAccess from a given byte buffer.
// This should match exactly the encoding above.
func (p *RegisterAccess[F, T]) UnmarshalBuffer(buf *bytes.Buffer) error {
	var (
		index     uint16
		bitwidth  uint16
		maskwidth uint16
		shift     int16
	)
	// Register index
	if err := binary.Read(buf, binary.BigEndian, &index); err != nil {
		return err
	}
	// Register bitwidth
	if err := binary.Read(buf, binary.BigEndian, &bitwidth); err != nil {
		return err
	}
	// Register maskwidth
	if err := binary.Read(buf, binary.BigEndian, &maskwidth); err != nil {
		return err
	}
	// Register shift
	if err := binary.Read(buf, binary.BigEndian, &shift); err != nil {
		return err
	}
	// Normalise bitwidth
	var normBitwidth = uint(bitwidth)
	//
	if bitwidth == math.MaxUint16 {
		normBitwidth = math.MaxUint
	}
	// Construct new register access
	*p = RegisterAccess[F, T]{
		register.NewId(uint(index)),
		normBitwidth,
		uint(maskwidth),
		int(shift),
	}
	// Done
	return nil
}
