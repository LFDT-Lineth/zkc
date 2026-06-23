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
package hir

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"reflect"

	"github.com/LFDT-Lineth/zkc/pkg/ir/term"
	"github.com/LFDT-Lineth/zkc/pkg/schema"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint"
	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/lookup"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

const (
	// Constraints
	assertionTag        = byte(0)
	interleavingTag     = byte(1)
	lookupTag           = byte(2)
	permutationTag      = byte(3)
	rangeTag            = byte(4)
	sortedUnfilteredTag = byte(5)
	sortedFilteredTag   = byte(6)
	vanishingTag        = byte(7)
	fnCallTag           = byte(8)
	fnCondCallTag       = byte(9)
	// Logicals
	conjunctTag   = byte(10)
	disjunctTag   = byte(11)
	equalTag      = byte(12)
	notEqualTag   = byte(13)
	lessThanTag   = byte(14)
	lessThanEqTag = byte(15)
	negationTag   = byte(16)
	iteTagTF      = byte(17)
	iteTagT       = byte(18)
	iteTagF       = byte(19)
	// Expressions
	addTag              = byte(30)
	castTag             = byte(31)
	constantTag         = byte(32)
	ifZeroTag           = byte(33)
	labelledConstantTag = byte(34)
	registerAccessTag   = byte(35)
	expTag              = byte(36)
	mulTag              = byte(37)
	normTag             = byte(38)
	subTag              = byte(39)
	vectorAccessTag     = byte(40)
)

func encode_constraint(constraint schema.Constraint[word.BigEndian]) ([]byte, error) {
	switch c := constraint.(type) {
	case Assertion:
		return encode_assertion(c)
	case FunctionCall:
		return encode_fncall(c)
	case InterleavingConstraint:
		return encode_interleaving(c)
	case LookupConstraint:
		return encode_lookup(c)
	case PermutationConstraint:
		return encode_permutation(c)
	case RangeConstraint:
		return encode_range(c)
	case SortedConstraint:
		return encode_sorted(c)
	case VanishingConstraint:
		return encode_vanishing(c)
	default:
		name := reflect.TypeOf(constraint).String()
		return nil, fmt.Errorf("unknown constraint: %s (%s)", name, constraint.Name())
	}
}

func encode_assertion(c Assertion) ([]byte, error) {
	var (
		buffer     bytes.Buffer
		gobEncoder = gob.NewEncoder(&buffer)
	)
	// Tag
	if _, err := buffer.Write([]byte{assertionTag}); err != nil {
		return nil, err
	}
	// Handle
	if err := gobEncoder.Encode(c.Handle); err != nil {
		return nil, err
	}
	// Context
	if err := gobEncoder.Encode(c.Context); err != nil {
		return nil, err
	}
	// Domain
	if err := gobEncoder.Encode(&c.Domain); err != nil {
		return nil, err
	}
	// Constraint
	err := encode_logical[constraint.Property, term.Computation[word.BigEndian]](c.Property, &buffer)
	// Done
	return buffer.Bytes(), err
}

func encode_fncall(c FunctionCall) ([]byte, error) {
	var (
		buffer     bytes.Buffer
		gobEncoder      = gob.NewEncoder(&buffer)
		tag        byte = fnCallTag
		err        error
	)
	//
	if c.Selector.HasValue() {
		tag = fnCondCallTag
	}
	// Tag
	if _, err := buffer.Write([]byte{tag}); err != nil {
		return nil, err
	}
	// Handle
	if err = gobEncoder.Encode(c.Handle); err != nil {
		return nil, err
	}
	// Function caller
	if err := gobEncoder.Encode(c.Caller); err != nil {
		return nil, err
	}
	// Function callee
	if err := gobEncoder.Encode(c.Callee); err != nil {
		return nil, err
	}
	// Returns
	if err = encode_nary(encode_term[LogicalTerm, Term], &buffer, c.Returns); err != nil {
		return nil, err
	}
	// Arguments
	if err = encode_nary(encode_term[LogicalTerm, Term], &buffer, c.Arguments); err != nil {
		return nil, err
	}
	// Selector (Optional)
	if c.Selector.HasValue() {
		err = encode_logical[LogicalTerm, Term](c.Selector.Unwrap(), &buffer)
	}
	//
	return buffer.Bytes(), err
}

func encode_interleaving(c InterleavingConstraint) ([]byte, error) {
	var (
		buffer     bytes.Buffer
		gobEncoder = gob.NewEncoder(&buffer)
	)
	// Tag
	if _, err := buffer.Write([]byte{interleavingTag}); err != nil {
		return nil, err
	}
	// Handle
	if err := gobEncoder.Encode(c.Handle); err != nil {
		return nil, err
	}
	// Target Context
	if err := gobEncoder.Encode(c.TargetContext); err != nil {
		return nil, err
	}
	// Target term
	if err := encode_term[LogicalTerm, Term](c.Target, &buffer); err != nil {
		return nil, err
	}
	// Source Context
	if err := gobEncoder.Encode(c.SourceContext); err != nil {
		return nil, err
	}
	// Source terms
	if err := encode_nary(encode_term[LogicalTerm, Term], &buffer, c.Sources); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

func encode_lookup(c LookupConstraint) ([]byte, error) {
	var (
		buffer     bytes.Buffer
		gobEncoder = gob.NewEncoder(&buffer)
	)
	// Tag
	if _, err := buffer.Write([]byte{lookupTag}); err != nil {
		return nil, err
	}
	// Handle
	if err := gobEncoder.Encode(c.Handle); err != nil {
		return nil, err
	}
	// Target terms
	if err := encode_nary(encode_lookup_vector, &buffer, c.Targets); err != nil {
		return nil, err
	}
	// Sources
	if err := encode_nary(encode_lookup_vector, &buffer, c.Sources); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

func encode_lookup_vector(vector lookup.Vector[word.BigEndian, Term], buffer *bytes.Buffer) error {
	var (
		gobEncoder = gob.NewEncoder(buffer)
		selector   = vector.HasSelector()
	)
	// Source Context
	if err := gobEncoder.Encode(vector.Module); err != nil {
		return err
	}
	// HasSelector flag
	if err := gobEncoder.Encode(selector); err != nil {
		return err
	}
	// Selector itself (if applicable)
	if selector {
		if err := encode_term[LogicalTerm, Term](vector.Selector.Unwrap(), buffer); err != nil {
			return err
		}
	}
	// Source terms
	return encode_nary(encode_term[LogicalTerm, Term], buffer, vector.Terms)
}

func encode_permutation(c PermutationConstraint) ([]byte, error) {
	var (
		buffer     bytes.Buffer
		gobEncoder = gob.NewEncoder(&buffer)
	)
	// Tag
	if _, err := buffer.Write([]byte{permutationTag}); err != nil {
		return nil, err
	}
	// Handle
	if err := gobEncoder.Encode(c.Handle); err != nil {
		return nil, err
	}
	// Column Context
	if err := gobEncoder.Encode(c.Context); err != nil {
		return nil, err
	}
	// Target terms
	if err := gobEncoder.Encode(c.Targets); err != nil {
		return nil, err
	}
	// Source terms
	if err := gobEncoder.Encode(c.Sources); err != nil {
		return nil, err
	}
	//
	return buffer.Bytes(), nil
}

func encode_sorted(c SortedConstraint) ([]byte, error) {
	var (
		buffer     bytes.Buffer
		gobEncoder = gob.NewEncoder(&buffer)
		tag        byte
	)
	//
	if c.Selector.HasValue() {
		tag = sortedFilteredTag
	} else {
		tag = sortedUnfilteredTag
	}
	// Tag
	if _, err := buffer.Write([]byte{tag}); err != nil {
		return nil, err
	}
	// Handle
	if err := gobEncoder.Encode(c.Handle); err != nil {
		return nil, err
	}
	// Context
	if err := gobEncoder.Encode(c.Context); err != nil {
		return nil, err
	}
	// Bitwidth
	if err := gobEncoder.Encode(c.BitWidth); err != nil {
		return nil, err
	}
	// Signs
	if err := gobEncoder.Encode(c.Signs); err != nil {
		return nil, err
	}
	// Strict
	if err := gobEncoder.Encode(c.Strict); err != nil {
		return nil, err
	}
	// Optional Selector
	if c.Selector.HasValue() {
		// Constraint
		if err := encode_term[LogicalTerm, Term](c.Selector.Unwrap(), &buffer); err != nil {
			return nil, err
		}
	}
	// Sources
	err := encode_nary(encode_term[LogicalTerm, Term], &buffer, c.Sources)
	//
	return buffer.Bytes(), err
}

func encode_vanishing(c VanishingConstraint) ([]byte, error) {
	var (
		buffer     bytes.Buffer
		gobEncoder = gob.NewEncoder(&buffer)
	)
	// Tag
	if _, err := buffer.Write([]byte{vanishingTag}); err != nil {
		return nil, err
	}
	// Handle
	if err := gobEncoder.Encode(c.Handle); err != nil {
		return nil, err
	}
	// Context
	if err := gobEncoder.Encode(c.Context); err != nil {
		return nil, err
	}
	// Domain
	if err := gobEncoder.Encode(&c.Domain); err != nil {
		return nil, err
	}
	// Constraint
	err := encode_logical[LogicalTerm, Term](c.Constraint, &buffer)
	// Done
	return buffer.Bytes(), err
}

func encode_range(c RangeConstraint) ([]byte, error) {
	var (
		buffer     bytes.Buffer
		gobEncoder = gob.NewEncoder(&buffer)
	)
	//
	buffer.Write([]byte{rangeTag})
	// Handle
	if err := gobEncoder.Encode(c.Handle); err != nil {
		return nil, err
	}
	// Context
	if err := gobEncoder.Encode(c.Context); err != nil {
		return nil, err
	}
	// Bitwidth
	if err := gobEncoder.Encode(c.Bitwidths); err != nil {
		return nil, err
	}
	// Expression
	err := encode_nary(encode_term[LogicalTerm, Term], &buffer, c.Sources)
	// Done
	return buffer.Bytes(), err
}

func decode_constraint(bytes []byte) (schema.Constraint[word.BigEndian], error) {
	switch bytes[0] {
	case assertionTag:
		return decode_assertion(bytes[1:])
	case fnCallTag, fnCondCallTag:
		return decode_fncall(bytes[0] == fnCondCallTag, bytes[1:])
	case interleavingTag:
		return decode_interleaving(bytes[1:])
	case lookupTag:
		return decode_lookup(bytes[1:])
	case permutationTag:
		return decode_permutation(bytes[1:])
	case rangeTag:
		return decode_range(bytes[1:])
	case sortedUnfilteredTag:
		return decode_sorted(false, bytes[1:])
	case sortedFilteredTag:
		return decode_sorted(true, bytes[1:])
	case vanishingTag:
		return decode_vanishing(bytes[1:])
	default:
		return nil, fmt.Errorf("unknown constraint (tag %d)", bytes[0])
	}
}

func decode_assertion(data []byte) (schema.Constraint[word.BigEndian], error) {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
		property   LogicalTerm
		assertion  Assertion
		err        error
	)
	// Handle
	if err = gobDecoder.Decode(&assertion.Handle); err != nil {
		return assertion, err
	}
	// Context
	if err = gobDecoder.Decode(&assertion.Context); err != nil {
		return assertion, err
	}
	// Domain
	if err = gobDecoder.Decode(&assertion.Domain); err != nil {
		return assertion, err
	}
	//
	property, err = decode_logical(buffer)
	//
	if err == nil {
		// NOTE: the following will eventually need to be replaced if/when
		// computations diverge from terms.  For example, computations support a
		// more diverse set of operations than arithmetic terms.
		assertion.Property = term.NewLogicalComputation[word.BigEndian, LogicalTerm, Term](property)
	}
	// Success!
	return assertion, err
}

func decode_fncall(conditional bool, data []byte) (schema.Constraint[word.BigEndian], error) {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
		call       FunctionCall
		err        error
	)
	// Handle
	if err = gobDecoder.Decode(&call.Handle); err != nil {
		return call, err
	}
	// Caller
	if err = gobDecoder.Decode(&call.Caller); err != nil {
		return call, err
	}
	// Callee
	if err = gobDecoder.Decode(&call.Callee); err != nil {
		return call, err
	}
	// Returns
	if call.Returns, err = decode_nary(decode_term, buffer); err != nil {
		return call, err
	}
	// Arguments
	if call.Arguments, err = decode_nary(decode_term, buffer); err != nil {
		return call, err
	}
	//
	if conditional {
		var selector LogicalTerm
		//
		if selector, err = decode_logical(buffer); err != nil {
			return call, err
		}
		// Wrap selector
		call.Selector = util.Some(selector)
	} else {
		call.Selector = util.None[LogicalTerm]()
	}
	//
	return call, nil
}

func decode_interleaving(data []byte) (schema.Constraint[word.BigEndian], error) {
	var (
		buffer       = bytes.NewBuffer(data)
		gobDecoder   = gob.NewDecoder(buffer)
		interleaving InterleavingConstraint
		err          error
	)
	// Handle
	if err = gobDecoder.Decode(&interleaving.Handle); err != nil {
		return interleaving, err
	}
	// Target Context
	if err = gobDecoder.Decode(&interleaving.TargetContext); err != nil {
		return interleaving, err
	}
	// Targets
	if interleaving.Target, err = decode_term(buffer); err != nil {
		return interleaving, err
	}
	// Source Context
	if err = gobDecoder.Decode(&interleaving.SourceContext); err != nil {
		return interleaving, err
	}
	// Sources
	if interleaving.Sources, err = decode_nary(decode_term, buffer); err != nil {
		return interleaving, err
	}
	//
	return interleaving, nil
}

func decode_lookup(data []byte) (schema.Constraint[word.BigEndian], error) {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
		lookup     LookupConstraint
		err        error
	)
	// Handle
	if err = gobDecoder.Decode(&lookup.Handle); err != nil {
		return lookup, err
	}
	// Targets
	if lookup.Targets, err = decode_nary(decode_lookup_vector, buffer); err != nil {
		return lookup, err
	}
	// Sources
	if lookup.Sources, err = decode_nary(decode_lookup_vector, buffer); err != nil {
		return lookup, err
	}
	//
	return lookup, nil
}

func decode_lookup_vector(buf *bytes.Buffer) (lookup.Vector[word.BigEndian, Term], error) {
	var (
		gobDecoder  = gob.NewDecoder(buf)
		vector      lookup.Vector[word.BigEndian, Term]
		hasSelector bool
		selector    Term
		err         error
	)
	// Context
	if err = gobDecoder.Decode(&vector.Module); err != nil {
		return vector, err
	}
	// HasSelector
	if err = gobDecoder.Decode(&hasSelector); err != nil {
		return vector, err
	}
	// Selector (if applicable)
	if hasSelector {
		if selector, err = decode_term(buf); err != nil {
			return vector, err
		}
		// Wrap selector
		vector.Selector = util.Some(selector)
	}
	// Contents
	if vector.Terms, err = decode_nary(decode_term, buf); err != nil {
		return vector, err
	}
	// Done
	return vector, nil
}

func decode_permutation(data []byte) (schema.Constraint[word.BigEndian], error) {
	var (
		buffer      = bytes.NewBuffer(data)
		gobDecoder  = gob.NewDecoder(buffer)
		permutation PermutationConstraint
		err         error
	)
	// Handle
	if err = gobDecoder.Decode(&permutation.Handle); err != nil {
		return permutation, err
	}
	// Column Context
	if err = gobDecoder.Decode(&permutation.Context); err != nil {
		return permutation, err
	}
	// Target terms
	if err = gobDecoder.Decode(&permutation.Targets); err != nil {
		return permutation, err
	}
	// Source terms
	if err = gobDecoder.Decode(&permutation.Sources); err != nil {
		return permutation, err
	}
	//
	return permutation, nil
}

func decode_sorted(selector bool, data []byte) (schema.Constraint[word.BigEndian], error) {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
		sorted     SortedConstraint
		err        error
	)
	// Handle
	if err = gobDecoder.Decode(&sorted.Handle); err != nil {
		return nil, err
	}
	// Context
	if err = gobDecoder.Decode(&sorted.Context); err != nil {
		return nil, err
	}
	// Bitwidth
	if err := gobDecoder.Decode(&sorted.BitWidth); err != nil {
		return nil, err
	}
	// Signs
	if err := gobDecoder.Decode(&sorted.Signs); err != nil {
		return nil, err
	}
	// Strict
	if err := gobDecoder.Decode(&sorted.Strict); err != nil {
		return nil, err
	}
	// Optional Selector
	if selector {
		var term Term
		//
		if term, err = decode_term(buffer); err != nil {
			return nil, err
		}
		//
		sorted.Selector = util.Some(term)
	}
	// Sources
	sorted.Sources, err = decode_nary(decode_term, buffer)
	// Done
	return sorted, err
}

func decode_range(data []byte) (schema.Constraint[word.BigEndian], error) {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
		constraint RangeConstraint
		err        error
	)
	// Handle
	if err = gobDecoder.Decode(&constraint.Handle); err != nil {
		return constraint, err
	}
	// Context
	if err = gobDecoder.Decode(&constraint.Context); err != nil {
		return constraint, err
	}
	// Bitwidth
	if err = gobDecoder.Decode(&constraint.Bitwidths); err != nil {
		return constraint, err
	}
	//
	constraint.Sources, err = decode_nary(decode_term, buffer)
	// Success!
	return constraint, err
}

func decode_vanishing(data []byte) (schema.Constraint[word.BigEndian], error) {
	var (
		buffer     = bytes.NewBuffer(data)
		gobDecoder = gob.NewDecoder(buffer)
		vanishing  VanishingConstraint
		err        error
	)
	// Handle
	if err = gobDecoder.Decode(&vanishing.Handle); err != nil {
		return vanishing, err
	}
	// Context
	if err = gobDecoder.Decode(&vanishing.Context); err != nil {
		return vanishing, err
	}
	// Domain
	if err = gobDecoder.Decode(&vanishing.Domain); err != nil {
		return vanishing, err
	}
	//
	vanishing.Constraint, err = decode_logical(buffer)
	// Success!
	return vanishing, err
}

// ============================================================================
// Logical Terms (encoding)
// ============================================================================

// Logical provides a convenient shorthand
type Logical[T any] = term.Logical[word.BigEndian, T]

// Expr provides a convenient shorthand
type Expr[T any] = term.Expr[word.BigEndian, T]

func encode_logical[S Logical[S], T Expr[T]](t S, buf *bytes.Buffer) error {
	var x = any(t).(Logical[S])
	//
	switch t := x.(type) {
	case *term.Conjunct[word.BigEndian, S]:
		return encode_tagged_nary_logicals[S, T](conjunctTag, buf, t.Args...)
	case *term.Disjunct[word.BigEndian, S]:
		return encode_tagged_nary_logicals[S, T](disjunctTag, buf, t.Args...)
	case *term.Equal[word.BigEndian, S, T]:
		return encode_tagged_terms[S, T](equalTag, buf, t.Lhs.(T), t.Rhs.(T))
	case *term.Ite[word.BigEndian, S]:
		return encode_ite[S, T](t, buf)
	case *term.Negate[word.BigEndian, S]:
		return encode_tagged_logicals[S, T](negationTag, buf, t.Arg)
	case *term.NotEqual[word.BigEndian, S, T]:
		return encode_tagged_terms[S, T](notEqualTag, buf, t.Lhs.(T), t.Rhs.(T))
	default:
		return fmt.Errorf("encoding unknown logical term (%s)", x.Lisp(false, nil).String(false))
	}
}

func encode_tagged_nary_logicals[S Logical[S], T Expr[T]](tag byte, buf *bytes.Buffer, terms ...S) error {
	// Write tag
	if err := buf.WriteByte(tag); err != nil {
		return err
	}
	//
	return encode_nary(encode_logical[S, T], buf, terms)
}

func encode_tagged_logicals[S Logical[S], T Expr[T]](tag byte, buf *bytes.Buffer, terms ...S) error {
	// Write tag
	if err := buf.WriteByte(tag); err != nil {
		return err
	}
	//
	return encode_n(encode_logical[S, T], buf, terms...)
}

func encode_ite[S Logical[S], T Expr[T]](term *term.Ite[word.BigEndian, S], buf *bytes.Buffer) error {
	//
	switch {
	case term.FalseBranch != nil && term.TrueBranch != nil:
		return encode_tagged_logicals[S, T](iteTagTF, buf, term.Condition, term.TrueBranch.(S), term.FalseBranch.(S))
	case term.FalseBranch == nil:
		return encode_tagged_logicals[S, T](iteTagT, buf, term.Condition, term.TrueBranch.(S))
	case term.TrueBranch == nil:
		return encode_tagged_logicals[S, T](iteTagF, buf, term.Condition, term.FalseBranch.(S))
	default:
		panic("unreachable")
	}
}

// ============================================================================
// Logical Terms (decoding)
// ============================================================================

func decode_logical(buf *bytes.Buffer) (LogicalTerm, error) {
	tag, err := buf.ReadByte()
	//
	if err != nil {
		return nil, err
	}
	//
	switch tag {
	case conjunctTag:
		return decode_nary_logicals(conjunctionConstructor, buf)
	case disjunctTag:
		return decode_nary_logicals(disjunctionConstructor, buf)
	case equalTag:
		return decode_terms(2, equalConstructor, buf)
	case iteTagTF, iteTagT, iteTagF:
		return decode_ite(tag, buf)
	case negationTag:
		return decode_logicals(1, negationConstructor, buf)
	case notEqualTag:
		return decode_terms(2, notEqualConstructor, buf)
	default:
		return nil, fmt.Errorf("unknown constraint (tag %d)", tag)
	}
}

// Decode a variable number of terms, as determined by the leading byte.
func decode_nary_logicals(constructor func([]LogicalTerm) LogicalTerm, buf *bytes.Buffer,
) (LogicalTerm, error) {
	//
	terms, err := decode_nary(decode_logical, buf)
	return constructor(terms), err
}

// Decode exactly n logicals terms
func decode_logicals[S any](n uint, constructor func([]LogicalTerm) S, buf *bytes.Buffer,
) (S, error) {
	//
	terms, err := decode_n(n, decode_logical, buf)
	return constructor(terms), err
}

func decode_ite(tag byte, buf *bytes.Buffer) (LogicalTerm, error) {
	//
	switch tag {
	case iteTagTF:
		return decode_logicals(3, iteTrueFalseConstructor, buf)
	case iteTagT:
		return decode_logicals(2, iteTrueConstructor, buf)
	case iteTagF:
		return decode_logicals(2, iteFalseConstructor, buf)
	default:
		panic("unreachable")
	}
}

// ============================================================================
// Arithmetic Terms (encoding)
// ============================================================================

func encode_term[S Logical[S], T Expr[T]](t T, buf *bytes.Buffer) error {
	var x = any(t).(Expr[T])
	//
	switch t := x.(type) {
	case *term.Add[word.BigEndian, T]:
		return encode_tagged_nary_terms[S, T](addTag, buf, t.Args...)
	case *term.Cast[word.BigEndian, T]:
		return encode_cast[S, T](*t, buf)
	case *term.Constant[word.BigEndian, T]:
		return encode_constant[T](*t, buf)
	case *term.Exp[word.BigEndian, T]:
		return encode_exponent[S, T](*t, buf)
	case *term.IfZero[word.BigEndian, S, T]:
		return encode_ifZero[S, T](*t, buf)
	case *term.LabelledConst[word.BigEndian, T]:
		return encode_labelled_constant[T](*t, buf)
	case *term.Mul[word.BigEndian, T]:
		return encode_tagged_nary_terms[S, T](mulTag, buf, t.Args...)
	case *term.Norm[word.BigEndian, T]:
		return encode_tagged_terms[S, T](normTag, buf, t.Arg)
	case *term.RegisterAccess[word.BigEndian, T]:
		return encode_reg_access(*t, buf)
	case *term.Sub[word.BigEndian, T]:
		return encode_tagged_nary_terms[S, T](subTag, buf, t.Args...)
	case *term.VectorAccess[word.BigEndian, T]:
		return encode_vec_access(*t, buf)
	default:
		return fmt.Errorf("unknown arithmetic term encountered (%s)", x.Lisp(false, nil).String(false))
	}
}

func encode_tagged_nary_terms[S Logical[S], T Expr[T]](tag byte, buf *bytes.Buffer, terms ...T) error {
	// Write tag
	if err := buf.WriteByte(tag); err != nil {
		return err
	}
	//
	return encode_nary(encode_term[S, T], buf, terms)
}

func encode_tagged_terms[S Logical[S], T Expr[T]](tag byte, buf *bytes.Buffer, terms ...T) error {
	// Write tag
	if err := buf.WriteByte(tag); err != nil {
		return err
	}
	//
	return encode_n(encode_term[S, T], buf, terms...)
}

func encode_cast[S Logical[S], T Expr[T]](term term.Cast[word.BigEndian, T], buf *bytes.Buffer) error {
	// Write tag
	if err := buf.WriteByte(castTag); err != nil {
		return err
	}
	// Bitwidth
	if err := binary.Write(buf, binary.BigEndian, uint16(term.BitWidth)); err != nil {
		return err
	}
	// term
	return encode_term[S, T](term.Arg, buf)
}

func encode_constant[T Expr[T]](term term.Constant[word.BigEndian, T], buf *bytes.Buffer) error {
	bytes := term.Value.Bytes()
	// Write tag
	if err := buf.WriteByte(constantTag); err != nil {
		return err
	}
	// Write length (in bytes)
	err := buf.WriteByte(byte(len(bytes)))
	//
	if err == nil {
		// Write value as 32bytes
		_, err = buf.Write(bytes[:])
	}
	//
	return err
}

func encode_ifZero[S Logical[S], T Expr[T]](term term.IfZero[word.BigEndian, S, T], buf *bytes.Buffer) error {
	// Write tag
	if err := buf.WriteByte(ifZeroTag); err != nil {
		return err
	}
	// Write condition
	if err := encode_logical[S, T](term.Condition, buf); err != nil {
		return err
	}
	// Write true + false branches
	return encode_n(encode_term[S, T], buf, term.TrueBranch, term.FalseBranch)
}

func encode_labelled_constant[T Expr[T]](term term.LabelledConst[word.BigEndian, T], buf *bytes.Buffer) error {
	var (
		str_bytes   = []byte(term.Label)
		str_len     = uint16(len(str_bytes))
		const_bytes = term.Value.Bytes()
	)
	// Write tag
	if err := buf.WriteByte(labelledConstantTag); err != nil {
		return err
	}
	// Write label length
	if err := binary.Write(buf, binary.BigEndian, str_len); err != nil {
		return err
	}
	// Write label contents
	if n, err := buf.Write(str_bytes); err != nil {
		return err
	} else if n != len(str_bytes) {
		return fmt.Errorf("failed encoding constant label (%d versus %d bytes)", n, len(str_bytes))
	}
	// Write length (in bytes)
	err := buf.WriteByte(byte(len(const_bytes)))
	//
	if err == nil {
		// Write value as 32bytes
		_, err = buf.Write(const_bytes[:])
	}
	//
	return err
}

func encode_exponent[S Logical[S], T Expr[T]](term term.Exp[word.BigEndian, T], buf *bytes.Buffer) error {
	// Write tag
	if err := buf.WriteByte(expTag); err != nil {
		return err
	}
	// Exponent
	if err := binary.Write(buf, binary.BigEndian, term.Pow); err != nil {
		return err
	}
	// term
	return encode_term[S, T](term.Arg, buf)
}

func encode_reg_access[T Expr[T]](term term.RegisterAccess[word.BigEndian, T], buf *bytes.Buffer) error {
	// Write (appropriate) tag
	if err := buf.WriteByte(registerAccessTag); err != nil {
		return err
	}
	//
	return encode_raw_access(&term, buf)
}

func encode_vec_access[T Expr[T]](term term.VectorAccess[word.BigEndian, T], buf *bytes.Buffer) error {
	// Write tag
	if err := buf.WriteByte(vectorAccessTag); err != nil {
		return err
	}
	//
	return encode_nary(encode_raw_access, buf, term.Vars)
}

func encode_raw_access[T Expr[T]](term *term.RegisterAccess[word.BigEndian, T], buf *bytes.Buffer) error {
	var bytes, err = term.MarshalBinary()
	//
	if err != nil {
		return err
	} else if n, err := buf.Write(bytes); err != nil {
		return err
	} else if n != len(bytes) {
		return fmt.Errorf("failed writing buffer (wrote only %d of %d bytes)", n, len(bytes))
	}
	//
	return nil
}

// ============================================================================
// Arithmetic Terms (decoding)
// ============================================================================

// Decode an arbitrary term from the buffer.
func decode_term(buf *bytes.Buffer) (Term, error) {
	tag, err := buf.ReadByte()
	//
	if err != nil {
		return nil, err
	}
	//
	switch tag {
	case addTag:
		return decode_nary_terms(addConstructor, buf)
	case castTag:
		return decode_cast(buf)
	case constantTag:
		return decode_constant(buf)
	case expTag:
		return decode_exponent(buf)
	case ifZeroTag:
		return decode_ifzero(buf)
	case labelledConstantTag:
		return decode_labelled_constant(buf)
	case registerAccessTag:
		return decode_reg_access(buf)
	case mulTag:
		return decode_nary_terms(mulConstructor, buf)
	case normTag:
		return decode_terms(1, normConstructor, buf)
	case subTag:
		return decode_nary_terms(subConstructor, buf)
	case vectorAccessTag:
		return decode_vec_access(buf)
	default:
		return nil, fmt.Errorf("unknown constraint (tag %d)", tag)
	}
}

// Decode a variable number of terms, as determined by the leading byte.
func decode_nary_terms(constructor func([]Term) Term, buf *bytes.Buffer) (Term, error) {
	terms, err := decode_nary(decode_term, buf)
	return constructor(terms), err
}

// Decode exactly n terms
func decode_terms[S any](n uint, constructor func([]Term) S, buf *bytes.Buffer) (S, error) {
	terms, err := decode_n(n, decode_term, buf)
	return constructor(terms), err
}

func decode_cast(buf *bytes.Buffer) (Term, error) {
	var (
		bitwidth uint16
		expr     Term
		err      error
	)
	// Exponent
	if err := binary.Read(buf, binary.BigEndian, &bitwidth); err != nil {
		return nil, err
	}
	// Term
	if expr, err = decode_term(buf); err != nil {
		return expr, err
	}
	// Done
	return term.CastOf(expr, uint(bitwidth)), nil
}

func decode_constant(buf *bytes.Buffer) (Term, error) {
	var (
		bytes   []byte
		len     byte
		element word.BigEndian
		err     error
	)
	// Read constant length
	if len, err = buf.ReadByte(); err != nil {
		return nil, err
	}
	//
	bytes = make([]byte, len)
	//
	if n, err := buf.Read(bytes); err != nil {
		return nil, err
	} else if n != int(len) {
		return nil, errors.New("failed decoding constant")
	}
	//
	element = element.SetBytes(bytes)
	//
	return term.Const[word.BigEndian, Term](element), nil
}

func decode_exponent(buf *bytes.Buffer) (Term, error) {
	var (
		exponent uint64
		expr     Term
		err      error
	)
	// Exponent
	if err := binary.Read(buf, binary.BigEndian, &exponent); err != nil {
		return nil, err
	}
	// Term
	if expr, err = decode_term(buf); err != nil {
		return expr, err
	}
	// Done
	return term.Exponent(expr, exponent), nil
}

func decode_ifzero(buf *bytes.Buffer) (Term, error) {
	var (
		condition LogicalTerm
		branches  []Term
		err       error
	)
	// Condition
	if condition, err = decode_logical(buf); err != nil {
		return &IfZero{}, err
	}
	// True / false branches
	if branches, err = decode_n(2, decode_term, buf); err != nil {
		return &IfZero{}, err
	}
	// Done
	return term.IfElse(condition, branches[0], branches[1]), nil
}

func decode_labelled_constant(buf *bytes.Buffer) (Term, error) {
	var (
		str_bytes   []byte
		str_len     uint16
		const_len   byte
		const_bytes []byte
		element     word.BigEndian
		err         error
	)
	// Label length
	if err := binary.Read(buf, binary.BigEndian, &str_len); err != nil {
		return nil, err
	}
	// Label contents
	str_bytes = make([]byte, str_len)
	if n, err := buf.Read(str_bytes); err != nil {
		return nil, err
	} else if n != int(str_len) {
		return nil, errors.New("failed decoding labelled constant")
	}
	// Read constant length
	if const_len, err = buf.ReadByte(); err != nil {
		return nil, err
	}
	//
	const_bytes = make([]byte, const_len)
	// Constant
	if n, err := buf.Read(const_bytes); err != nil {
		return nil, err
	} else if n != int(const_len) {
		return nil, errors.New("failed decoding labelled constant")
	}
	//
	element = element.SetBytes(const_bytes)
	//
	return term.LabelledConstant[word.BigEndian, Term](string(str_bytes), element), nil
}

func decode_reg_access(buf *bytes.Buffer) (*RegisterAccess, error) {
	var term RegisterAccess
	//
	err := term.UnmarshalBuffer(buf)
	//
	return &term, err
}

func decode_vec_access(buf *bytes.Buffer) (Term, error) {
	vars, err := decode_nary(decode_reg_access, buf)
	//
	return &term.VectorAccess[word.BigEndian, Term]{Vars: vars}, err
}

// ============================================================================
// Helpers
// ============================================================================

func encode_nary[T any](encoder func(T, *bytes.Buffer) error, buf *bytes.Buffer, terms []T) error {
	var n byte = byte(len(terms))
	// Write n
	if err := buf.WriteByte(n); err != nil {
		return err
	}
	//
	return encode_n(encoder, buf, terms...)
}

func encode_n[T any](encoder func(T, *bytes.Buffer) error, buf *bytes.Buffer, terms ...T) error {
	//
	for _, t := range terms {
		if err := encoder(t, buf); err != nil {
			return err
		}
	}
	//
	return nil
}

func decode_nary[T any](decoder func(*bytes.Buffer) (T, error), buf *bytes.Buffer) ([]T, error) {
	var (
		// NOTE: hard limit enforced here that we have at most 256 terms.
		n, err = buf.ReadByte()
	)
	//
	if err != nil {
		return nil, err
	}
	//
	return decode_n(uint(n), decoder, buf)
}

func decode_n[T any](n uint, decoder func(*bytes.Buffer) (T, error), buf *bytes.Buffer) ([]T, error) {
	var (
		terms = make([]T, n)
		err   error
	)
	//
	for i := range terms {
		if terms[i], err = decoder(buf); err != nil {
			return nil, err
		}
	}
	//
	return terms, nil
}

// ============================================================================
// Constructors
// ============================================================================

func addConstructor(terms []Term) Term {
	return term.Sum(terms...)
}

func conjunctionConstructor(terms []LogicalTerm) LogicalTerm {
	return term.Conjunction(terms...)
}

func disjunctionConstructor(terms []LogicalTerm) LogicalTerm {
	return term.Disjunction(terms...)
}

func equalConstructor(terms []Term) LogicalTerm {
	return term.Equals[word.BigEndian, LogicalTerm](terms[0], terms[1])
}

func iteTrueFalseConstructor(terms []LogicalTerm) LogicalTerm {
	return term.IfThenElse(terms[0], terms[1], terms[2])
}

func iteTrueConstructor(terms []LogicalTerm) LogicalTerm {
	return term.IfThenElse(terms[0], terms[1], nil)
}

func iteFalseConstructor(terms []LogicalTerm) LogicalTerm {
	return term.IfThenElse(terms[0], nil, terms[1])
}

func mulConstructor(terms []Term) Term {
	return term.Product(terms...)
}

func negationConstructor(terms []LogicalTerm) LogicalTerm {
	return term.Negation(terms[0])
}

func notEqualConstructor(terms []Term) LogicalTerm {
	return term.NotEquals[word.BigEndian, LogicalTerm](terms[0], terms[1])
}

func subConstructor(terms []Term) Term {
	return term.Subtract(terms...)
}

func normConstructor(terms []Term) Term {
	return term.Normalise(terms[0])
}
