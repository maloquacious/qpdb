// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

// page is the result of Query.Page.
type page struct {
	offset, limit int
	limited       bool
}

func pageOf(q *Query) page {
	o, l, ok := q.Page()
	return page{o, l, ok}
}

func TestQueryBuild(t *testing.T) {
	a := Eq(Path{"a"}, 1)
	b := Eq(Path{"b"}, 2)
	c := Eq(Path{"c"}, 3)
	var nilQ *Query
	tests := []struct {
		name   string
		q      *Query
		filter Filter
		order  []OrderKey
		page   page
	}{
		{"nil query", nil, And(), nil, page{}},
		{"Where()", Where(), And(), nil, page{}},
		{"Where one", Where(a), a, nil, page{}},
		{"Where two", Where(a, b), And(a, b), nil, page{}},
		{"Where chained", Where(a).Where(b, c), And(a, b, c), nil, page{}},
		{"nil receiver Where", nilQ.Where(a), a, nil, page{}},
		{"Where then empty Where", Where(a).Where(), a, nil, page{}},
		{"nil receiver OrderBy", nilQ.OrderBy(Asc(Path{"a"})), And(),
			[]OrderKey{{Path: Path{"a"}}}, page{}},
		{"OrderBy", Where().OrderBy(Asc(Path{"a"}), NaturalDesc(Path{"b", "c"})), And(),
			[]OrderKey{{Path: Path{"a"}}, {Path: Path{"b", "c"}, Desc: true, Natural: true}}, page{}},
		{"OrderBy replaces", Where().OrderBy(Asc(Path{"a"})).OrderBy(Desc(Path{"b"})), And(),
			[]OrderKey{{Path: Path{"b"}, Desc: true}}, page{}},
		{"OrderBy() clears", Where().OrderBy(Asc(Path{"a"})).OrderBy(), And(), nil, page{}},
		{"OrderBy IDPath", Where().OrderBy(Asc(IDPath)), And(), []OrderKey{{Path: IDPath}}, page{}},
		{"nil receiver Offset", nilQ.Offset(3), And(), nil, page{offset: 3}},
		{"nil receiver Limit", nilQ.Limit(3), And(), nil, page{limit: 3, limited: true}},
		{"Offset", Where().Offset(5), And(), nil, page{offset: 5}},
		{"Offset 0", Where().Offset(0), And(), nil, page{}},
		{"Limit", Where().Limit(5), And(), nil, page{limit: 5, limited: true}},
		{"Limit 0", Where().Limit(0), And(), nil, page{limit: 0, limited: true}},
		{"Offset replaces", Where().Offset(5).Offset(2), And(), nil, page{offset: 2}},
		{"Limit replaces", Where().Limit(5).Limit(2), And(), nil, page{limit: 2, limited: true}},
		{"Limit replaced by 0", Where().Limit(5).Limit(0), And(), nil, page{limit: 0, limited: true}},
		{"all", Where(a).OrderBy(Natural(Path{"n"})).Limit(10).Offset(20).Where(b), And(a, b),
			[]OrderKey{{Path: Path{"n"}, Natural: true}}, page{offset: 20, limit: 10, limited: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.q.Err(); err != nil {
				t.Fatalf("Err = %v", err)
			}
			if got := tt.q.Filter(); !reflect.DeepEqual(got, tt.filter) {
				t.Errorf("Filter = %#v, want %#v", got, tt.filter)
			}
			if got := tt.q.Order(); !reflect.DeepEqual(got, tt.order) {
				t.Errorf("Order = %v, want %v", got, tt.order)
			}
			if got := pageOf(tt.q); got != tt.page {
				t.Errorf("Page = %+v, want %+v", got, tt.page)
			}
		})
	}
}

func TestOrderKeys(t *testing.T) {
	p := Path{"a", "b"}
	tests := []struct {
		key  OrderKey
		want OrderKey
		str  string
	}{
		{Asc(p), OrderKey{Path: p}, `Asc(["a" "b"])`},
		{Desc(p), OrderKey{Path: p, Desc: true}, `Desc(["a" "b"])`},
		{Natural(p), OrderKey{Path: p, Natural: true}, `Natural(["a" "b"])`},
		{NaturalDesc(p), OrderKey{Path: p, Desc: true, Natural: true}, `NaturalDesc(["a" "b"])`},
	}
	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if !reflect.DeepEqual(tt.key, tt.want) {
				t.Errorf("key = %#v, want %#v", tt.key, tt.want)
			}
			if got := tt.key.String(); got != tt.str {
				t.Errorf("String = %q, want %q", got, tt.str)
			}
		})
	}

	// The constructors copy the path.
	src := Path{"x"}
	keys := []OrderKey{Asc(src), Desc(src), Natural(src), NaturalDesc(src)}
	src[0] = "changed"
	for _, k := range keys {
		if k.Path[0] != "x" {
			t.Errorf("%s: path aliases its argument", k)
		}
	}
}

func TestQueryBuildErrors(t *testing.T) {
	ok := Eq(Path{"a"}, 1)
	tests := []struct {
		name string
		q    *Query
		want error
	}{
		{"empty path", Where(Eq(Path{}, 1)), ErrEmptyPath},
		{"nil path", Where(ok, Like(nil, "x")), ErrEmptyPath},
		{"nested empty path", Where(And(ok, Not(IsNull(Path{})))), ErrEmptyPath},
		{"nil regexp", Where(Regex(Path{"a"}, nil)), ErrNilRegexp},
		{"nil filter", Where(ok, nil), ErrNilFilter},
		{"nil filter chained", Where(ok).Where(nil), ErrNilFilter},
		{"invalid operand", Where(Has(Path{"a"}, []int{1})), ErrInvalidOperand},
		{"invalid filter", Where(&CompareNode{}), ErrInvalidFilter},
		{"negative offset", Where().Offset(-1), ErrNegativeOffset},
		{"negative limit", Where().Limit(-1), ErrNegativeLimit},
		{"nil receiver negative offset", (*Query)(nil).Offset(-1), ErrNegativeOffset},
		{"empty Asc key", Where().OrderBy(Asc(Path{})), ErrEmptyPath},
		{"empty Desc key", Where().OrderBy(Asc(Path{"a"}), Desc(nil)), ErrEmptyPath},
		{"empty Natural key", Where().OrderBy(Natural(nil)), ErrEmptyPath},
		{"empty NaturalDesc key", Where().OrderBy(NaturalDesc(nil)), ErrEmptyPath},
		{"zero key", Where().OrderBy(OrderKey{}), ErrEmptyPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.q.Err(); !errors.Is(err, tt.want) {
				t.Errorf("Err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestQueryBuildErrorMessages(t *testing.T) {
	tests := []struct {
		q    *Query
		want string
	}{
		{Where(Eq(Path{"a"}, 1), nil), "qpdb: nil filter: Where: filter 1"},
		{Where().Offset(-2), "qpdb: negative offset: Offset(-2)"},
		{Where().Limit(-3), "qpdb: negative limit: Limit(-3)"},
		{Where().OrderBy(Asc(Path{"a"}), Desc(nil)), `qpdb: empty path: OrderBy: key 1 (Desc([]))`},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if err := tt.q.Err(); err == nil || err.Error() != tt.want {
				t.Errorf("Err = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestQueryStickyError checks that a query holding an error ignores every
// later builder call, while still returning a new *Query.
func TestQueryStickyError(t *testing.T) {
	a := Eq(Path{"a"}, 1)
	base := Where(a).OrderBy(Asc(Path{"a"})).Offset(1).Limit(2)
	bad := base.Where(Regex(Path{"r"}, nil))
	first := bad.Err()
	if !errors.Is(first, ErrNilRegexp) {
		t.Fatalf("Err = %v, want ErrNilRegexp", first)
	}
	tests := []struct {
		name string
		q    *Query
	}{
		{"Where valid", bad.Where(Eq(Path{"b"}, 2))},
		{"Where with another error", bad.Where(Eq(Path{}, 2))},
		{"Where nil", bad.Where(nil)},
		{"OrderBy valid", bad.OrderBy(Desc(Path{"z"}))},
		{"OrderBy() clear", bad.OrderBy()},
		{"OrderBy empty key", bad.OrderBy(Asc(nil))},
		{"Offset valid", bad.Offset(9)},
		{"Offset negative", bad.Offset(-1)},
		{"Limit valid", bad.Limit(9)},
		{"Limit negative", bad.Limit(-1)},
		{"chain", bad.Limit(-1).Offset(-1).Where(nil).OrderBy(Asc(Path{"q"})).Limit(3)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.q == bad {
				t.Fatal("builder returned its receiver")
			}
			if err := tt.q.Err(); err != first {
				t.Errorf("Err = %v, want %v", err, first)
			}
			if got := tt.q.Filter(); !reflect.DeepEqual(got, a) {
				t.Errorf("Filter = %#v, want %#v", got, a)
			}
			if got, want := tt.q.Order(), []OrderKey{Asc(Path{"a"})}; !reflect.DeepEqual(got, want) {
				t.Errorf("Order = %v, want %v", got, want)
			}
			if got, want := pageOf(tt.q), (page{1, 2, true}); got != want {
				t.Errorf("Page = %+v, want %+v", got, want)
			}
		})
	}

	// The first error wins within a single call, too.
	q := Where(Eq(Path{}, 1), Regex(Path{"r"}, nil))
	if err := q.Err(); !errors.Is(err, ErrEmptyPath) {
		t.Errorf("Err = %v, want ErrEmptyPath", err)
	}
	// A failed call adds none of its filters.
	q = Where(a).Where(Eq(Path{"b"}, 2), nil)
	if got := q.Filter(); !reflect.DeepEqual(got, a) {
		t.Errorf("Filter after failed Where = %#v, want %#v", got, a)
	}
}

// TestQueryImmutable checks that builder methods leave the receiver
// unchanged and that sibling queries do not share mutable state.
func TestQueryImmutable(t *testing.T) {
	a := Eq(Path{"a"}, 1)
	b := Eq(Path{"b"}, 2)
	c := Eq(Path{"c"}, 3)

	t.Run("receiver unchanged", func(t *testing.T) {
		base := Where(a)
		calls := []struct {
			name string
			q    *Query
		}{
			{"Where", base.Where(b)},
			{"OrderBy", base.OrderBy(Asc(Path{"x"}))},
			{"Offset", base.Offset(1)},
			{"Limit", base.Limit(1)},
			{"Where error", base.Where(nil)},
			{"Offset error", base.Offset(-1)},
		}
		for _, call := range calls {
			if call.q == base {
				t.Errorf("%s returned its receiver", call.name)
			}
		}
		if err := base.Err(); err != nil {
			t.Errorf("base.Err = %v", err)
		}
		if got := base.Filter(); !reflect.DeepEqual(got, a) {
			t.Errorf("base.Filter = %#v, want %#v", got, a)
		}
		if got := base.Order(); got != nil {
			t.Errorf("base.Order = %v, want nil", got)
		}
		if got := pageOf(base); got != (page{}) {
			t.Errorf("base.Page = %+v, want zero", got)
		}
	})

	t.Run("sibling Where", func(t *testing.T) {
		// Grow base by repeated Where, as append would give it spare
		// capacity that siblings could share.
		base := Where(a)
		for range 5 {
			base = base.Where(a)
		}
		q1 := base.Where(b)
		q2 := base.Where(c)
		want1 := And(a, a, a, a, a, a, b)
		want2 := And(a, a, a, a, a, a, c)
		if got := q1.Filter(); !reflect.DeepEqual(got, want1) {
			t.Errorf("q1.Filter = %#v, want %#v", got, want1)
		}
		if got := q2.Filter(); !reflect.DeepEqual(got, want2) {
			t.Errorf("q2.Filter = %#v, want %#v", got, want2)
		}
	})

	t.Run("sibling Where from Where", func(t *testing.T) {
		base := Where(a)
		q1 := base.Where(b)
		q2 := base.Where(c)
		if got, want := q1.Filter(), And(a, b); !reflect.DeepEqual(got, want) {
			t.Errorf("q1.Filter = %#v, want %#v", got, want)
		}
		if got, want := q2.Filter(), And(a, c); !reflect.DeepEqual(got, want) {
			t.Errorf("q2.Filter = %#v, want %#v", got, want)
		}
	})

	t.Run("caller filter slice", func(t *testing.T) {
		fs := []Filter{a, b}
		q := Where(fs...)
		fs[0] = c
		if got, want := q.Filter(), And(a, b); !reflect.DeepEqual(got, want) {
			t.Errorf("Filter = %#v, want %#v", got, want)
		}
	})

	t.Run("caller key slice and path", func(t *testing.T) {
		p := Path{"k"}
		keys := []OrderKey{{Path: p}}
		q := Where().OrderBy(keys...)
		keys[0].Desc = true
		p[0] = "changed"
		if got, want := q.Order(), []OrderKey{{Path: Path{"k"}}}; !reflect.DeepEqual(got, want) {
			t.Errorf("Order = %v, want %v", got, want)
		}
	})

	t.Run("Order returns a copy", func(t *testing.T) {
		q := Where().OrderBy(Asc(Path{"k"}))
		got := q.Order()
		got[0].Desc = true
		got[0].Path[0] = "changed"
		if got, want := q.Order(), []OrderKey{Asc(Path{"k"})}; !reflect.DeepEqual(got, want) {
			t.Errorf("Order = %v, want %v", got, want)
		}
	})

	t.Run("Filter returns a copy of the list", func(t *testing.T) {
		q := Where(a, b)
		f := q.Filter().(AndNode)
		f.Filters[0] = c
		if got, want := q.Filter(), And(a, b); !reflect.DeepEqual(got, want) {
			t.Errorf("Filter = %#v, want %#v", got, want)
		}
	})
}

// TestQueryZeroValue checks that the zero Query is a valid query, like a nil
// *Query.
func TestQueryZeroValue(t *testing.T) {
	a := Eq(Path{"a"}, 1)
	var z Query
	if err := z.Err(); err != nil {
		t.Errorf("Err = %v", err)
	}
	if got := z.Filter(); !reflect.DeepEqual(got, And()) {
		t.Errorf("Filter = %#v, want And()", got)
	}
	if got := z.Order(); got != nil {
		t.Errorf("Order = %v, want nil", got)
	}
	if got := pageOf(&z); got != (page{}) {
		t.Errorf("Page = %+v, want zero", got)
	}
	tests := []struct {
		name   string
		q      *Query
		filter Filter
		order  []OrderKey
		page   page
	}{
		{"Where", z.Where(a), a, nil, page{}},
		{"OrderBy", z.OrderBy(Asc(Path{"a"})), And(), []OrderKey{Asc(Path{"a"})}, page{}},
		{"OrderBy()", z.OrderBy(), And(), nil, page{}},
		{"Offset", z.Offset(2), And(), nil, page{offset: 2}},
		{"Limit", z.Limit(0), And(), nil, page{limited: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.q == &z {
				t.Fatal("builder returned its receiver")
			}
			if err := tt.q.Err(); err != nil {
				t.Fatalf("Err = %v", err)
			}
			if got := tt.q.Filter(); !reflect.DeepEqual(got, tt.filter) {
				t.Errorf("Filter = %#v, want %#v", got, tt.filter)
			}
			if got := tt.q.Order(); !reflect.DeepEqual(got, tt.order) {
				t.Errorf("Order = %v, want %v", got, tt.order)
			}
			if got := pageOf(tt.q); got != tt.page {
				t.Errorf("Page = %+v, want %+v", got, tt.page)
			}
		})
	}
	if !reflect.DeepEqual(z, Query{}) {
		t.Errorf("zero Query changed: %#v", z)
	}
}

// TestQueryErrorOrder checks that the first error across chained calls is
// the one reported.
func TestQueryErrorOrder(t *testing.T) {
	tests := []struct {
		name string
		q    *Query
		want error
	}{
		{"Offset then Limit", Where().Offset(-1).Limit(-1), ErrNegativeOffset},
		{"Limit then Offset", Where().Limit(-1).Offset(-1), ErrNegativeLimit},
		{"Where then Offset", Where(nil).Offset(-1), ErrNilFilter},
		{"OrderBy then Where", Where().OrderBy(Asc(nil)).Where(Regex(Path{"a"}, nil)), ErrEmptyPath},
		{"Limit then OrderBy", Where().Limit(-1).OrderBy(Asc(nil)), ErrNegativeLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.q.Err()
			if !errors.Is(err, tt.want) {
				t.Errorf("Err = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestQueryConcurrentDerive derives queries from one shared parent in
// parallel. Run with -race to detect shared mutable state.
func TestQueryConcurrentDerive(t *testing.T) {
	a := Eq(Path{"a"}, 1)
	parent := Where(a).OrderBy(Asc(Path{"a"})).Offset(1).Limit(2)
	const n = 16
	got := make([]*Query, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			q := parent.Where(Eq(Path{"b"}, i)).OrderBy(Desc(Path{"b"})).Offset(i).Limit(i)
			_ = parent.Filter()
			_ = parent.Order()
			_, _, _ = parent.Page()
			got[i] = q
		})
	}
	wg.Wait()
	for i, q := range got {
		if err := q.Err(); err != nil {
			t.Fatalf("query %d: Err = %v", i, err)
		}
		if want := And(a, Eq(Path{"b"}, i)); !reflect.DeepEqual(q.Filter(), want) {
			t.Errorf("query %d: Filter = %#v, want %#v", i, q.Filter(), want)
		}
		if want := (page{i, i, true}); pageOf(q) != want {
			t.Errorf("query %d: Page = %+v, want %+v", i, pageOf(q), want)
		}
	}
	if !reflect.DeepEqual(parent.Filter(), a) || pageOf(parent) != (page{1, 2, true}) {
		t.Errorf("parent changed")
	}
}
