// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestOperatorConstructors(t *testing.T) {
	p := Path{"f"}
	re := regexp.MustCompile(`^a`)
	tests := []struct {
		name     string
		f        Filter
		op       Op
		operand  any
		operands []any
	}{
		{"Eq", Eq(p, 1), OpEq, 1.0, nil},
		{"Ne", Ne(p, "a"), OpNe, "a", nil},
		{"In", In(p, 1, 2), OpIn, nil, []any{1.0, 2.0}},
		{"Lt", Lt(p, 1), OpLt, 1.0, nil},
		{"Lte", Lte(p, 1), OpLte, 1.0, nil},
		{"Gt", Gt(p, 1), OpGt, 1.0, nil},
		{"Gte", Gte(p, 1), OpGte, 1.0, nil},
		{"EqFold", EqFold(p, "A"), OpEqFold, "A", nil},
		{"Like", Like(p, "a"), OpLike, "a", nil},
		{"LikeFold", LikeFold(p, "a"), OpLikeFold, "a", nil},
		{"Prefix", Prefix(p, "a"), OpPrefix, "a", nil},
		{"PrefixFold", PrefixFold(p, "a"), OpPrefixFold, "a", nil},
		{"Suffix", Suffix(p, "a"), OpSuffix, "a", nil},
		{"SuffixFold", SuffixFold(p, "a"), OpSuffixFold, "a", nil},
		{"Regex", Regex(p, re), OpRegex, re, nil},
		{"Contains", Contains(p, "a"), OpContains, "a", nil},
		{"Has scalar", Has(p, 1), OpHas, 1.0, nil},
		{"Has object", Has(p, map[string]int{"a": 1}), OpHas, map[string]any{"a": 1.0}, nil},
		{"Has nested object", Has(p, map[string]map[string]int{"a": {"b": 1}}), OpHas,
			map[string]any{"a": map[string]any{"b": 1.0}}, nil},
		{"Has object with null value", Has(p, map[string]any{"a": nil}), OpHas, map[string]any{"a": nil}, nil},
		{"HasAny", HasAny(p, "a", 2, map[string]bool{"b": true}), OpHasAny, nil,
			[]any{"a", 2.0, map[string]any{"b": true}}},
		{"HasAny none", HasAny(p), OpHasAny, nil, []any{}},
		{"HasAll", HasAll(p, "a", myInt(2)), OpHasAll, nil, []any{"a", 2.0}},
		{"HasAll none", HasAll(p), OpHasAll, nil, []any{}},
		{"SameArray", SameArray(p, []int{1, 2}), OpSameArray, []any{1.0, 2.0}, nil},
		{"SameObject", SameObject(p, map[string]string{"a": "b"}), OpSameObject, map[string]any{"a": "b"}, nil},
		{"SameObject with array value", SameObject(p, map[string][]string{"a": {"x"}}), OpSameObject,
			map[string]any{"a": []any{"x"}}, nil},
		{"SameArray nested", SameArray(p, [][]int{{1}, {}}), OpSameArray, []any{[]any{1.0}, []any{}}, nil},
		{"IsNull", IsNull(p), OpIsNull, nil, nil},
		{"IsBool", IsBool(p), OpIsBool, nil, nil},
		{"IsNumber", IsNumber(p), OpIsNumber, nil, nil},
		{"IsString", IsString(p), OpIsString, nil, nil},
		{"IsArray", IsArray(p), OpIsArray, nil, nil},
		{"IsObject", IsObject(p), OpIsObject, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := FilterErr(tt.f); err != nil {
				t.Fatalf("FilterErr = %v", err)
			}
			n, ok := tt.f.(CompareNode)
			if !ok {
				t.Fatalf("got %T, want CompareNode", tt.f)
			}
			if n.Op != tt.op || n.Op.String() != strings.Fields(tt.name)[0] {
				t.Errorf("Op = %v, want %v", n.Op, tt.op)
			}
			if !reflect.DeepEqual(n.Path, p) {
				t.Errorf("Path = %q, want %q", []string(n.Path), []string(p))
			}
			if !reflect.DeepEqual(n.Operand, tt.operand) {
				t.Errorf("Operand = %#v, want %#v", n.Operand, tt.operand)
			}
			if !reflect.DeepEqual(n.Operands, tt.operands) {
				t.Errorf("Operands = %#v, want %#v", n.Operands, tt.operands)
			}
		})
	}
}

func TestCompositeConstructors(t *testing.T) {
	a, b := Eq(Path{"a"}, 1), Eq(Path{"b"}, 2)
	fn := func(any) bool { return true }
	tests := []struct {
		name string
		f    Filter
		want Filter
	}{
		{"And", And(a, b), AndNode{Filters: []Filter{a, b}}},
		{"And empty", And(), AndNode{}},
		{"Or", Or(a, b), OrNode{Filters: []Filter{a, b}}},
		{"Or empty", Or(), OrNode{}},
		{"Not", Not(a), NotNode{Filter: a}},
		{"NotIn", NotIn(Path{"a"}, 1, 2), NotNode{Filter: In(Path{"a"}, 1, 2)}},
		{"ID", ID("x", "y"), IDNode{IDs: []string{"x", "y"}}},
		{"ID empty", ID(), IDNode{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := FilterErr(tt.f); err != nil {
				t.Fatalf("FilterErr = %v", err)
			}
			if !reflect.DeepEqual(tt.f, tt.want) {
				t.Errorf("got %#v, want %#v", tt.f, tt.want)
			}
		})
	}

	f, ok := Func(fn).(FuncNode)
	if !ok || f.Fn == nil {
		t.Errorf("Func(fn) = %#v, want FuncNode with Fn set", f)
	}
}

func TestCompositeArgumentsCopied(t *testing.T) {
	fs := []Filter{Eq(Path{"a"}, 1)}
	and := And(fs...).(AndNode)
	or := Or(fs...).(OrNode)
	ids := []string{"x"}
	id := ID(ids...).(IDNode)
	fs[0] = nil
	ids[0] = "y"
	if and.Filters[0] == nil || or.Filters[0] == nil || id.IDs[0] != "x" {
		t.Errorf("composite shares the caller's slice")
	}
}

func TestBuildErrors(t *testing.T) {
	p := Path{"f"}
	ch := make(chan int)
	tests := []struct {
		name    string
		f       Filter
		wantErr error
		wantMsg string // substring of the error text
	}{
		{"Eq empty path", Eq(nil, 1), ErrEmptyPath, "Eq"},
		{"In empty path", In(Path{}, 1), ErrEmptyPath, "In"},
		{"NotIn empty path", NotIn(Path{}, 1), ErrEmptyPath, "In"},
		{"Like empty path", Like(nil, "a"), ErrEmptyPath, "Like"},
		{"IsNull empty path", IsNull(nil), ErrEmptyPath, "IsNull"},
		{"Regex empty path", Regex(nil, regexp.MustCompile("a")), ErrEmptyPath, "Regex"},
		{"Regex empty path and nil", Regex(nil, nil), ErrEmptyPath, "Regex"},
		{"Regex nil", Regex(p, nil), ErrNilRegexp, "Regex"},
		{"Func nil", Func(nil), ErrNilFilter, "Func"},
		{"And nil child", And(Eq(p, 1), nil), ErrNilFilter, "And: child 1"},
		{"Or nil child", Or(nil), ErrNilFilter, "Or: child 0"},
		{"Not nil", Not(nil), ErrNilFilter, "Not: child"},
		{"nested nil child", Not(And(Or(Eq(p, 1), nil))), ErrNilFilter, "Or: child 1"},

		{"Has empty path", Has(nil, 1), ErrEmptyPath, "Has"},
		{"Has nil", Has(p, nil), ErrInvalidOperand, "null operand"},
		{"Has []any", Has(p, []any{1}), ErrInvalidOperand, "array operand"},
		{"Has []string", Has(p, []string{"a"}), ErrInvalidOperand, "array operand"},
		{"Has nil []string", Has(p, []string(nil)), ErrInvalidOperand, "array operand"},
		{"Has [1]int", Has(p, [1]int{1}), ErrInvalidOperand, "array operand"},
		{"Has struct", Has(p, struct{}{}), ErrInvalidOperand, "not a JSON kind"},
		{"Has nested chan", Has(p, map[string]any{"a": ch}), ErrInvalidOperand, `operand["a"]`},
		{"Has int keys", Has(p, map[int]int{}), ErrInvalidOperand, "string keys"},
		{"Has nested array", Has(p, map[string][]string{"a": {"x"}}), ErrInvalidOperand,
			`operand["a"]: array not allowed`},
		{"Has deeply nested array", Has(p, map[string]any{"a": map[string]any{"b": []any{}}}), ErrInvalidOperand,
			`operand["a"]["b"]: array not allowed`},

		{"HasAny empty path", HasAny(nil, 1), ErrEmptyPath, "HasAny"},
		{"HasAny slice element", HasAny(p, []string{"a", "b"}), ErrInvalidOperand, "element 0: array operand"},
		{"HasAny nil element", HasAny(p, "a", nil), ErrInvalidOperand, "element 1: null operand"},
		{"HasAny bad element", HasAny(p, "a", "b", ch), ErrInvalidOperand, "element 2"},
		{"HasAll slice element", HasAll(p, 1, []int{1}), ErrInvalidOperand, "HasAll"},
		{"HasAny object with array", HasAny(p, map[string]any{"a": []int{1}}), ErrInvalidOperand,
			`element 0: operand["a"]: array not allowed`},
		{"HasAll object with array", HasAll(p, "x", map[string][]int{"a": nil}), ErrInvalidOperand,
			`element 1: operand["a"]: array not allowed`},

		{"SameArray empty path", SameArray(nil, []int{}), ErrEmptyPath, "SameArray"},
		{"SameArray nil", SameArray(p, nil), ErrInvalidOperand, "not an array"},
		{"SameArray scalar", SameArray(p, 1), ErrInvalidOperand, "not an array"},
		{"SameArray object", SameArray(p, map[string]any{}), ErrInvalidOperand, "not an array"},
		{"SameArray bad element", SameArray(p, []any{ch}), ErrInvalidOperand, "operand[0]"},
		{"SameObject empty path", SameObject(nil, map[string]any{}), ErrEmptyPath, "SameObject"},
		{"SameObject nil", SameObject(p, nil), ErrInvalidOperand, "not an object"},
		{"SameObject array", SameObject(p, []any{}), ErrInvalidOperand, "not an object"},
		{"SameObject int keys", SameObject(p, map[int]any{}), ErrInvalidOperand, "string keys"},
		{"SameObject bad value", SameObject(p, map[string]any{"k": func() {}}), ErrInvalidOperand, `operand["k"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FilterErr(tt.f)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("FilterErr = %v, want %v", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err, tt.wantMsg)
			}
		})
	}
}

func TestSameArrayNilSliceIsEmpty(t *testing.T) {
	n := SameArray(Path{"f"}, []string(nil)).(CompareNode)
	if !reflect.DeepEqual(n.Operand, []any{}) {
		t.Errorf("Operand = %#v, want []any{}", n.Operand)
	}
	o := SameObject(Path{"f"}, map[string]int(nil)).(CompareNode)
	if !reflect.DeepEqual(o.Operand, map[string]any{}) {
		t.Errorf("Operand = %#v, want map[string]any{}", o.Operand)
	}
}

func TestFilterErrWalkOrder(t *testing.T) {
	ok := Eq(Path{"a"}, 1)
	errA := Eq(nil, 1)              // ErrEmptyPath
	errB := Regex(Path{"b"}, nil)   // ErrNilRegexp
	errC := Has(Path{"c"}, []int{}) // ErrInvalidOperand
	tests := []struct {
		name string
		f    Filter
		want error // nil means no error
	}{
		{"nil filter", nil, ErrNilFilter},
		{"valid leaf", ok, nil},
		{"ID", ID("x"), nil},
		{"Func", Func(func(any) bool { return false }), nil},
		{"And first error", And(ok, errB, errA), ErrNilRegexp},
		{"Or first error", Or(errC, errA), ErrInvalidOperand},
		{"Not propagates", Not(errA), ErrEmptyPath},
		{"depth first", And(Or(ok, Not(errC)), errA), ErrInvalidOperand},
		{"left to right across levels", Or(And(ok), Not(Or(ok, errB)), errC), ErrNilRegexp},
		{"nested valid", And(Or(ok, Not(ok)), ID(), And()), nil},
		{"zero ErrorNode", ErrorNode{}, ErrInvalidFilter},
		{"zero NotNode", NotNode{}, ErrNilFilter},
		{"pointer to AndNode", &AndNode{}, ErrInvalidFilter},
		{"pointer to CompareNode", &CompareNode{Op: OpEq, Path: Path{"a"}}, ErrInvalidFilter},
		{"typed nil *OrNode", (*OrNode)(nil), ErrInvalidFilter},
		{"typed nil *ErrorNode", (*ErrorNode)(nil), ErrInvalidFilter},
		{"pointer child", And(ok, &NotNode{Filter: ok}), ErrInvalidFilter},
		{"pointer under Not", Not((*IDNode)(nil)), ErrInvalidFilter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FilterErr(tt.f)
			if tt.want == nil {
				if err != nil {
					t.Errorf("FilterErr = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("FilterErr = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestFilterErrDepth(t *testing.T) {
	// A cyclic AndNode, built with composite literals.
	fs := make([]Filter, 1)
	cyclicAnd := AndNode{Filters: fs}
	fs[0] = cyclicAnd
	// A cyclic OrNode reached through a NotNode.
	gs := make([]Filter, 2)
	cyclicOr := OrNode{Filters: gs}
	gs[0] = Eq(Path{"a"}, 1)
	gs[1] = NotNode{Filter: cyclicOr}

	chain := func(n int) Filter {
		f := Eq(Path{"a"}, 1)
		for range n {
			f = Not(f)
		}
		return f
	}
	tests := []struct {
		name string
		f    Filter
		want error // nil means no error
	}{
		{"cyclic And", cyclicAnd, ErrInvalidFilter},
		{"cyclic Or via Not", cyclicOr, ErrInvalidFilter},
		{"cycle under And", And(Eq(Path{"a"}, 1), cyclicAnd), ErrInvalidFilter},
		{"deepest allowed", chain(maxDepth), nil},
		{"one too deep", chain(maxDepth + 1), ErrInvalidFilter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FilterErr(tt.f)
			if tt.want == nil {
				if err != nil {
					t.Errorf("FilterErr = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.want) || !strings.Contains(err.Error(), "nested more than 1000 deep") {
				t.Errorf("FilterErr = %v, want %v for depth", err, tt.want)
			}
		})
	}
}

func TestOpString(t *testing.T) {
	tests := []struct {
		op   Op
		want string
	}{
		{OpEq, "Eq"},
		{OpIsObject, "IsObject"},
		{Op(0), "Op(0)"},
		{Op(999), "Op(999)"},
	}
	for _, tt := range tests {
		if got := tt.op.String(); got != tt.want {
			t.Errorf("Op(%d).String() = %q, want %q", int(tt.op), got, tt.want)
		}
	}
}
