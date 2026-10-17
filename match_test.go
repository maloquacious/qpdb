// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"testing"
)

var errNotImplemented = errors.New("mapDB: not implemented")

// mapDB is a minimal Querier over map[string]any records for testing Match.
// Only Lookup is implemented.
type mapDB struct{}

func (mapDB) Get(context.Context, *Query) ([]map[string]any, error) {
	return nil, errNotImplemented
}

func (mapDB) Count(context.Context, *Query) (int, error) { return 0, errNotImplemented }

// Lookup resolves IDPath to the "___id" field and descends nested objects.
func (mapDB) Lookup(r map[string]any, p Path) (any, bool) {
	if p.IsID() {
		v, ok := r["___id"]
		return v, ok
	}
	var cur any = r
	for _, seg := range p {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = obj[seg]; !ok {
			return nil, false
		}
	}
	return cur, true
}

var _ Querier[map[string]any] = mapDB{}

// triMatch evaluates f against r through Match and returns its three-valued
// result: true if f matches, false if Not(f) matches, and unknown if
// neither does. It fails the test if both match or Match returns an error.
func triMatch(t *testing.T, f Filter, r map[string]any) tri {
	t.Helper()
	pos, err := Match(mapDB{}, f, r)
	if err != nil {
		t.Fatalf("Match(f) error: %v", err)
	}
	neg, err := Match(mapDB{}, Not(f), r)
	if err != nil {
		t.Fatalf("Match(Not(f)) error: %v", err)
	}
	switch {
	case pos && neg:
		t.Fatalf("both f and Not(f) match")
	case pos:
		return triTrue
	case neg:
		return triFalse
	}
	return triUnknown
}

// kindRecord has one field of each kind; "missing" is absent.
func kindRecord() map[string]any {
	return map[string]any{
		"null": nil,
		"bool": true,
		"num":  1.0,
		"str":  "abc",
		"arr":  []any{1.0, "a", true, nil, []any{"n"}, map[string]any{"k": "v"}},
		"obj": map[string]any{
			"k": "v",
			"n": nil,
			"o": map[string]any{"x": 1.0},
		},
	}
}

// kindFields are the fields of kindRecord in the column order of the want
// strings in TestMatchOperators.
var kindFields = [...]string{"missing", "null", "bool", "num", "str", "arr", "obj"}

func parseTri(c byte) tri {
	switch c {
	case 'T':
		return triTrue
	case 'F':
		return triFalse
	}
	return triUnknown
}

func TestMatchOperators(t *testing.T) {
	arrCopy := []any{1.0, "a", true, nil, []any{"n"}, map[string]any{"k": "v"}}
	objCopy := map[string]any{"k": "v", "n": nil, "o": map[string]any{"x": 1.0}}
	tests := []struct {
		name string
		f    func(p Path) Filter
		// want has one of T, F, or U for each of kindFields: missing,
		// null, bool, number, string, array, object.
		want string
	}{
		{"Eq 1", func(p Path) Filter { return Eq(p, 1) }, "UUUTUUU"},
		{"Eq 2", func(p Path) Filter { return Eq(p, 2) }, "UUUFUUU"},
		{"Eq abc", func(p Path) Filter { return Eq(p, "abc") }, "UUUUTUU"},
		{"Eq ABC", func(p Path) Filter { return Eq(p, "ABC") }, "UUUUFUU"},
		{"Eq true", func(p Path) Filter { return Eq(p, true) }, "UUTUUUU"},
		{"Eq false", func(p Path) Filter { return Eq(p, false) }, "UUFUUUU"},
		{"Eq 1 string", func(p Path) Filter { return Eq(p, "1") }, "UUUUFUU"},

		{"Ne 1", func(p Path) Filter { return Ne(p, 1) }, "UUUFUUU"},
		{"Ne 2", func(p Path) Filter { return Ne(p, 2) }, "UUUTUUU"},
		{"Ne x", func(p Path) Filter { return Ne(p, "x") }, "UUUUTUU"},
		{"Ne false", func(p Path) Filter { return Ne(p, false) }, "UUTUUUU"},

		{"In 1 2", func(p Path) Filter { return In(p, 1, 2) }, "UUUTUUU"},
		{"In 2 3", func(p Path) Filter { return In(p, 2, 3) }, "UUUFUUU"},
		{"In abc x", func(p Path) Filter { return In(p, "abc", "x") }, "UUUUTUU"},
		{"In true", func(p Path) Filter { return In(p, true) }, "UUTUUUU"},
		{"In none", func(p Path) Filter { return In[string](p) }, "UUFFFFF"},
		{"NotIn 2 3", func(p Path) Filter { return NotIn(p, 2, 3) }, "UUUTUUU"},
		{"NotIn 1", func(p Path) Filter { return NotIn(p, 1) }, "UUUFUUU"},
		{"NotIn none", func(p Path) Filter { return NotIn[string](p) }, "UUTTTTT"},

		{"Lt 2", func(p Path) Filter { return Lt(p, 2) }, "UUUTUUU"},
		{"Lt 1", func(p Path) Filter { return Lt(p, 1) }, "UUUFUUU"},
		{"Lte 1", func(p Path) Filter { return Lte(p, 1) }, "UUUTUUU"},
		{"Lte 0", func(p Path) Filter { return Lte(p, 0) }, "UUUFUUU"},
		{"Gt 0", func(p Path) Filter { return Gt(p, 0) }, "UUUTUUU"},
		{"Gt 1", func(p Path) Filter { return Gt(p, 1) }, "UUUFUUU"},
		{"Gte 1", func(p Path) Filter { return Gte(p, 1) }, "UUUTUUU"},
		{"Gte 1.5", func(p Path) Filter { return Gte(p, 1.5) }, "UUUFUUU"},
		{"Lt abd", func(p Path) Filter { return Lt(p, "abd") }, "UUUUTUU"},
		{"Lt abc", func(p Path) Filter { return Lt(p, "abc") }, "UUUUFUU"},
		{"Lte abc", func(p Path) Filter { return Lte(p, "abc") }, "UUUUTUU"},
		{"Gt ab", func(p Path) Filter { return Gt(p, "ab") }, "UUUUTUU"},
		{"Gt B", func(p Path) Filter { return Gt(p, "B") }, "UUUUTUU"}, // byte order: 'a' > 'B'
		{"Gte abd", func(p Path) Filter { return Gte(p, "abd") }, "UUUUFUU"},
		{"Lt true", func(p Path) Filter { return Lt(p, true) }, "UUUUUUU"},
		{"Lt string 5", func(p Path) Filter { return Lt(p, "5") }, "UUUUFUU"},

		{"EqFold ABC", func(p Path) Filter { return EqFold(p, "ABC") }, "UUUUTUU"},
		{"EqFold AB", func(p Path) Filter { return EqFold(p, "AB") }, "UUUUFUU"},
		{"Like b", func(p Path) Filter { return Like(p, "b") }, "UUUUTUU"},
		{"Like B", func(p Path) Filter { return Like(p, "B") }, "UUUUFUU"},
		{"Like empty", func(p Path) Filter { return Like(p, "") }, "UUUUTUU"},
		{"Like a on array", func(p Path) Filter { return Like(p, "a") }, "UUUUTUU"},
		{"LikeFold B", func(p Path) Filter { return LikeFold(p, "B") }, "UUUUTUU"},
		{"LikeFold X", func(p Path) Filter { return LikeFold(p, "X") }, "UUUUFUU"},
		{"Prefix ab", func(p Path) Filter { return Prefix(p, "ab") }, "UUUUTUU"},
		{"Prefix bc", func(p Path) Filter { return Prefix(p, "bc") }, "UUUUFUU"},
		{"PrefixFold AB", func(p Path) Filter { return PrefixFold(p, "AB") }, "UUUUTUU"},
		{"PrefixFold BC", func(p Path) Filter { return PrefixFold(p, "BC") }, "UUUUFUU"},
		{"Suffix bc", func(p Path) Filter { return Suffix(p, "bc") }, "UUUUTUU"},
		{"Suffix ab", func(p Path) Filter { return Suffix(p, "ab") }, "UUUUFUU"},
		{"SuffixFold BC", func(p Path) Filter { return SuffixFold(p, "BC") }, "UUUUTUU"},
		{"SuffixFold AB", func(p Path) Filter { return SuffixFold(p, "AB") }, "UUUUFUU"},
		{"Regex ^a", func(p Path) Filter { return Regex(p, regexp.MustCompile(`^a`)) }, "UUUUTUU"},
		{"Regex ^b", func(p Path) Filter { return Regex(p, regexp.MustCompile(`^b`)) }, "UUUUFUU"},
		{"Regex (?i)^A", func(p Path) Filter { return Regex(p, regexp.MustCompile(`(?i)^A`)) }, "UUUUTUU"},

		{"Contains 1", func(p Path) Filter { return Contains(p, 1) }, "UUUUUTU"},
		{"Contains 2", func(p Path) Filter { return Contains(p, 2) }, "UUUUUFU"},
		{"Contains a", func(p Path) Filter { return Contains(p, "a") }, "UUUUUTU"},
		{"Contains true", func(p Path) Filter { return Contains(p, true) }, "UUUUUTU"},
		{"Contains false", func(p Path) Filter { return Contains(p, false) }, "UUUUUFU"},
		{"Contains nested n", func(p Path) Filter { return Contains(p, "n") }, "UUUUUFU"},
		{"Contains k", func(p Path) Filter { return Contains(p, "k") }, "UUUUUFU"},

		{"Has 1", func(p Path) Filter { return Has(p, 1) }, "UUUTUTU"},
		{"Has 2", func(p Path) Filter { return Has(p, 2) }, "UUUFUFU"},
		{"Has true", func(p Path) Filter { return Has(p, true) }, "UUTUUTU"},
		{"Has false", func(p Path) Filter { return Has(p, false) }, "UUFUUFU"},
		{"Has abc", func(p Path) Filter { return Has(p, "abc") }, "UUUUTFF"},
		{"Has k", func(p Path) Filter { return Has(p, "k") }, "UUUUFFT"},
		{"Has null-valued key", func(p Path) Filter { return Has(p, "n") }, "UUUUFTT"},
		{"Has object", func(p Path) Filter { return Has(p, map[string]string{"k": "v"}) }, "UUUUUTT"},
		{"Has object mismatch", func(p Path) Filter { return Has(p, map[string]string{"k": "w"}) }, "UUUUUFF"},
		{"Has empty object", func(p Path) Filter { return Has(p, map[string]any{}) }, "UUUUUTT"},
		{"Has object null value", func(p Path) Filter { return Has(p, map[string]any{"n": nil}) }, "UUUUUFT"},
		{"Has object null vs string", func(p Path) Filter { return Has(p, map[string]any{"k": nil}) }, "UUUUUFF"},
		{"Has nested object", func(p Path) Filter {
			return Has(p, map[string]any{"o": map[string]any{"x": 1}})
		}, "UUUUUFT"},
		{"Has nested object mismatch", func(p Path) Filter {
			return Has(p, map[string]any{"o": map[string]any{"x": 2}})
		}, "UUUUUFF"},
		{"Has nested key", func(p Path) Filter { return Has(p, map[string]any{"o": "x"}) }, "UUUUUFT"},
		{"Has object under scalar", func(p Path) Filter {
			return Has(p, map[string]any{"k": map[string]any{"z": 1}})
		}, "UUUUUFF"},

		{"HasAny k zzz", func(p Path) Filter { return HasAny(p, "k", "zzz") }, "UUUUFFT"},
		{"HasAny zzz", func(p Path) Filter { return HasAny(p, "zzz") }, "UUUUFFF"},
		{"HasAny none", func(p Path) Filter { return HasAny(p) }, "UUFFFFF"},
		{"HasAny mixed", func(p Path) Filter { return HasAny(p, 1, "k") }, "UUUTUTT"},
		{"HasAll none", func(p Path) Filter { return HasAll(p) }, "UUTTTTT"},
		{"HasAll k n", func(p Path) Filter { return HasAll(p, "k", "n") }, "UUUUFFT"},
		{"HasAll mixed", func(p Path) Filter { return HasAll(p, 1, "a") }, "UUUUFTF"},
		{"HasAll 1 2", func(p Path) Filter { return HasAll(p, 1, 2) }, "UUUFUFU"},

		{"SameArray equal", func(p Path) Filter { return SameArray(p, arrCopy) }, "UUUUUTU"},
		{"SameArray prefix", func(p Path) Filter { return SameArray(p, []int{1}) }, "UUUUUFU"},
		{"SameArray empty", func(p Path) Filter { return SameArray(p, []int{}) }, "UUUUUFU"},
		{"SameArray reordered", func(p Path) Filter {
			return SameArray(p, []any{"a", 1.0, true, nil, []any{"n"}, map[string]any{"k": "v"}})
		}, "UUUUUFU"},
		{"SameObject equal", func(p Path) Filter { return SameObject(p, objCopy) }, "UUUUUUT"},
		{"SameObject subset", func(p Path) Filter { return SameObject(p, map[string]string{"k": "v"}) }, "UUUUUUF"},
		{"SameObject empty", func(p Path) Filter { return SameObject(p, map[string]any{}) }, "UUUUUUF"},

		{"IsNull", IsNull, "FTFFFFF"},
		{"IsBool", IsBool, "FFTFFFF"},
		{"IsNumber", IsNumber, "FFFTFFF"},
		{"IsString", IsString, "FFFFTFF"},
		{"IsArray", IsArray, "FFFFFTF"},
		{"IsObject", IsObject, "FFFFFFT"},
	}
	r := kindRecord()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.want) != len(kindFields) {
				t.Fatalf("want %q has %d columns, need %d", tt.want, len(tt.want), len(kindFields))
			}
			for i, field := range kindFields {
				f := tt.f(Path{field})
				if err := FilterErr(f); err != nil {
					t.Fatalf("FilterErr = %v", err)
				}
				if got, want := triMatch(t, f, r), parseTri(tt.want[i]); got != want {
					t.Errorf("%s field: got %v, want %v", field, got, want)
				}
			}
		})
	}
}

func TestMatchKleene(t *testing.T) {
	r := map[string]any{"x": 1.0}
	leaf := map[tri]Filter{
		triTrue:    Eq(Path{"x"}, 1),
		triFalse:   Eq(Path{"x"}, 2),
		triUnknown: Eq(Path{"missing"}, 1),
	}
	vals := []tri{triFalse, triUnknown, triTrue}
	for v, f := range leaf {
		if got := triMatch(t, f, r); got != v {
			t.Fatalf("leaf %v evaluates to %v", v, got)
		}
	}

	// Kleene truth tables, indexed [a][b] in the order of vals.
	and := [3][3]tri{
		{triFalse, triFalse, triFalse},
		{triFalse, triUnknown, triUnknown},
		{triFalse, triUnknown, triTrue},
	}
	or := [3][3]tri{
		{triFalse, triUnknown, triTrue},
		{triUnknown, triUnknown, triTrue},
		{triTrue, triTrue, triTrue},
	}
	for i, a := range vals {
		if got, want := triMatch(t, Not(leaf[a]), r), -a; got != want {
			t.Errorf("NOT %v = %v, want %v", a, got, want)
		}
		if got := triMatch(t, And(leaf[a]), r); got != a {
			t.Errorf("And(%v) = %v, want %v", a, got, a)
		}
		if got := triMatch(t, Or(leaf[a]), r); got != a {
			t.Errorf("Or(%v) = %v, want %v", a, got, a)
		}
		for j, b := range vals {
			if got := triMatch(t, And(leaf[a], leaf[b]), r); got != and[i][j] {
				t.Errorf("%v AND %v = %v, want %v", a, b, got, and[i][j])
			}
			if got := triMatch(t, Or(leaf[a], leaf[b]), r); got != or[i][j] {
				t.Errorf("%v OR %v = %v, want %v", a, b, got, or[i][j])
			}
		}
	}
	if got := triMatch(t, And(), r); got != triTrue {
		t.Errorf("And() = %v, want true", got)
	}
	if got := triMatch(t, Or(), r); got != triFalse {
		t.Errorf("Or() = %v, want false", got)
	}
	if got := triMatch(t, Not(Not(leaf[triUnknown])), r); got != triUnknown {
		t.Errorf("Not(Not(unknown)) = %v, want unknown", got)
	}
	if got := triMatch(t, And(leaf[triTrue], leaf[triTrue], leaf[triUnknown], leaf[triTrue]), r); got != triUnknown {
		t.Errorf("And(T, T, U, T) = %v, want unknown", got)
	}
	if got := triMatch(t, Or(leaf[triFalse], leaf[triUnknown], leaf[triFalse], leaf[triTrue]), r); got != triTrue {
		t.Errorf("Or(F, U, F, T) = %v, want true", got)
	}
}

// TestMatchConsequences checks the consequences listed in design §6.2.
func TestMatchConsequences(t *testing.T) {
	recs := map[string]map[string]any{
		"missing":  {},
		"null":     {"p": nil},
		"mismatch": {"p": "1"},
		"one":      {"p": 1.0},
		"two":      {"p": 2.0},
	}
	p := Path{"p"}
	tests := []struct {
		name string
		f    Filter
		want []string // the records that match
	}{
		{"Eq", Eq(p, 1), []string{"one"}},
		{"Not Eq", Not(Eq(p, 1)), []string{"two"}},
		{"Ne", Ne(p, 1), []string{"two"}},
		{"Not Ne", Not(Ne(p, 1)), []string{"one"}},
		{"NotIn", NotIn(p, 1), []string{"two"}},
		{"Not Lt", Not(Lt(p, 2)), []string{"two"}},
		{"IsNull", IsNull(p), []string{"null"}},
		{"Not IsNull", Not(IsNull(p)), []string{"missing", "mismatch", "one", "two"}},
		{"Not IsNumber", Not(IsNumber(p)), []string{"missing", "null", "mismatch"}},
		{"Not IsString", Not(IsString(p)), []string{"missing", "null", "one", "two"}},
		{"Eq or IsNull", Or(Eq(p, 1), IsNull(p)), []string{"null", "one"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, name := range []string{"missing", "null", "mismatch", "one", "two"} {
				ok, err := Match(mapDB{}, tt.f, recs[name])
				if err != nil {
					t.Fatalf("%s: Match error: %v", name, err)
				}
				if ok {
					got = append(got, name)
				}
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("matched %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatchPaths(t *testing.T) {
	r := map[string]any{
		"___id": "r1",
		"addr":  map[string]any{"city": "NYC", "geo": map[string]any{"lat": 40.7}},
		"a.b":   "dotted",
		"str":   "abc",
		"":      "empty key",
	}
	tests := []struct {
		name string
		f    Filter
		want tri
	}{
		{"nested", Eq(Path{"addr", "city"}, "NYC"), triTrue},
		{"nested false", Eq(Path{"addr", "city"}, "LA"), triFalse},
		{"deeply nested", Gt(Path{"addr", "geo", "lat"}, 40), triTrue},
		{"nested object", IsObject(Path{"addr", "geo"}), triTrue},
		{"missing leaf", Eq(Path{"addr", "zip"}, "1"), triUnknown},
		{"missing parent", Eq(Path{"nope", "city"}, "NYC"), triUnknown},
		{"through a string", Eq(Path{"str", "x"}, "abc"), triUnknown},
		{"through a string IsNull", IsNull(Path{"str", "x"}), triFalse},
		{"segment not split", Eq(Path{"a.b"}, "dotted"), triTrue},
		{"dotted is not nested", Eq(Path{"a", "b"}, "dotted"), triUnknown},
		{"empty segment", Eq(Path{""}, "empty key"), triTrue},
		{"IDPath Eq", Eq(IDPath, "r1"), triTrue},
		{"IDPath Prefix", Prefix(IDPath, "r"), triTrue},
		{"ID match", ID("r0", "r1"), triTrue},
		{"ID no match", ID("r2"), triFalse},
		{"ID none", ID(), triFalse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := triMatch(t, tt.f, r); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	// A record whose ID is missing or not a string: ID is unknown.
	for name, rec := range map[string]map[string]any{
		"missing ID": {},
		"null ID":    {"___id": nil},
		"number ID":  {"___id": 1.0},
	} {
		if got := triMatch(t, ID("r1"), rec); got != triUnknown {
			t.Errorf("%s: ID(r1) = %v, want unknown", name, got)
		}
		if got := triMatch(t, ID(), rec); got != triUnknown {
			t.Errorf("%s: ID() = %v, want unknown", name, got)
		}
	}
}

// foreign is a Go type that is not a JSON kind.
type foreign struct{ n int }

func TestMatchForeignValues(t *testing.T) {
	r := map[string]any{
		"int":     1,
		"float32": float32(1),
		"slice":   []string{"a"},
		"map":     map[string]int{"k": 1},
		"struct":  foreign{1},
		"ptr":     (*foreign)(nil),
		"arr":     []any{"a", 1},                    // the int is the second element
		"obj":     map[string]any{"a": "x", "k": 1}, // the int is under "k"
		"deep":    map[string]any{"o": map[string]any{"k": []string{"x"}}},
		"ok":      []any{"a"},
		"___id":   "r1",
	}
	tests := []struct {
		name   string
		f      Filter
		goType string // named in the error
	}{
		{"Eq int", Eq(Path{"int"}, 1), "int"},
		{"Eq float32", Eq(Path{"float32"}, 1), "float32"},
		{"IsNull int", IsNull(Path{"int"}), "int"},
		{"IsNumber int", IsNumber(Path{"int"}), "int"},
		{"IsArray slice", IsArray(Path{"slice"}), "[]string"},
		{"Like slice", Like(Path{"slice"}, "a"), "[]string"},
		{"HasAll none map", HasAll(Path{"map"}), "map[string]int"},
		{"IsObject struct", IsObject(Path{"struct"}), "qpdb.foreign"},
		{"IsNull typed nil pointer", IsNull(Path{"ptr"}), "*qpdb.foreign"},
		{"Contains reaches nested int", Contains(Path{"arr"}, 1), "int"},
		{"Has reaches nested int", Has(Path{"arr"}, "b"), "int"},
		{"SameArray reaches nested int", SameArray(Path{"arr"}, []any{"a", 1}), "int"},
		{"SameObject reaches nested int", SameObject(Path{"obj"}, map[string]any{"a": "x", "k": 1}), "int"},
		{"Has object reaches nested int", Has(Path{"obj"}, map[string]any{"k": 1}), "int"},
		{"Has reaches deeply nested slice", Has(Path{"deep"}, map[string]any{"o": map[string]any{"k": "x"}}), "[]string"},
		{"under Not", Not(Eq(Path{"int"}, 1)), "int"},
		{"in undecided And", And(Eq(Path{"ok"}, 1), IsNull(Path{"int"})), "int"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Match(mapDB{}, tt.f, r)
			if got || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("Match = %v, %v; want false, ErrUnsupported", got, err)
			}
			if !strings.Contains(err.Error(), "Go type "+tt.goType+" ") {
				t.Errorf("err = %v, want it to name Go type %s", err, tt.goType)
			}
		})
	}

	// Match inspects only what it needs: a foreign value it does not
	// reach is not an error.
	for _, tt := range []struct {
		name string
		f    Filter
		want tri
	}{
		{"missing field", Eq(Path{"none"}, 1), triUnknown},
		{"type test does not descend", IsArray(Path{"arr"}), triTrue},
		{"Contains stops at first match", Contains(Path{"arr"}, "a"), triTrue},
		{"Has stops at first match", Has(Path{"arr"}, "a"), triTrue},
		{"SameArray stops at first difference", SameArray(Path{"arr"}, []any{"b", 1}), triFalse},
		{"SameArray length differs", SameArray(Path{"arr"}, []any{"a"}), triFalse},
		{"SameObject stops at first sorted key", SameObject(Path{"obj"}, map[string]any{"a": "y", "k": 1}), triFalse},
		{"Has object visits only its keys", Has(Path{"obj"}, map[string]any{"a": "x"}), triTrue},
		{"Has object stops at first sorted key", Has(Path{"obj"}, map[string]any{"a": "y", "k": 1}), triFalse},
		{"Eq kind mismatch does not descend", Eq(Path{"arr"}, 1), triUnknown},
		{"decided And skips Lookup", And(Eq(Path{"___id"}, "r2"), Eq(Path{"int"}, 1)), triFalse},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := triMatch(t, tt.f, r); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	for _, id := range []any{1, int64(1), []byte("r1")} {
		ok, err := Match(mapDB{}, ID("r1"), map[string]any{"___id": id})
		if ok || !errors.Is(err, ErrUnsupported) {
			t.Errorf("ID with ___id %T: Match = %v, %v; want false, ErrUnsupported", id, ok, err)
		}
	}
}

func TestMatchNilCollections(t *testing.T) {
	// A nil []any is an empty array and a nil map[string]any an empty
	// object, not null.
	r := map[string]any{"a": []any(nil), "o": map[string]any(nil)}
	tests := []struct {
		name string
		f    Filter
		want tri
	}{
		{"IsNull array", IsNull(Path{"a"}), triFalse},
		{"IsArray", IsArray(Path{"a"}), triTrue},
		{"SameArray empty", SameArray(Path{"a"}, []int{}), triTrue},
		{"Contains", Contains(Path{"a"}, 1), triFalse},
		{"IsNull object", IsNull(Path{"o"}), triFalse},
		{"IsObject", IsObject(Path{"o"}), triTrue},
		{"SameObject empty", SameObject(Path{"o"}, map[string]any{}), triTrue},
		{"Has key", Has(Path{"o"}, "k"), triFalse},
		{"descend into nil object", IsNull(Path{"o", "k"}), triFalse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := triMatch(t, tt.f, r); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchHasNestedArrays(t *testing.T) {
	r := map[string]any{
		"a": []any{[]any{[]any{"deep"}}, []any{map[string]any{"k": 1.0}}},
		"o": map[string]any{
			"list":      []any{"x", "y"},
			"sub":       map[string]any{"k": nil},
			"nulls":     []any{1.0, nil},
			"deepNulls": []any{[]any{nil}},
		},
	}
	tests := []struct {
		name string
		f    Filter
		want tri
	}{
		{"scalar in nested arrays", Has(Path{"a"}, "deep"), triTrue},
		{"object in nested array", Has(Path{"a"}, map[string]any{"k": 1}), triTrue},
		{"absent in nested arrays", Has(Path{"a"}, "x"), triFalse},
		{"Contains does not descend", Contains(Path{"a"}, "deep"), triFalse},
		{"object value array has", Has(Path{"o"}, map[string]any{"list": "y"}), triTrue},
		{"object value array has not", Has(Path{"o"}, map[string]any{"list": "z"}), triFalse},
		{"null Has null", Has(Path{"o"}, map[string]any{"sub": map[string]any{"k": nil}}), triTrue},
		{"null key", Has(Path{"o"}, map[string]any{"sub": "k"}), triTrue},
		{"null in array", Has(Path{"o"}, map[string]any{"nulls": nil}), triTrue},
		{"null in nested array", Has(Path{"o"}, map[string]any{"deepNulls": nil}), triTrue},
		{"null not in array", Has(Path{"o"}, map[string]any{"list": nil}), triFalse},
		{"scalar in array value", Has(Path{"o"}, map[string]any{"nulls": 1}), triTrue},
		{"null vs object value", Has(Path{"o"}, map[string]any{"sub": nil}), triFalse},
		{"HasAny across kinds", HasAny(Path{"o"}, "zzz", map[string]any{"list": "x"}), triTrue},
		{"HasAll across kinds", HasAll(Path{"o"}, "sub", map[string]any{"list": "x"}), triTrue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := triMatch(t, tt.f, r); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchFunc(t *testing.T) {
	r := map[string]any{"n": 1.0}
	var got any
	f := Func(func(rec any) bool {
		got = rec
		m, ok := rec.(map[string]any)
		return ok && m["n"] == 1.0
	})
	if ok, err := Match(mapDB{}, f, r); err != nil || !ok {
		t.Fatalf("Match = %v, %v; want true, nil", ok, err)
	}
	if m, ok := got.(map[string]any); !ok || m["n"] != 1.0 {
		t.Errorf("predicate received %#v, want the record", got)
	}
	if v := triMatch(t, Func(func(any) bool { return false }), r); v != triFalse {
		t.Errorf("false predicate = %v, want false (never unknown)", v)
	}

	// A predicate in a decided AND or OR is not called.
	calls := 0
	counting := Func(func(any) bool { calls++; return true })
	for _, f := range []Filter{
		And(Eq(Path{"n"}, 2), counting),
		Or(Eq(Path{"n"}, 1), counting),
		Not(And(Eq(Path{"n"}, 2), Not(counting))),
	} {
		if _, err := Match(mapDB{}, f, r); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Errorf("predicate called %d times in decided junctions, want 0", calls)
	}
	// But it is called when the result is not yet decided.
	if ok, _ := Match(mapDB{}, And(Eq(Path{"n"}, 1), counting), r); !ok || calls != 1 {
		t.Errorf("And(T, Func) = %v with %d calls, want true with 1", ok, calls)
	}
}

func TestMatchFold(t *testing.T) {
	// fold agrees with strings.EqualFold on pairs that stress simple case
	// folding: the Kelvin sign, long s, final sigma, dotted I, and invalid
	// UTF-8.
	strs := []string{
		"", "k", "K", "K", "s", "S", "ſ", "σ", "Σ", "ς", "ß", "ss", "SS", "ẞ",
		"i", "I", "İ", "ı", "\xff", "\xfe", "�", "Straße", "STRASSE", "ǅ", "ǆ", "Ǆ",
	}
	for _, a := range strs {
		for _, b := range strs {
			if got, want := fold(a) == fold(b), strings.EqualFold(a, b); got != want {
				t.Errorf("fold(%q) == fold(%q) is %v; EqualFold is %v", a, b, got, want)
			}
		}
	}

	r := map[string]any{"s": "Mr. Kelvin of Σparta"}
	p := Path{"s"}
	tests := []struct {
		name string
		f    Filter
		want tri
	}{
		{"EqFold", EqFold(p, "mr. kelvin of σparta"), triTrue},
		{"LikeFold kelvin sign", LikeFold(p, "KELVIN"), triTrue},
		{"LikeFold final sigma", LikeFold(p, "ςpart"), triTrue},
		{"PrefixFold", PrefixFold(p, "MR. K"), triTrue},
		{"SuffixFold", SuffixFold(p, "SPARTA"), triFalse}, // Σ does not fold to S
		{"SuffixFold sigma", SuffixFold(p, "ΣPARTA"), triTrue},
		{"Like is case-sensitive", Like(p, "kelvin"), triFalse},
		{"LikeFold no full folding", LikeFold(Path{"g"}, "ss"), triFalse},
		{"LikeFold invalid UTF-8", LikeFold(Path{"bad"}, "\xff"), triTrue},
		{"Like invalid UTF-8", Like(Path{"bad"}, "\xff"), triFalse},
	}
	r["g"] = "Straße"
	r["bad"] = "a\xfeb"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := triMatch(t, tt.f, r); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchErrors(t *testing.T) {
	p := Path{"f"}
	ok := Eq(p, 1)                     // true on the record below
	bad := CompareNode{Op: 0, Path: p} // an unknown operator
	cyclic := make([]Filter, 1)
	cyclic[0] = AndNode{Filters: cyclic}
	deep := Filter(Eq(p, 1))
	for range maxDepth + 1 {
		deep = NotNode{Filter: deep}
	}
	buildErr := errors.New("build failed")
	tests := []struct {
		name  string
		f     Filter
		cause error // also wrapped, or nil
	}{
		{"nil filter", nil, ErrNilFilter},
		{"nil And child", AndNode{Filters: []Filter{ok, nil}}, ErrNilFilter},
		{"nil Or child", OrNode{Filters: []Filter{nil}}, ErrNilFilter},
		{"nil Not child", NotNode{}, ErrNilFilter},
		{"nil Fn", FuncNode{}, ErrNilFilter},
		{"ErrorNode", ErrorNode{Err: buildErr}, buildErr},
		{"constructor error", Eq(nil, 1), ErrEmptyPath},
		{"ErrorNode nil Err", ErrorNode{}, ErrInvalidFilter},
		{"pointer node", &AndNode{}, ErrInvalidFilter},
		{"typed nil pointer", (*CompareNode)(nil), ErrInvalidFilter},
		{"cyclic", AndNode{Filters: cyclic}, ErrInvalidFilter},
		{"too deep", deep, ErrInvalidFilter},
		{"unknown Op", bad, ErrInvalidFilter},
		{"Op out of range", CompareNode{Op: 999, Path: p}, ErrInvalidFilter},
		{"empty Path", CompareNode{Op: OpIsNull}, ErrEmptyPath},
		{"Eq int operand", CompareNode{Op: OpEq, Path: p, Operand: 1}, ErrInvalidOperand},
		{"Eq nil operand", CompareNode{Op: OpEq, Path: p}, ErrInvalidOperand},
		{"Eq NaN operand", CompareNode{Op: OpEq, Path: p, Operand: math.NaN()}, ErrInvalidOperand},
		{"Lt array operand", CompareNode{Op: OpLt, Path: p, Operand: []any{}}, ErrInvalidOperand},
		{"Contains object operand", CompareNode{Op: OpContains, Path: p, Operand: map[string]any{}}, ErrInvalidOperand},
		{"In int element", CompareNode{Op: OpIn, Path: p, Operands: []any{1.0, 2}}, ErrInvalidOperand},
		{"Like number operand", CompareNode{Op: OpLike, Path: p, Operand: 1.0}, ErrInvalidOperand},
		{"Regex string operand", CompareNode{Op: OpRegex, Path: p, Operand: "x"}, ErrInvalidOperand},
		{"Regex nil operand", CompareNode{Op: OpRegex, Path: p, Operand: (*regexp.Regexp)(nil)}, ErrInvalidOperand},
		{"Has array operand", CompareNode{Op: OpHas, Path: p, Operand: []any{}}, ErrInvalidOperand},
		{"Has nil operand", CompareNode{Op: OpHas, Path: p}, ErrInvalidOperand},
		{"Has typed map operand", CompareNode{Op: OpHas, Path: p, Operand: map[string]int{}}, ErrInvalidOperand},
		{"HasAny array element", CompareNode{Op: OpHasAny, Path: p, Operands: []any{"a", []any{}}}, ErrInvalidOperand},
		{"HasAll nil element", CompareNode{Op: OpHasAll, Path: p, Operands: []any{nil}}, ErrInvalidOperand},
		{"SameArray object operand", CompareNode{Op: OpSameArray, Path: p, Operand: map[string]any{}}, ErrInvalidOperand},
		{"SameArray typed slice", CompareNode{Op: OpSameArray, Path: p, Operand: []string{}}, ErrInvalidOperand},
		{"SameObject array operand", CompareNode{Op: OpSameObject, Path: p, Operand: []any{}}, ErrInvalidOperand},

		// Errors do not depend on short-circuiting.
		{"after decided And", And(Eq(p, 2), bad), ErrInvalidFilter},
		{"after decided Or", Or(ok, bad), ErrInvalidFilter},
		{"nested after decided Or", Or(ok, Not(And(ok, ErrorNode{Err: buildErr}))), buildErr},
		{"nil Fn after decided Or", Or(ok, FuncNode{}), ErrNilFilter},
		{"nil child after decided And", And(Eq(p, 2), OrNode{Filters: []Filter{nil}}), ErrNilFilter},
	}
	r := map[string]any{"f": 1.0}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Match(mapDB{}, tt.f, r)
			if got {
				t.Errorf("Match = true, want false")
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("err = %v, want ErrUnsupported", err)
			}
			if tt.cause != nil && !errors.Is(err, tt.cause) {
				t.Errorf("err = %v, want it to wrap %v", err, tt.cause)
			}
		})
	}

	if _, err := Match[map[string]any](nil, ok, r); err == nil {
		t.Error("Match with nil Querier: no error")
	}
}

// nestArrays returns a value nested depth arrays deep around leaf.
func nestArrays(depth int, leaf any) any {
	v := leaf
	for range depth {
		v = []any{v}
	}
	return v
}

func TestMatchValueLimits(t *testing.T) {
	// Cyclic values with two self-references, from Lookup or in a
	// hand-built operand, must stop quickly with an error rather than take
	// exponential time or report false.
	a := make([]any, 2)
	a[0], a[1] = a, a
	o := map[string]any{}
	o["a"], o["b"] = o, o
	deep := nestArrays(maxDepth+100, "x")
	deepObj := any("x")
	for range maxDepth + 100 {
		deepObj = map[string]any{"k": deepObj}
	}
	r := map[string]any{"a": a, "o": o, "deep": deep, "deepObj": deepObj, "ok": []any{1.0}}
	tests := []struct {
		name string
		f    Filter
	}{
		{"Has scalar in cyclic array", Has(Path{"a"}, "x")},
		{"Has object in cyclic array", Has(Path{"a"}, map[string]any{"k": 1})},
		{"HasAny in cyclic array", HasAny(Path{"a"}, "x", "y")},
		{"HasAll in cyclic array", HasAll(Path{"a"}, "x")},
		{"SameArray cyclic both", CompareNode{Op: OpSameArray, Path: Path{"a"}, Operand: a}},
		{"SameObject cyclic both", CompareNode{Op: OpSameObject, Path: Path{"o"}, Operand: o}},
		{"Has cyclic operand", CompareNode{Op: OpHas, Path: Path{"o"}, Operand: o}},
		{"SameArray deep equal value", CompareNode{Op: OpSameArray, Path: Path{"deep"}, Operand: nestArrays(maxDepth+100, "x")}},
		{"Not SameArray deep equal value", Not(CompareNode{Op: OpSameArray, Path: Path{"deep"}, Operand: nestArrays(maxDepth+100, "x")})},
		{"Has in deep arrays", Has(Path{"deep"}, "y")},
		{"Not Has in deep arrays", Not(Has(Path{"deep"}, "y"))},
		{"SameObject deep equal value", CompareNode{Op: OpSameObject, Path: Path{"deepObj"}, Operand: deepObj}},
		{"cyclic after undecided Or", Or(Eq(Path{"ok"}, 1), Has(Path{"a"}, "x"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Match(mapDB{}, tt.f, r)
			if got || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("Match = %v, %v; want false, ErrUnsupported", got, err)
			}
			if !strings.Contains(err.Error(), "nested more than") {
				t.Errorf("err = %v, want a depth error", err)
			}
		})
	}

	// Containment in an object descends only as deep as the operand, so a
	// cyclic object with a finite operand is fine.
	if got := triMatch(t, Has(Path{"o"}, map[string]any{"a": map[string]any{"b": "a"}}), r); got != triTrue {
		t.Errorf("Has in cyclic object = %v, want true", got)
	}
	if got := triMatch(t, Has(Path{"o"}, map[string]any{"a": map[string]any{"b": "x"}}), r); got != triFalse {
		t.Errorf("Has in cyclic object = %v, want false", got)
	}

	// Values nested exactly as deep as allowed are fine.
	ok := map[string]any{"v": nestArrays(maxDepth, "x")}
	if got, err := Match(mapDB{}, Has(Path{"v"}, "x"), ok); err != nil || !got {
		t.Errorf("Has at depth %d = %v, %v; want true, nil", maxDepth, got, err)
	}
	if got, err := Match(mapDB{}, SameArray(Path{"v"}, []any{nestArrays(maxDepth-1, "x")}), ok); err != nil || !got {
		t.Errorf("SameArray at depth %d = %v, %v; want true, nil", maxDepth, got, err)
	}
}

func TestMatchStepBudget(t *testing.T) {
	defer func(n int) { maxSteps = n }(maxSteps)
	maxSteps = 1000
	budgetErr := fmt.Sprintf("more than %d values", maxSteps)

	// An acyclic value whose arrays are shared has 2^30 paths but only 30
	// levels; the step budget stops it, in a field value or an operand.
	v := any("leaf")
	for range 30 {
		v = []any{v, v}
	}
	r := map[string]any{"v": v, "small": []any{1.0, 2.0, 3.0}}
	for _, f := range []Filter{
		Has(Path{"v"}, "x"),
		CompareNode{Op: OpSameArray, Path: Path{"small"}, Operand: v},
	} {
		got, err := Match(mapDB{}, f, r)
		if got || !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), budgetErr) {
			t.Errorf("Match = %v, %v; want false and a step budget error", got, err)
		}
	}

	// The budget is per walk, not per call to Match: many comparisons in
	// one filter, or many operands in one comparison, each get their own.
	many := make([]Filter, maxSteps)
	for i := range many {
		many[i] = Has(Path{"small"}, 3)
	}
	if got, err := Match(mapDB{}, And(many...), r); err != nil || !got {
		t.Errorf("And of %d Has = %v, %v; want true, nil", len(many), got, err)
	}
	xs := make([]any, maxSteps)
	for i := range xs {
		xs[i] = 4.0
	}
	if got, err := Match(mapDB{}, HasAny(Path{"small"}, xs...), r); err != nil || got {
		t.Errorf("HasAny of %d operands = %v, %v; want false, nil", len(xs), got, err)
	}
	// One walk of more than maxSteps values does reach it.
	big := make([]any, maxSteps+1)
	for i := range big {
		big[i] = 0.0
	}
	if _, err := Match(mapDB{}, Contains(Path{"big"}, 1), map[string]any{"big": big}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Contains on %d elements: err = %v, want a step budget error", len(big), err)
	}
}

func TestMatchLargeFlatArray(t *testing.T) {
	// Ordinary encoding/json data must not reach the default budget, even
	// when one filter visits more than maxSteps values in total.
	arr := make([]any, 200_000)
	for i := range arr {
		arr[i] = float64(i)
	}
	r := map[string]any{"a": arr}
	xs := make([]any, 100)
	for i := range xs {
		xs[i] = -float64(i + 1) // never found, so every element is visited
	}
	if got, err := Match(mapDB{}, HasAny(Path{"a"}, xs...), r); err != nil || got {
		t.Errorf("HasAny = %v, %v; want false, nil", got, err)
	}
	ors := make([]Filter, 100)
	for i := range ors {
		ors[i] = Contains(Path{"a"}, -float64(i+1))
	}
	if got, err := Match(mapDB{}, Or(ors...), r); err != nil || got {
		t.Errorf("Or of Contains = %v, %v; want false, nil", got, err)
	}
}

func TestMatchNestedOperandErrors(t *testing.T) {
	// A malformed operand nested in a hand-built CompareNode is an error on
	// every record, whether or not evaluation reaches it.
	p := Path{"v"}
	w := Path{"w"}
	cyclic := map[string]any{}
	cyclic["a"], cyclic["b"] = cyclic, cyclic
	tests := []struct {
		name string
		f    Filter
	}{
		{"SameArray int element", CompareNode{Op: OpSameArray, Path: p, Operand: []any{1}}},
		{"SameArray nested NaN", CompareNode{Op: OpSameArray, Path: p, Operand: []any{[]any{math.NaN()}}}},
		{"SameObject typed slice value", CompareNode{Op: OpSameObject, Path: p, Operand: map[string]any{"k": []string{}}}},
		{"SameObject cyclic", CompareNode{Op: OpSameObject, Path: p, Operand: cyclic}},
		{"Has nested array", CompareNode{Op: OpHas, Path: p, Operand: map[string]any{"k": []any{1.0}}}},
		{"Has nested int", CompareNode{Op: OpHas, Path: p, Operand: map[string]any{"k": map[string]any{"j": 1}}}},
		{"Has cyclic", CompareNode{Op: OpHas, Path: p, Operand: cyclic}},
		{"HasAny nested array", CompareNode{Op: OpHasAny, Path: p, Operands: []any{"a", map[string]any{"k": []any{}}}}},
		{"HasAll nested int", CompareNode{Op: OpHasAll, Path: p, Operands: []any{map[string]any{"k": 1}}}},
	}
	recs := []map[string]any{
		{"w": 1.0, "v": []any{1.0}},
		{"w": 2.0, "v": map[string]any{"k": 1.0}},
		{"w": 1.0},
		{},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, f := range []Filter{
				tt.f,
				Or(Eq(w, 1), tt.f),  // decided true on records with w = 1
				And(Eq(w, 2), tt.f), // decided false on records without w = 2
			} {
				for i, r := range recs {
					got, err := Match(mapDB{}, f, r)
					if got || !errors.Is(err, ErrUnsupported) || !errors.Is(err, ErrInvalidOperand) {
						t.Errorf("record %d: Match = %v, %v; want false, ErrInvalidOperand", i, got, err)
					}
				}
			}
		})
	}
}

func TestMatchMixedKindOperands(t *testing.T) {
	// Hand-built In and HasAny may mix operand kinds. Each is the Kleene OR
	// over its operands, so a kind mismatch with no match is unknown, and
	// Not does not match.
	r := map[string]any{"n": 1.0, "o": map[string]any{"k": "v"}}
	tests := []struct {
		name string
		f    Filter
		want tri
	}{
		{"In no match", CompareNode{Op: OpIn, Path: Path{"n"}, Operands: []any{"a", 2.0}}, triUnknown},
		{"In match", CompareNode{Op: OpIn, Path: Path{"n"}, Operands: []any{"a", 1.0}}, triTrue},
		{"In all same kind", CompareNode{Op: OpIn, Path: Path{"n"}, Operands: []any{3.0, 2.0}}, triFalse},
		{"HasAny no match", HasAny(Path{"o"}, 1, "z"), triUnknown},
		{"HasAny match", HasAny(Path{"o"}, 1, "k"), triTrue},
		{"HasAll mismatch and miss", HasAll(Path{"o"}, 1, "z"), triFalse},
		{"HasAll mismatch and hit", HasAll(Path{"o"}, 1, "k"), triUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := triMatch(t, tt.f, r); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchQueryFilter(t *testing.T) {
	r := map[string]any{"state": "NY", "age": nil}
	q := Where(
		Eq(Path{"state"}, "NY"),
		Or(Gt(Path{"age"}, 40), IsNull(Path{"age"})),
	)
	if ok, err := Match(mapDB{}, q.Filter(), r); err != nil || !ok {
		t.Errorf("Match = %v, %v; want true, nil", ok, err)
	}
	var nilQ *Query
	if ok, err := Match(mapDB{}, nilQ.Filter(), r); err != nil || !ok {
		t.Errorf("nil query: Match = %v, %v; want true, nil", ok, err)
	}
}
