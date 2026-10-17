// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
)

// And returns a filter that matches when every one of filters matches.
// And() matches every record. A nil filter is a build error.
func And(filters ...Filter) Filter {
	return AndNode{Filters: slices.Clone(filters)}
}

// Or returns a filter that matches when any one of filters matches.
// Or() matches no record. A nil filter is a build error.
func Or(filters ...Filter) Filter {
	return OrNode{Filters: slices.Clone(filters)}
}

// Not returns a filter that negates f under three-valued logic: it matches
// only when f is false, not when f is unknown. A nil f is a build error.
func Not(f Filter) Filter {
	return NotNode{Filter: f}
}

// ID returns a filter that matches records whose ID is one of ids.
// ID() matches no record.
func ID(ids ...string) Filter {
	return IDNode{IDs: slices.Clone(ids)}
}

// Func returns a filter that matches when fn returns true. fn receives the
// record as any and asserts it to the record type. A nil fn is a build error.
func Func(fn func(any) bool) Filter {
	if fn == nil {
		return ErrorNode{Err: fmt.Errorf("%w: Func: nil predicate", ErrNilFilter)}
	}
	return FuncNode{Fn: fn}
}

// compare builds a CompareNode with a single operand.
func compare(op Op, p Path, operand any) Filter {
	p, err := checkPath(op.String(), p)
	if err != nil {
		return ErrorNode{Err: err}
	}
	return CompareNode{Op: op, Path: p, Operand: operand}
}

// compareValue builds a CompareNode with a single Value operand.
func compareValue[V Value](op Op, p Path, v V) Filter {
	p, err := checkPath(op.String(), p)
	if err != nil {
		return ErrorNode{Err: err}
	}
	operand, err := value(v)
	if err != nil {
		return invalid(op, p, "%v", err)
	}
	return CompareNode{Op: op, Path: p, Operand: operand}
}

// Eq returns a filter that is true when the field equals v: same kind and
// same value. Numbers compare as float64.
func Eq[V Value](p Path, v V) Filter { return compareValue(OpEq, p, v) }

// Ne returns a filter that is true when the field and v are the same kind
// and not equal.
func Ne[V Value](p Path, v V) Filter { return compareValue(OpNe, p, v) }

// In returns a filter that is true when the field equals any of vs.
// In with no operands is false; Go cannot infer V from no arguments, so
// write it as In[string](p).
func In[V Value](p Path, vs ...V) Filter {
	p, err := checkPath(OpIn.String(), p)
	if err != nil {
		return ErrorNode{Err: err}
	}
	operands := make([]any, len(vs))
	for i, v := range vs {
		if operands[i], err = value(v); err != nil {
			return invalid(OpIn, p, "element %d: %v", i, err)
		}
	}
	return CompareNode{Op: OpIn, Path: p, Operands: operands}
}

// NotIn returns Not(In(p, vs...)).
func NotIn[V Value](p Path, vs ...V) Filter { return Not(In(p, vs...)) }

// Lt returns a filter that is true when the field is less than v: number
// against number numerically, or string against string by byte order.
func Lt[V Value](p Path, v V) Filter { return compareValue(OpLt, p, v) }

// Lte is as [Lt], for less than or equal.
func Lte[V Value](p Path, v V) Filter { return compareValue(OpLte, p, v) }

// Gt is as [Lt], for greater than.
func Gt[V Value](p Path, v V) Filter { return compareValue(OpGt, p, v) }

// Gte is as [Lt], for greater than or equal.
func Gte[V Value](p Path, v V) Filter { return compareValue(OpGte, p, v) }

// EqFold returns a filter that is true when the field is a string equal to s
// under Unicode case folding.
func EqFold(p Path, s string) Filter { return compare(OpEqFold, p, s) }

// Like returns a filter that is true when the field is a string containing s.
func Like(p Path, s string) Filter { return compare(OpLike, p, s) }

// LikeFold is as [Like], case-folded.
func LikeFold(p Path, s string) Filter { return compare(OpLikeFold, p, s) }

// Prefix returns a filter that is true when the field is a string starting
// with s.
func Prefix(p Path, s string) Filter { return compare(OpPrefix, p, s) }

// PrefixFold is as [Prefix], case-folded.
func PrefixFold(p Path, s string) Filter { return compare(OpPrefixFold, p, s) }

// Suffix returns a filter that is true when the field is a string ending
// with s.
func Suffix(p Path, s string) Filter { return compare(OpSuffix, p, s) }

// SuffixFold is as [Suffix], case-folded.
func SuffixFold(p Path, s string) Filter { return compare(OpSuffixFold, p, s) }

// Regex returns a filter that is true when the field is a string and
// re.MatchString(field) is true. A nil re is a build error.
func Regex(p Path, re *regexp.Regexp) Filter {
	p, err := checkPath(OpRegex.String(), p)
	if err != nil {
		return ErrorNode{Err: err}
	}
	if re == nil {
		return ErrorNode{Err: fmt.Errorf("%w: Regex(%q)", ErrNilRegexp, []string(p))}
	}
	return CompareNode{Op: OpRegex, Path: p, Operand: re}
}

// Contains returns a filter that is true when the field is an array with an
// element equal to v.
func Contains[V Value](p Path, v V) Filter { return compareValue(OpContains, p, v) }

// invalid returns an ErrorNode wrapping ErrInvalidOperand.
func invalid(op Op, p Path, format string, args ...any) Filter {
	return ErrorNode{Err: fmt.Errorf("%w: %s(%q): %s", ErrInvalidOperand, op, []string(p), fmt.Sprintf(format, args...))}
}

// hasOperand validates and converts one operand of Has, HasAny, or HasAll.
// It returns a description of the problem, or "" if x is valid.
func hasOperand(x any) (any, string) {
	if x == nil {
		return nil, "null operand; use IsNull"
	}
	if k := reflect.ValueOf(x).Kind(); k == reflect.Slice || k == reflect.Array {
		return nil, "array operand; use HasAny or HasAll"
	}
	v, err := toJSON(x, false)
	if err != nil {
		return nil, err.Error()
	}
	return v, ""
}

// Has returns a filter that is true when the field contains x, as defined by
// the containment rules in the design (§6.4).
//
// x is a scalar (bool, string, or number) or an object. Objects may be typed
// maps with string keys, such as map[string]int; they are deep-copied to
// map[string]any, with numbers converted to float64. Values nested inside an
// object may be scalars, null, or objects.
//
// These are build errors: an array or slice x (use [HasAny] or [HasAll]), an
// array or slice anywhere inside an object x, a nil x (use [IsNull]), and any
// value, at any depth, that is not a JSON kind (see [Value] for the rules on
// numbers).
func Has(p Path, x any) Filter {
	p, err := checkPath(OpHas.String(), p)
	if err != nil {
		return ErrorNode{Err: err}
	}
	v, problem := hasOperand(x)
	if problem != "" {
		return invalid(OpHas, p, "%s", problem)
	}
	return CompareNode{Op: OpHas, Path: p, Operand: v}
}

// hasMany builds HasAny and HasAll.
func hasMany(op Op, p Path, xs []any) Filter {
	p, err := checkPath(op.String(), p)
	if err != nil {
		return ErrorNode{Err: err}
	}
	operands := make([]any, len(xs))
	for i, x := range xs {
		v, problem := hasOperand(x)
		if problem != "" {
			return invalid(op, p, "element %d: %s", i, problem)
		}
		operands[i] = v
	}
	return CompareNode{Op: op, Path: p, Operands: operands}
}

// HasAny returns a filter that is true when the field [Has] any of xs.
// HasAny with no operands is false.
//
// Each of xs is validated and converted as the operand of [Has], so an
// element that is an array or nil, or an object containing an array, is a
// build error. Pass the elements, not a
// slice of them: HasAny(p, "a", "b"), not HasAny(p, []string{"a", "b"}).
func HasAny(p Path, xs ...any) Filter { return hasMany(OpHasAny, p, xs) }

// HasAll returns a filter that is true when the field [Has] every one of xs.
// HasAll with no operands is true for any present, non-null field.
//
// Each of xs is validated and converted as the operand of [Has], as for
// [HasAny].
func HasAll(p Path, xs ...any) Filter { return hasMany(OpHasAll, p, xs) }

// SameArray returns a filter that is true when the field is an array
// deep-equal to a: same length, same order.
//
// a may be any slice or array, such as []string; it is deep-copied to []any,
// with numbers converted to float64. A nil slice is the empty array. These
// are build errors: an a that is not a slice or array (including nil), and
// any element, at any depth, that is not a JSON kind (see [Value] for the
// rules on numbers).
func SameArray(p Path, a any) Filter {
	p, err := checkPath(OpSameArray.String(), p)
	if err != nil {
		return ErrorNode{Err: err}
	}
	v, err := toJSON(a, true)
	if err != nil {
		return invalid(OpSameArray, p, "%v", err)
	}
	if _, ok := v.([]any); !ok {
		return invalid(OpSameArray, p, "operand is %T, not an array", a)
	}
	return CompareNode{Op: OpSameArray, Path: p, Operand: v}
}

// SameObject returns a filter that is true when the field is an object
// deep-equal to o: same key set, deep-equal values.
//
// o may be any map with string keys, such as map[string]int; it is
// deep-copied to map[string]any, with numbers converted to float64. A nil
// map is the empty object. These are build errors: an o that is not a map
// with string keys (including nil), and any value, at any depth, that is not
// a JSON kind (see [Value] for the rules on numbers). Arrays are allowed
// inside o.
func SameObject(p Path, o any) Filter {
	p, err := checkPath(OpSameObject.String(), p)
	if err != nil {
		return ErrorNode{Err: err}
	}
	v, err := toJSON(o, true)
	if err != nil {
		return invalid(OpSameObject, p, "%v", err)
	}
	if _, ok := v.(map[string]any); !ok {
		return invalid(OpSameObject, p, "operand is %T, not an object", o)
	}
	return CompareNode{Op: OpSameObject, Path: p, Operand: v}
}

// IsNull returns a filter that is true when the field is present and null,
// and false when it is missing.
func IsNull(p Path) Filter { return compare(OpIsNull, p, nil) }

// IsBool returns a filter that is true when the field is present and a bool.
func IsBool(p Path) Filter { return compare(OpIsBool, p, nil) }

// IsNumber returns a filter that is true when the field is present and a
// number.
func IsNumber(p Path) Filter { return compare(OpIsNumber, p, nil) }

// IsString returns a filter that is true when the field is present and a
// string.
func IsString(p Path) Filter { return compare(OpIsString, p, nil) }

// IsArray returns a filter that is true when the field is present and an
// array.
func IsArray(p Path) Filter { return compare(OpIsArray, p, nil) }

// IsObject returns a filter that is true when the field is present and an
// object.
func IsObject(p Path) Filter { return compare(OpIsObject, p, nil) }
