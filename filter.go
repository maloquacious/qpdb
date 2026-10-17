// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"errors"
	"fmt"
)

// Build errors recorded by filter constructors. A recorded error wraps one of
// these, so callers can test for them with [errors.Is].
var (
	// ErrEmptyPath reports a filter built with an empty or nil [Path].
	ErrEmptyPath = errors.New("qpdb: empty path")
	// ErrNilRegexp reports a [Regex] filter built with a nil *regexp.Regexp.
	ErrNilRegexp = errors.New("qpdb: nil regular expression")
	// ErrNilFilter reports a nil [Filter] passed to [And], [Or], or [Not],
	// or a nil predicate passed to [Func].
	ErrNilFilter = errors.New("qpdb: nil filter")
	// ErrInvalidOperand reports an operand that is not allowed for its
	// operator: a value that is not a JSON kind, NaN or ±Inf, a json.Number,
	// an array passed to or nested inside the operand of [Has], a null
	// operand, or a non-array passed to [SameArray] and the like.
	ErrInvalidOperand = errors.New("qpdb: invalid operand")
	// ErrInvalidFilter reports a tree that [FilterErr] cannot walk: a
	// [Filter] whose dynamic type is not one of the node types listed on
	// Filter (such as a pointer to a node), an [ErrorNode] with a nil Err,
	// or a tree nested more than 1000 deep (usually a cycle built with
	// composite literals). [Match] also reports a [CompareNode] with an
	// unknown Op with it.
	ErrInvalidFilter = errors.New("qpdb: invalid filter")
)

// Filter is a node in a filter tree.
//
// The node types are [AndNode], [OrNode], [NotNode], [CompareNode],
// [IDNode], [FuncNode], and [ErrorNode]. The set is closed; implementations
// walk a tree with a type switch over these types.
//
// Build filters with the constructors ([And], [Eq], [ID], and so on) rather
// than composite literals: the constructors validate and copy their input,
// and record any error in the tree (see [FilterErr]). The tree holds node
// values, not pointers; a pointer to a node also satisfies Filter, but
// [FilterErr] reports it as [ErrInvalidFilter].
//
// Compare filters with [reflect.DeepEqual], not ==. Most node types hold
// slices, maps, or funcs, so == on two Filters can panic. A tree containing
// a [FuncNode] with a non-nil Fn never compares equal under
// reflect.DeepEqual, not even to itself, because DeepEqual treats non-nil
// funcs as unequal.
type Filter interface {
	isFilter()
}

// AndNode matches when every child matches. With no children it matches
// every record. Built by [And].
type AndNode struct {
	Filters []Filter
}

// OrNode matches when any child matches. With no children it matches no
// record. Built by [Or].
type OrNode struct {
	Filters []Filter
}

// NotNode negates its child under three-valued logic. Built by [Not].
type NotNode struct {
	Filter Filter
}

// CompareNode is one test on one path. Built by the operator constructors
// such as [Eq], [Like], and [Has].
//
// Operand holds the single operand of Eq, Ne, Lt, Lte, Gt, Gte, Contains,
// the string operators, Regex, Has, SameArray, and SameObject. Operands holds
// the operands of In, HasAny, and HasAll. The type-test operators (IsNull and
// the like) use neither.
//
// Operand values are normalized: numbers are float64, arrays are []any, and
// objects are map[string]any (see [Value] and [Has]). Regex stores a
// *regexp.Regexp in Operand.
type CompareNode struct {
	Op       Op
	Path     Path
	Operand  any
	Operands []any
}

// IDNode matches when the record's ID is one of IDs. With no IDs it matches
// no record. Built by [ID].
type IDNode struct {
	IDs []string
}

// FuncNode matches when Fn returns true. Fn receives the record as any and
// asserts it to the record type. Built by [Func].
type FuncNode struct {
	Fn func(any) bool
}

// ErrorNode records a build error from a filter constructor that was given
// bad input. [FilterErr] reports it.
type ErrorNode struct {
	Err error
}

func (AndNode) isFilter()     {}
func (OrNode) isFilter()      {}
func (NotNode) isFilter()     {}
func (CompareNode) isFilter() {}
func (IDNode) isFilter()      {}
func (FuncNode) isFilter()    {}
func (ErrorNode) isFilter()   {}

// FilterErr returns the first build error recorded in the tree rooted at f,
// or nil if there is none. It walks the tree depth first, left to right, so
// an [AndNode], [OrNode], or [NotNode] reports the first error among its
// children.
//
// A nil f, or a nil child of an [AndNode], [OrNode], or [NotNode], is
// reported as [ErrNilFilter]; for a child, the error names its position. A
// Filter of any type other than the node types listed on [Filter], including
// a pointer to a node (nil or not), an [ErrorNode] with a nil Err, and a
// tree nested more than 1000 deep are reported as [ErrInvalidFilter], so
// FilterErr terminates even on a cyclic tree. FilterErr does not otherwise
// validate nodes built with composite literals rather than constructors.
func FilterErr(f Filter) error {
	return filterErr(f, 0)
}

func filterErr(f Filter, depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("%w: nested more than %d deep", ErrInvalidFilter, maxDepth)
	}
	switch n := f.(type) {
	case nil:
		return ErrNilFilter
	case ErrorNode:
		if n.Err == nil {
			return fmt.Errorf("%w: ErrorNode with nil Err", ErrInvalidFilter)
		}
		return n.Err
	case AndNode:
		return childrenErr("And", n.Filters, depth)
	case OrNode:
		return childrenErr("Or", n.Filters, depth)
	case NotNode:
		if n.Filter == nil {
			return fmt.Errorf("%w: Not: child", ErrNilFilter)
		}
		return filterErr(n.Filter, depth+1)
	case CompareNode, IDNode, FuncNode:
		return nil
	}
	return fmt.Errorf("%w: unknown type %T", ErrInvalidFilter, f)
}

// childrenErr returns the first error among the children, at depth+1, of
// the node named name.
func childrenErr(name string, fs []Filter, depth int) error {
	for i, f := range fs {
		if f == nil {
			return fmt.Errorf("%w: %s: child %d", ErrNilFilter, name, i)
		}
		if err := filterErr(f, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// Op identifies the operator of a [CompareNode].
type Op int

// The operators. Each is named after its constructor.
const (
	OpEq Op = iota + 1
	OpNe
	OpIn
	OpLt
	OpLte
	OpGt
	OpGte
	OpEqFold
	OpLike
	OpLikeFold
	OpPrefix
	OpPrefixFold
	OpSuffix
	OpSuffixFold
	OpRegex
	OpContains
	OpHas
	OpHasAny
	OpHasAll
	OpSameArray
	OpSameObject
	OpIsNull
	OpIsBool
	OpIsNumber
	OpIsString
	OpIsArray
	OpIsObject
)

var opNames = [...]string{
	OpEq:         "Eq",
	OpNe:         "Ne",
	OpIn:         "In",
	OpLt:         "Lt",
	OpLte:        "Lte",
	OpGt:         "Gt",
	OpGte:        "Gte",
	OpEqFold:     "EqFold",
	OpLike:       "Like",
	OpLikeFold:   "LikeFold",
	OpPrefix:     "Prefix",
	OpPrefixFold: "PrefixFold",
	OpSuffix:     "Suffix",
	OpSuffixFold: "SuffixFold",
	OpRegex:      "Regex",
	OpContains:   "Contains",
	OpHas:        "Has",
	OpHasAny:     "HasAny",
	OpHasAll:     "HasAll",
	OpSameArray:  "SameArray",
	OpSameObject: "SameObject",
	OpIsNull:     "IsNull",
	OpIsBool:     "IsBool",
	OpIsNumber:   "IsNumber",
	OpIsString:   "IsString",
	OpIsArray:    "IsArray",
	OpIsObject:   "IsObject",
}

// String returns the name of the operator's constructor, such as "Eq".
func (op Op) String() string {
	if op > 0 && int(op) < len(opNames) {
		return opNames[op]
	}
	return fmt.Sprintf("Op(%d)", int(op))
}
