package logical

import (
	"cmp"
	"math/big"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/source/bexp"
)

func Test_Prop_01(t *testing.T) {
	testEquivalent(t, "x==1", "x==1")
}

func Test_Prop_02(t *testing.T) {
	testEquivalent(t, "x==y", "y==x")
}

func Test_Prop_03(t *testing.T) {
	testEquivalent(t, "x!=x", "⊥")
}

func Test_Prop_04(t *testing.T) {
	testEquivalent(t, "x==x", "⊤")
}

// Conjunctions

func Test_Prop_10(t *testing.T) {
	testEquivalent(t, "⊥ ∧ ⊥", "⊥")
}

func Test_Prop_11(t *testing.T) {
	testEquivalent(t, "⊥ ∧ ⊤", "⊥")
}

func Test_Prop_12(t *testing.T) {
	testEquivalent(t, "⊤ ∧ ⊥", "⊥")
}

func Test_Prop_13(t *testing.T) {
	testEquivalent(t, "⊤ ∧ ⊤", "⊤")
}

func Test_Prop_14(t *testing.T) {
	testEquivalent(t, "x==y ∧ ⊤", "x==y")
}

func Test_Prop_15(t *testing.T) {
	testEquivalent(t, "⊤ ∧ x==y", "x==y")
}

func Test_Prop_16(t *testing.T) {
	testEquivalent(t, "x==y ∧ ⊥", "⊥")
}

func Test_Prop_17(t *testing.T) {
	testEquivalent(t, "⊥ ∧ x==y", "⊥")
}

func Test_Prop_18(t *testing.T) {
	testEquivalent(t, "x==y ∧ x==y", "x==y")
}

func Test_Prop_19(t *testing.T) {
	testEquivalent(t, "x==y ∧ x≠y", "⊥")
}

func Test_Prop_20(t *testing.T) {
	testEquivalent(t, "x==1 ∧ x==2", "⊥")
}

func Test_Prop_21(t *testing.T) {
	testEquivalent(t, "x==1 ∧ x!=2", "x==1")
}

func Test_Prop_22(t *testing.T) {
	testEquivalent(t, "x==1 ∧ x!=y", "x==1 ∧ x!=y")
}

func Test_Prop_23(t *testing.T) {
	testEquivalent(t, "x==y ∧ x!=1", "x==y ∧ x!=1")
}

func Test_Prop_24(t *testing.T) {
	testEquivalent(t, "x==y ∧ x!=z", "x==y ∧ x!=z")
}

func Test_Prop_25(t *testing.T) {
	testEquivalent(t, "x==0 ∧ y==x", "x==0 ∧ y==0")
}

func Test_Prop_26(t *testing.T) {
	testEquivalent(t, "x==0 ∧ y!=x", "x==0 ∧ y!=0")
}

func Test_Prop_27(t *testing.T) {
	testEquivalent(t, "x==0 ∧ y==x ∧ y==1", "⊥")
}

func Test_Prop_28(t *testing.T) {
	testEquivalent(t, "x==0 ∧ y!=x ∧ y==0", "⊥")
}

func Test_Prop_29(t *testing.T) {
	testEquivalent(t, "x==y ∧ y==z", "x==z ∧ y==z")
}

func Test_Prop_30(t *testing.T) {
	testEquivalent(t, "x==z ∧ y==x ∧ y!=z", "⊥")
}

// Disjunctions

func Test_Prop_50(t *testing.T) {
	testEquivalent(t, "⊥ ∨ ⊥", "⊥")
}

func Test_Prop_51(t *testing.T) {
	testEquivalent(t, "⊥ ∨ ⊤", "⊤")
}

func Test_Prop_52(t *testing.T) {
	testEquivalent(t, "⊤ ∨ ⊥", "⊤")
}

func Test_Prop_53(t *testing.T) {
	testEquivalent(t, "⊤ ∨ ⊤", "⊤")
}

func Test_Prop_54(t *testing.T) {
	testEquivalent(t, "x==y ∨ ⊤", "⊤")
}

func Test_Prop_55(t *testing.T) {
	testEquivalent(t, "⊤ ∨ x==y", "⊤")
}

func Test_Prop_56(t *testing.T) {
	testEquivalent(t, "x==y ∨ ⊥", "x==y")
}

func Test_Prop_57(t *testing.T) {
	testEquivalent(t, "⊥ ∨ x==y", "x==y")
}

func Test_Prop_58(t *testing.T) {
	testEquivalent(t, "x==y ∨ x==y", "x==y")
}

func Test_Prop_59(t *testing.T) {
	testEquivalent(t, "x==y ∨ x≠y", "⊤")
}
func Test_Prop_60(t *testing.T) {
	testEquivalent(t, "x≠0 ∨ x==0 ∨ y==0", "⊤")
}

// Combine Disjunctions and Conjunctions

// Missing rule which infers x==0 throuh x≠y
// func Test_Prop_80(t *testing.T) {
// 	testEquivalent(t, "x≠0 ∨ (x==0 ∧ x≠y)", "x≠0 ∨ x≠y")
// }

func Test_Prop_81(t *testing.T) {
	testEquivalent(t, "(x≠0 ∧ y≠0) ∨ x==0 ∨ y==0", "⊤")
}

func Test_Prop_82(t *testing.T) {
	testEquivalent(t, "x==0", "x==0 ∨ (x==0 ∧ y≠0)", "x==0 ∨ (x==0 ∧ y≠0) ∨ (x==0 ∧ x==0 ∧ y≠0)")
}
func Test_Prop_83(t *testing.T) {
	testEquivalent(t, "(x==0 ∧ y==0) ∨ (x==0 ∧ y==0 ∧ z==0)", "(x==0 ∧ y==0)")
}
func Test_Prop_84(t *testing.T) {
	testEquivalent(t, "(x==0 ∧ y==0) ∨ (x==0 ∧ y==0)", "x==0 ∧ y==0")
}

func Test_Prop_85(t *testing.T) {
	testEquivalent(t, "(x==0 ∧ y==0) ∨ x!=0 ∨ y!=0", "⊤")
}

func Test_Prop_86(t *testing.T) {
	testEquivalent(t, "(n==w0 ∧ w1==0) ∧ (w0!=0 ∨ w1!=0)", "n==w0 ∧ w1==0 ∧ w0!=0")
}

// ============================================================================
// Framework
// ============================================================================

func testEquivalent(t *testing.T, terms ...string) {
	var (
		ts = make([]Proposition[Var, Equality[Var]], len(terms))
	)
	// Parse them
	for i, term := range terms {
		ts[i] = Parse(t, term)
	}
	// Check them
	for i := 1; i < len(ts); i++ {
		var (
			lhs = ts[i-1]
			rhs = ts[i]
		)
		if !lhs.Equals(rhs) {
			t.Errorf("not equivalent: %s=>%s but %s=>%s", terms[i-1], lhs.String(id), terms[i], rhs.String(id))
		}
	}
}

// ============================================================================
// Parser
// ============================================================================

func Parse(t *testing.T, input string) Proposition[Var, Equality[Var]] {
	var env = func(string) bool { return true }
	// Parse input
	term, errs := bexp.Parse[LogicalTerm](input, env)
	// Sanity check errors
	if len(errs) > 0 {
		t.Errorf("internal failure: %s", errs[0].Message())
		t.FailNow()
	}
	//
	return term.prop
}

func id(x Var) string {
	return x.name
}

// Var is a wrapper around a string
type Var struct {
	name string
}

// Cmp
func (p Var) Cmp(o Var) int {
	return cmp.Compare(p.name, o.name)
}

func (p Var) String() string {
	return p.name
}

type LogicalTerm struct {
	expr util.Union[string, big.Int]
	prop Proposition[Var, Equality[Var]]
}

func (p LogicalTerm) Variable(v string) LogicalTerm {
	return LogicalTerm{expr: util.Union1[string, big.Int](v)}
}

func (p LogicalTerm) Number(v big.Int) LogicalTerm {
	return LogicalTerm{expr: util.Union2[string](v)}
}

func (p LogicalTerm) Or(terms ...LogicalTerm) LogicalTerm {
	term := p.prop
	//
	for _, t := range terms {
		term = term.Or(t.prop)
	}
	//
	return LogicalTerm{prop: term}
}

func (p LogicalTerm) And(terms ...LogicalTerm) LogicalTerm {
	term := p.prop
	//
	for _, t := range terms {
		term = term.And(t.prop)
	}
	//
	return LogicalTerm{prop: term}
}

func (p LogicalTerm) Truth(val bool) LogicalTerm {
	return LogicalTerm{prop: Truth[Var, Equality[Var]](val)}
}

func (p LogicalTerm) Equals(o LogicalTerm) LogicalTerm {
	var (
		atom Equality[Var]
		lhs  = Var{p.expr.First()}
	)
	// Parse rhs
	if o.expr.HasFirst() {
		// var = var
		atom = Equals(lhs, Var{o.expr.First()})
	} else {
		atom = EqualsConst(lhs, o.expr.Second())
	}
	//
	return LogicalTerm{prop: NewProposition(atom)}
}

func (p LogicalTerm) NotEquals(o LogicalTerm) LogicalTerm {
	var (
		atom Equality[Var]
		lhs  = Var{p.expr.First()}
	)
	// Parse rhs
	if o.expr.HasFirst() {
		// var = var
		atom = NotEquals(lhs, Var{o.expr.First()})
	} else {
		atom = NotEqualsConst(lhs, o.expr.Second())
	}
	//
	return LogicalTerm{prop: NewProposition(atom)}
}

func (p LogicalTerm) LessThan(LogicalTerm) LogicalTerm {
	panic("unsupported operation")
}

func (p LogicalTerm) LessThanEquals(LogicalTerm) LogicalTerm {
	panic("unsupported operation")
}

// Arithmetic
func (p LogicalTerm) Add(...LogicalTerm) LogicalTerm {
	panic("unsupported operation")
}

func (p LogicalTerm) Mul(...LogicalTerm) LogicalTerm {
	panic("unsupported operation")
}

func (p LogicalTerm) Sub(...LogicalTerm) LogicalTerm {
	panic("unsupported operation")
}
