// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"errors"
	"fmt"
	"slices"
)

// Match reports whether record r matches filter f. It reads fields only
// through db.Lookup, so it works for any record type; in-memory [Querier]
// implementations use it instead of writing their own evaluator.
//
// Evaluation follows SQL's three-valued logic (design §6.2). A comparison is
// unknown when its field is missing, when the field is null, or when the
// field's kind does not suit the operator. The type tests ([IsNull],
// [IsBool], [IsNumber], [IsString], [IsArray], [IsObject]) are the
// exception: they are true or false, and false on a missing field. NOT
// unknown is unknown, and AND and OR follow Kleene logic. Match returns true
// only when the whole filter is true, so any other comparison on a missing
// or null field never matches, even under [Not].
//
// Match returns an error wrapping [ErrUnsupported] for a node it cannot
// evaluate:
//   - a nil filter, or a nil child of an [AndNode], [OrNode], or [NotNode]
//     (also wrapping [ErrNilFilter]), and a [FuncNode] with a nil Fn;
//   - an [ErrorNode] (also wrapping its Err). A query built with one
//     already carries its error, so this arises only for a filter that did
//     not come from [Query.Filter];
//   - a [CompareNode] with an empty Path, which also wraps [ErrEmptyPath];
//   - a CompareNode with an unknown Op, which also wraps [ErrInvalidFilter];
//   - a CompareNode with an operand of a type its Op does not take, such as
//     an int rather than a float64, which also wraps [ErrInvalidOperand].
//     Operands are checked at every depth: a value nested in a [SameArray]
//     or [SameObject] operand must be a JSON kind, and one nested in an
//     object operand of [Has], [HasAny], or [HasAll] a JSON kind other than
//     an array. An operand nested more than 1000 deep (as a cyclic
//     hand-built operand is), or with more than 2^24 values to check, is
//     also an error wrapping ErrInvalidOperand;
//   - a Filter of any type other than the node types listed on [Filter],
//     including a pointer to a node, an ErrorNode with a nil Err, and a tree
//     nested more than 1000 deep (also wrapping [ErrInvalidFilter]).
//
// Match checks the whole tree for these errors on every call, including the
// children of an AND or OR whose result is already decided, so whether it
// returns one does not depend on short-circuiting or on the record. It does
// not call Lookup or a predicate for a decided child, so a [FuncNode]
// predicate may not be called.
//
// Match also returns an error wrapping ErrUnsupported for a value it cannot
// evaluate:
//   - a value from Lookup, at any depth Match inspects, that is not a JSON
//     kind (see [Querier]); the error names its Go type;
//   - a value nested more than 1000 deep, which includes any cyclic value
//     Match descends into;
//   - a value that a single comparison of it against one operand value
//     (one Contains, one SameArray, one Has test against one operand of
//     HasAny, and so on) would visit more than 2^24 values of. In practice
//     only an aliased value, whose arrays or objects are shared so that it
//     has far more paths than distinct values, or a very large one gets
//     there; encoding/json never produces the former.
//
// These errors depend on the record. Match inspects only the fields of the
// comparisons it evaluates, visits array elements in order and object keys
// in sorted order, and stops descending once a comparison's result is
// known, so it may not reach every nested value.
//
// Match does not recover from a panic in a FuncNode predicate or in Lookup.
// A nil db is an error.
func Match[R any](db Querier[R], f Filter, r R) (bool, error) {
	if db == nil {
		return false, errors.New("qpdb: Match: nil Querier")
	}
	m := matcher[R]{db: db, r: r}
	t, err := m.eval(f, 0, false)
	if err != nil {
		return false, err
	}
	return t == triTrue, nil
}

// tri is a three-valued truth value. The values are ordered so that Kleene
// AND is min, Kleene OR is max, and NOT is negation.
type tri int8

const (
	triFalse   tri = -1
	triUnknown tri = 0
	triTrue    tri = 1
)

// triOf converts a bool to a tri.
func triOf(b bool) tri {
	if b {
		return triTrue
	}
	return triFalse
}

// String returns "true", "false", or "unknown".
func (t tri) String() string {
	switch t {
	case triTrue:
		return "true"
	case triFalse:
		return "false"
	}
	return "unknown"
}

// unsupported wraps err, the reason a node cannot be evaluated, with
// ErrUnsupported.
func unsupported(err error) error {
	return fmt.Errorf("%w: %w", ErrUnsupported, err)
}

// matcher evaluates filters against one record.
type matcher[R any] struct {
	db Querier[R]
	r  R
}

// eval evaluates f at the given depth. If skip is true, the result is
// already decided by an ancestor: eval checks f for errors but does not call
// Lookup or a predicate, and returns triUnknown.
func (m *matcher[R]) eval(f Filter, depth int, skip bool) (tri, error) {
	if depth > maxDepth {
		return triUnknown, unsupported(fmt.Errorf("%w: nested more than %d deep", ErrInvalidFilter, maxDepth))
	}
	switch n := f.(type) {
	case nil:
		return triUnknown, unsupported(ErrNilFilter)
	case AndNode:
		return m.junction("And", n.Filters, triFalse, depth, skip)
	case OrNode:
		return m.junction("Or", n.Filters, triTrue, depth, skip)
	case NotNode:
		if n.Filter == nil {
			return triUnknown, unsupported(fmt.Errorf("%w: Not: child", ErrNilFilter))
		}
		t, err := m.eval(n.Filter, depth+1, skip)
		return -t, err
	case CompareNode:
		if err := checkCompare(n); err != nil {
			return triUnknown, unsupported(err)
		}
		if skip {
			return triUnknown, nil
		}
		v, ok := m.db.Lookup(m.r, n.Path)
		t, err := evalCompare(n, v, ok)
		if err != nil {
			return triUnknown, unsupported(fmt.Errorf("%s(%q): %w", n.Op, []string(n.Path), err))
		}
		return t, nil
	case IDNode:
		if skip {
			return triUnknown, nil
		}
		return m.id(n.IDs)
	case FuncNode:
		if n.Fn == nil {
			return triUnknown, unsupported(fmt.Errorf("%w: Func: nil predicate", ErrNilFilter))
		}
		if skip {
			return triUnknown, nil
		}
		return triOf(n.Fn(m.r)), nil
	case ErrorNode:
		if n.Err == nil {
			return triUnknown, unsupported(fmt.Errorf("%w: ErrorNode with nil Err", ErrInvalidFilter))
		}
		return triUnknown, unsupported(n.Err)
	}
	return triUnknown, unsupported(fmt.Errorf("%w: unknown type %T", ErrInvalidFilter, f))
}

// junction evaluates the children of the AND or OR node named name. dominant
// is the value that decides the result: false for AND, true for OR. Once
// the result is decided, the remaining children are only checked for
// errors.
func (m *matcher[R]) junction(name string, fs []Filter, dominant tri, depth int, skip bool) (tri, error) {
	result := -dominant // the identity: true for AND, false for OR
	for i, f := range fs {
		if f == nil {
			return triUnknown, unsupported(fmt.Errorf("%w: %s: child %d", ErrNilFilter, name, i))
		}
		t, err := m.eval(f, depth+1, skip || result == dominant)
		if err != nil {
			return triUnknown, err
		}
		if dominant == triFalse {
			result = min(result, t)
		} else {
			result = max(result, t)
		}
	}
	return result, nil
}

// id evaluates an IDNode. The ID comes from Lookup(r, IDPath); if it is
// missing or a JSON kind other than string, the result is unknown, as for
// any comparison on a missing field or a kind mismatch. A value that is not
// a JSON kind is an error.
func (m *matcher[R]) id(ids []string) (tri, error) {
	v, ok := m.db.Lookup(m.r, IDPath)
	if ok && kindOf(v) == kindForeign {
		return triUnknown, unsupported(fmt.Errorf("ID: %w", foreignValue(v)))
	}
	id, isString := v.(string)
	if !ok || !isString {
		return triUnknown, nil
	}
	return triOf(slices.Contains(ids, id)), nil
}
