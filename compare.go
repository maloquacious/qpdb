// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// This file evaluates a CompareNode (design §6.3 and §6.4) for [Match].

// checkCompare reports why n cannot be evaluated: an empty path, an unknown
// operator, or an operand of a type its operator does not take. It checks
// only the operand fields the operator uses, but checks those at every
// depth: the arrays and objects inside an operand of SameArray and
// SameObject must hold only JSON kinds, and an object operand of Has,
// HasAny, and HasAll must hold only JSON kinds other than arrays. An operand
// nested more than maxDepth deep, which a cyclic hand-built operand is, or
// too large for one walk (see maxSteps), is also an error.
func checkCompare(n CompareNode) error {
	if len(n.Path) == 0 {
		return fmt.Errorf("%w: %s", ErrEmptyPath, n.Op)
	}
	bad := func(i int, x any, want string) error {
		where := "operand"
		if i >= 0 {
			where = fmt.Sprintf("element %d", i)
		}
		return fmt.Errorf("%w: %s(%q): %s is %T, want %s", ErrInvalidOperand, n.Op, []string(n.Path), where, x, want)
	}
	deep := func(i int, x any, arrays bool) error {
		err := checkOperand(&walker{}, x, arrays, 0)
		if err == nil {
			return nil
		}
		where := "operand"
		if i >= 0 {
			where = fmt.Sprintf("element %d", i)
		}
		return fmt.Errorf("%w: %s(%q): %s: %w", ErrInvalidOperand, n.Op, []string(n.Path), where, err)
	}
	switch n.Op {
	case OpEq, OpNe, OpLt, OpLte, OpGt, OpGte, OpContains:
		if !isScalar(n.Operand) {
			return bad(-1, n.Operand, "bool, string, or finite float64")
		}
	case OpIn:
		for i, x := range n.Operands {
			if !isScalar(x) {
				return bad(i, x, "bool, string, or finite float64")
			}
		}
	case OpEqFold, OpLike, OpLikeFold, OpPrefix, OpPrefixFold, OpSuffix, OpSuffixFold:
		if _, ok := n.Operand.(string); !ok {
			return bad(-1, n.Operand, "string")
		}
	case OpRegex:
		if re, ok := n.Operand.(*regexp.Regexp); !ok || re == nil {
			return bad(-1, n.Operand, "non-nil *regexp.Regexp")
		}
	case OpHas:
		if !isHasOperand(n.Operand) {
			return bad(-1, n.Operand, "bool, string, finite float64, or map[string]any")
		}
		return deep(-1, n.Operand, false)
	case OpHasAny, OpHasAll:
		for i, x := range n.Operands {
			if !isHasOperand(x) {
				return bad(i, x, "bool, string, finite float64, or map[string]any")
			}
			if err := deep(i, x, false); err != nil {
				return err
			}
		}
	case OpSameArray:
		if _, ok := n.Operand.([]any); !ok {
			return bad(-1, n.Operand, "[]any")
		}
		return deep(-1, n.Operand, true)
	case OpSameObject:
		if _, ok := n.Operand.(map[string]any); !ok {
			return bad(-1, n.Operand, "map[string]any")
		}
		return deep(-1, n.Operand, true)
	case OpIsNull, OpIsBool, OpIsNumber, OpIsString, OpIsArray, OpIsObject:
	default:
		return fmt.Errorf("%w: unknown operator %v", ErrInvalidFilter, n.Op)
	}
	return nil
}

// isScalar reports whether x is a normalized scalar operand.
func isScalar(x any) bool {
	switch x := x.(type) {
	case bool, string:
		return true
	case float64:
		return !math.IsNaN(x) && !math.IsInf(x, 0)
	}
	return false
}

// checkOperand reports why operand value x, at the given depth, cannot be
// evaluated: it is not a JSON kind, or it is an array and arrays is false,
// or so is a value nested inside it, or w's limits are exceeded. Object keys
// are visited in sorted order, so the error reported is deterministic.
func checkOperand(w *walker, x any, arrays bool, depth int) error {
	if err := w.enter(depth); err != nil {
		return err
	}
	switch y := x.(type) {
	case nil, bool, string:
		return nil
	case float64:
		if math.IsNaN(y) || math.IsInf(y, 0) {
			return fmt.Errorf("%v is not a JSON number", y)
		}
		return nil
	case []any:
		if !arrays {
			return errors.New("array not allowed inside a Has operand")
		}
		for _, e := range y {
			if err := checkOperand(w, e, arrays, depth+1); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(y)) {
			if err := checkOperand(w, y[k], arrays, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("nested value of Go type %T is not a JSON kind", x)
}

// isHasOperand reports whether x is a normalized operand of Has: a scalar or
// an object.
func isHasOperand(x any) bool {
	_, isObject := x.(map[string]any)
	return isObject || isScalar(x)
}

// kind is the JSON kind of a value (design §2.2).
type kind int8

const (
	// kindForeign is a Go type that is not a JSON kind. Match cannot
	// evaluate a field value of a foreign kind and returns an error.
	kindForeign kind = iota
	kindNull
	kindBool
	kindNumber
	kindString
	kindArray
	kindObject
)

// kindOf returns the JSON kind of v. A nil []any is an empty array and a nil
// map[string]any an empty object, as for operands.
func kindOf(v any) kind {
	switch v.(type) {
	case nil:
		return kindNull
	case bool:
		return kindBool
	case float64:
		return kindNumber
	case string:
		return kindString
	case []any:
		return kindArray
	case map[string]any:
		return kindObject
	}
	return kindForeign
}

// maxSteps bounds the number of values one walk visits. A walk is one
// traversal of a field value against one operand value (one Has test
// against one operand of HasAny, one Contains, one SameArray, and so on),
// or the check of one operand. Only an aliased value, whose arrays or
// objects are shared so that it has far more paths than distinct values, or
// a very large one, reaches the budget; encoding/json never produces the
// former. It is a variable only so that tests can lower it.
var maxSteps = 1 << 24

// walker counts the values visited during one walk. Every function that
// descends into a value takes it, so a walk visits at most maxSteps values,
// and no path is deeper than maxDepth.
type walker struct {
	steps int
}

// enter records a visit to a value at the given depth. It returns an error
// if the value is nested more than maxDepth deep or the step budget is
// spent; the caller abandons the whole evaluation.
func (w *walker) enter(depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("value nested more than %d deep", maxDepth)
	}
	w.steps++
	if w.steps > maxSteps {
		return fmt.Errorf("value too large: a single comparison visits more than %d values", maxSteps)
	}
	return nil
}

// foreignValue is the error for a field value, at any depth, that is not a
// JSON kind.
func foreignValue(v any) error {
	return fmt.Errorf("field value of Go type %T is not a JSON kind", v)
}

// evalCompare evaluates n, already checked by checkCompare, against the field
// value v; ok is false if the field is missing. It returns an error for a
// field value that is not a JSON kind or that is nested too deep.
func evalCompare(n CompareNode, v any, ok bool) (tri, error) {
	k := kindOf(v)
	if ok && k == kindForeign {
		return triUnknown, foreignValue(v)
	}

	// The type tests are two-valued: false, not unknown, on a missing or
	// null field.
	switch n.Op {
	case OpIsNull:
		return triOf(ok && k == kindNull), nil
	case OpIsBool:
		return triOf(ok && k == kindBool), nil
	case OpIsNumber:
		return triOf(ok && k == kindNumber), nil
	case OpIsString:
		return triOf(ok && k == kindString), nil
	case OpIsArray:
		return triOf(ok && k == kindArray), nil
	case OpIsObject:
		return triOf(ok && k == kindObject), nil
	}

	if !ok || k == kindNull {
		return triUnknown, nil
	}
	switch n.Op {
	case OpEq:
		return eqScalar(v, n.Operand), nil
	case OpNe:
		return -eqScalar(v, n.Operand), nil
	case OpIn:
		// Kleene OR of Eq over the operands; In(p) is false.
		result := triFalse
		for _, x := range n.Operands {
			result = max(result, eqScalar(v, x))
		}
		return result, nil
	case OpLt, OpLte, OpGt, OpGte:
		return order(n.Op, v, n.Operand), nil

	case OpEqFold, OpLike, OpLikeFold, OpPrefix, OpPrefixFold, OpSuffix, OpSuffixFold:
		s, isString := v.(string)
		if !isString {
			return triUnknown, nil
		}
		return triOf(stringOp(n.Op, s, n.Operand.(string))), nil
	case OpRegex:
		s, isString := v.(string)
		if !isString {
			return triUnknown, nil
		}
		return triOf(n.Operand.(*regexp.Regexp).MatchString(s)), nil

	case OpContains:
		return contains(v, n.Operand)
	case OpHas:
		return has(v, n.Operand)
	case OpHasAny:
		// Kleene OR of Has over the operands; HasAny(p) is false.
		result := triFalse
		for _, x := range n.Operands {
			t, err := has(v, x)
			if err != nil {
				return triUnknown, err
			}
			result = max(result, t)
		}
		return result, nil
	case OpHasAll:
		// Kleene AND of Has over the operands; HasAll(p) is true.
		result := triTrue
		for _, x := range n.Operands {
			t, err := has(v, x)
			if err != nil {
				return triUnknown, err
			}
			result = min(result, t)
		}
		return result, nil

	case OpSameArray:
		if k != kindArray {
			return triUnknown, nil
		}
		eq, err := equal(&walker{}, v, n.Operand, 0)
		return triOf(eq), err
	case OpSameObject:
		if k != kindObject {
			return triUnknown, nil
		}
		eq, err := equal(&walker{}, v, n.Operand, 0)
		return triOf(eq), err
	}
	return triUnknown, nil // not reached: checkCompare rejects unknown operators
}

// eqScalar compares field value v, a JSON kind, with scalar operand x under
// strict equality: true or false when they are the same kind, and unknown
// otherwise, including when v is an array or object. Because a kind
// mismatch is unknown rather than false, Ne is exactly Not(Eq).
func eqScalar(v, x any) tri {
	switch a := v.(type) {
	case bool:
		if b, ok := x.(bool); ok {
			return triOf(a == b)
		}
	case float64:
		if b, ok := x.(float64); ok {
			return triOf(a == b)
		}
	case string:
		if b, ok := x.(string); ok {
			return triOf(a == b)
		}
	}
	return triUnknown
}

// order evaluates Lt, Lte, Gt, or Gte: number against number numerically,
// string against string by byte order, and unknown otherwise.
func order(op Op, v, x any) tri {
	switch a := v.(type) {
	case float64:
		if b, ok := x.(float64); ok {
			return triOf(ordered(op, a, b))
		}
	case string:
		if b, ok := x.(string); ok {
			return triOf(ordered(op, a, b))
		}
	}
	return triUnknown
}

// ordered applies op with the built-in operators, so a NaN field value,
// which Lookup should never return, compares false.
func ordered[T cmp.Ordered](op Op, a, b T) bool {
	switch op {
	case OpLt:
		return a < b
	case OpLte:
		return a <= b
	case OpGt:
		return a > b
	}
	return a >= b
}

// stringOp evaluates a string operator on field s and operand x.
func stringOp(op Op, s, x string) bool {
	switch op {
	case OpEqFold:
		return fold(s) == fold(x)
	case OpLike:
		return strings.Contains(s, x)
	case OpLikeFold:
		return strings.Contains(fold(s), fold(x))
	case OpPrefix:
		return strings.HasPrefix(s, x)
	case OpPrefixFold:
		return strings.HasPrefix(fold(s), fold(x))
	case OpSuffix:
		return strings.HasSuffix(s, x)
	}
	return strings.HasSuffix(fold(s), fold(x)) // OpSuffixFold
}

// fold maps each rune of s to a canonical member of its simple case-folding
// orbit (see [unicode.SimpleFold]), so two strings are equal after fold
// exactly when [strings.EqualFold] reports them equal. This is Unicode
// simple case folding: each rune folds to one rune, so "ß" does not equal
// "ss". The case-folded string operators compare folded strings, so
// LikeFold, PrefixFold, and SuffixFold agree with EqFold.
//
// Invalid UTF-8 bytes fold to U+FFFD, as strings.EqualFold treats them.
func fold(s string) string {
	return strings.Map(foldRune, s)
}

// foldRune returns the smallest rune in r's simple case-folding orbit.
func foldRune(r rune) rune {
	m := r
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		m = min(m, f)
	}
	return m
}

// equal reports whether field value a and operand value b are the same
// JSON kind and deep-equal: numbers as float64, arrays element by element
// with the same length, and objects with the same key set.
//
// It visits array elements in order and object keys in sorted order, and
// stops at the first difference, so whether it reaches a given value does
// not depend on map iteration order. A value it reaches that is not a JSON
// kind is an error, as is exceeding the walker's limits.
func equal(w *walker, a, b any, depth int) (bool, error) {
	if err := w.enter(depth); err != nil {
		return false, err
	}
	ka := kindOf(a)
	if ka == kindForeign {
		return false, foreignValue(a)
	}
	if ka != kindOf(b) {
		return false, nil
	}
	switch x := a.(type) {
	case nil:
		return true, nil
	case bool:
		return x == b.(bool), nil
	case float64:
		return x == b.(float64), nil
	case string:
		return x == b.(string), nil
	case []any:
		y := b.([]any)
		if len(x) != len(y) {
			return false, nil
		}
		for i := range x {
			if eq, err := equal(w, x[i], y[i], depth+1); err != nil || !eq {
				return false, err
			}
		}
		return true, nil
	}
	x, y := a.(map[string]any), b.(map[string]any)
	if len(x) != len(y) {
		return false, nil
	}
	for _, k := range slices.Sorted(maps.Keys(x)) {
		yv, ok := y[k]
		if !ok {
			return false, nil
		}
		if eq, err := equal(w, x[k], yv, depth+1); err != nil || !eq {
			return false, err
		}
	}
	return true, nil
}

// contains evaluates Contains for field value v, which is present, non-null,
// and a JSON kind: unknown unless v is an array; otherwise true if some
// element equals x, and false if none does. An element of another kind,
// including null, is not equal to x; it does not make the result unknown.
// Contains does not descend into nested arrays. Elements are visited in
// order up to the first match, in one walk.
func contains(v, x any) (tri, error) {
	a, ok := v.([]any)
	if !ok {
		return triUnknown, nil
	}
	w := &walker{}
	for _, e := range a {
		eq, err := equal(w, e, x, 1)
		if err != nil {
			return triUnknown, err
		}
		if eq {
			return triTrue, nil
		}
	}
	return triFalse, nil
}

// has evaluates Has for field value v, which is present, non-null, and a
// JSON kind, and operand x, a scalar or an object (design §6.4).
//
// The design's table gives the cases in which Has can be true. For the
// combinations it leaves out, has is unknown when the top-level field's kind
// cannot contain x, and false when it can but does not:
//
//	field    x               result
//	array    scalar, object  true or false (no matching element is false)
//	object   string, object  true or false
//	object   bool, number    unknown
//	scalar   same kind       true or false
//	scalar   other scalar    unknown (as for Eq)
//	scalar   object          unknown
//
// Below the top level, inside an array element or an object value, a
// mismatch is simply not contained (see hasIn).
func has(v, x any) (tri, error) {
	switch v.(type) {
	case []any:
	case map[string]any:
		switch x.(type) {
		case string, map[string]any:
		default:
			return triUnknown, nil
		}
	default:
		return eqScalar(v, x), nil
	}
	in, err := hasIn(&walker{}, v, prepare(x), 0)
	return triOf(in), err
}

// hasObject is an object operand of Has prepared for hasIn, with its keys
// sorted once per walk rather than at every field value it is tested
// against.
type hasObject struct {
	keys []string
	vals []any // vals[i] is the prepared value at keys[i]
}

// prepare converts every object in x, an operand of Has already checked by
// checkCompare (so it is finite and holds no arrays), to a *hasObject. Other
// values are returned unchanged.
func prepare(x any) any {
	m, ok := x.(map[string]any)
	if !ok {
		return x
	}
	o := &hasObject{keys: slices.Sorted(maps.Keys(m)), vals: make([]any, len(m))}
	for i, k := range o.keys {
		o.vals[i] = prepare(m[k])
	}
	return o
}

// hasIn is the containment relation of design §6.4 on nested values, where
// the result is true or false. A null value inside an object operand acts
// as a scalar:
//
//	field    x               true when
//	object   string          x is a key of the field, even if its value is
//	                         null
//	object   object          every key of x is in the field, and the field's
//	                         value hasIn the value in x
//	array    scalar or null  some element equals x, or some element that is
//	                         an array hasIn x
//	array    object          some element hasIn x
//	scalar   scalar          same kind and equal
//	null     null            always
//
// Every other combination is false. It visits array elements in order and
// the keys of an object x in sorted order, and stops as soon as the result
// is known. A field value it reaches that is not a JSON kind is an error, as
// is exceeding the walker's limits. x has been checked by checkCompare and
// converted by prepare, so an object x is a *hasObject.
func hasIn(w *walker, v, x any, depth int) (bool, error) {
	if err := w.enter(depth); err != nil {
		return false, err
	}
	if kindOf(v) == kindForeign {
		return false, foreignValue(v)
	}
	switch f := v.(type) {
	case map[string]any:
		switch y := x.(type) {
		case string:
			_, ok := f[y]
			return ok, nil
		case *hasObject:
			for i, k := range y.keys {
				fv, ok := f[k]
				if !ok {
					return false, nil
				}
				if in, err := hasIn(w, fv, y.vals[i], depth+1); err != nil || !in {
					return false, err
				}
			}
			return true, nil
		}
		return false, nil
	case []any:
		_, isObject := x.(*hasObject)
		for _, e := range f {
			var in bool
			var err error
			if _, isArray := e.([]any); isArray || isObject {
				in, err = hasIn(w, e, x, depth+1)
			} else {
				in, err = equal(w, e, x, depth+1)
			}
			if err != nil {
				return false, err
			}
			if in {
				return true, nil
			}
		}
		return false, nil
	}
	// v is null or a scalar. equal requires the same kind, so null matches
	// only null, and a scalar only an equal scalar; a *hasObject is not a
	// JSON kind, so it equals no scalar.
	return equal(w, v, x, depth)
}
