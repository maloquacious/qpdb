// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

type (
	myBool   bool
	myString string
	myInt    int
	myUint8  uint8
	myFloat  float32
)

func TestValueNormalization(t *testing.T) {
	tests := []struct {
		name string
		f    Filter
		want any
	}{
		{"bool", Eq(Path{"x"}, true), true},
		{"string", Eq(Path{"x"}, "s"), "s"},
		{"int", Eq(Path{"x"}, 3), 3.0},
		{"int8", Eq(Path{"x"}, int8(-8)), -8.0},
		{"int16", Eq(Path{"x"}, int16(16)), 16.0},
		{"int32", Eq(Path{"x"}, int32(32)), 32.0},
		{"int64", Eq(Path{"x"}, int64(64)), 64.0},
		{"uint", Eq(Path{"x"}, uint(1)), 1.0},
		{"uint8", Eq(Path{"x"}, uint8(8)), 8.0},
		{"uint16", Eq(Path{"x"}, uint16(16)), 16.0},
		{"uint32", Eq(Path{"x"}, uint32(32)), 32.0},
		{"uint64", Eq(Path{"x"}, uint64(64)), 64.0},
		{"float32", Eq(Path{"x"}, float32(0.5)), 0.5},
		{"float64", Eq(Path{"x"}, 0.1), 0.1},
		{"named bool", Eq(Path{"x"}, myBool(true)), true},
		{"named string", Eq(Path{"x"}, myString("s")), "s"},
		{"named int", Eq(Path{"x"}, myInt(7)), 7.0},
		{"named uint8", Eq(Path{"x"}, myUint8(9)), 9.0},
		{"named float32", Eq(Path{"x"}, myFloat(2.5)), 2.5},
		{"Contains", Contains(Path{"x"}, myInt(1)), 1.0},
		{"Ne", Ne(Path{"x"}, 1), 1.0},
		{"Lt", Lt(Path{"x"}, 1), 1.0},
		{"Lte", Lte(Path{"x"}, 1), 1.0},
		{"Gt", Gt(Path{"x"}, 1), 1.0},
		{"Gte", Gte(Path{"x"}, "a"), "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, ok := tt.f.(CompareNode)
			if !ok {
				t.Fatalf("got %T, want CompareNode", tt.f)
			}
			if !reflect.DeepEqual(n.Operand, tt.want) {
				t.Errorf("Operand = %#v (%T), want %#v (%T)", n.Operand, n.Operand, tt.want, tt.want)
			}
		})
	}
}

// TestValuePrecisionLimits pins the documented float32 and 2^53 limits.
func TestValuePrecisionLimits(t *testing.T) {
	if got := Eq(Path{"x"}, float32(0.1)).(CompareNode).Operand; got == 0.1 {
		t.Errorf("float32(0.1) normalized to exactly 0.1; doc comment is wrong")
	}
	big := int64(1)<<53 + 1
	if got := Eq(Path{"x"}, big).(CompareNode).Operand.(float64); got != math.Exp2(53) {
		t.Errorf("2^53+1 normalized to %v; doc comment is wrong", got)
	}
}

func TestInOperands(t *testing.T) {
	tests := []struct {
		name string
		f    Filter
		want []any
	}{
		{"none", In[string](Path{"x"}), []any{}},
		{"strings", In(Path{"x"}, "a", "b"), []any{"a", "b"}},
		{"ints", In(Path{"x"}, 1, 2), []any{1.0, 2.0}},
		{"named", In(Path{"x"}, myInt(1)), []any{1.0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := tt.f.(CompareNode)
			if n.Op != OpIn || !reflect.DeepEqual(n.Operands, tt.want) {
				t.Errorf("got %v %#v, want In %#v", n.Op, n.Operands, tt.want)
			}
		})
	}
}

func TestToJSON(t *testing.T) {
	ch := make(chan int)
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	tests := []struct {
		name    string
		in      any
		want    any
		wantErr string // substring; "" means no error
	}{
		{"nil", nil, nil, ""},
		{"bool", true, true, ""},
		{"string", "s", "s", ""},
		{"int", 1, 1.0, ""},
		{"named int", myInt(2), 2.0, ""},
		{"float32", float32(0.5), 0.5, ""},
		{"[]any", []any{1, "a", nil, true}, []any{1.0, "a", nil, true}, ""},
		{"[]string", []string{"a", "b"}, []any{"a", "b"}, ""},
		{"[]int", []int{1, 2}, []any{1.0, 2.0}, ""},
		{"[2]int array", [2]int{1, 2}, []any{1.0, 2.0}, ""},
		{"nil []string", []string(nil), []any{}, ""},
		{"map[string]int", map[string]int{"a": 1}, map[string]any{"a": 1.0}, ""},
		{"named key map", map[myString]bool{"k": true}, map[string]any{"k": true}, ""},
		{"nil map", map[string]int(nil), map[string]any{}, ""},
		{"nested", map[string]any{"a": []map[string]uint8{{"b": 3}}, "n": nil},
			map[string]any{"a": []any{map[string]any{"b": 3.0}}, "n": nil}, ""},
		{"struct", struct{}{}, nil, "struct {} is not a JSON kind"},
		{"pointer", new(int), nil, "*int is not a JSON kind"},
		{"chan", ch, nil, "chan int is not a JSON kind"},
		{"func", func() {}, nil, "func() is not a JSON kind"},
		{"complex", complex(1, 2), nil, "complex128 is not a JSON kind"},
		{"uintptr", uintptr(1), nil, "uintptr is not a JSON kind"},
		{"int keys", map[int]string{1: "a"}, nil, "does not have string keys"},
		{"nested bad", []any{1, map[string]any{"k": ch}}, nil, `operand[1]["k"]: chan int`},
		{"cycle", cyclic, nil, "nested more than"},
		{"[]byte", []byte("hi"), []any{104.0, 105.0}, ""},
		{"json.RawMessage", json.RawMessage(`1`), []any{49.0}, ""},
		{"NaN", math.NaN(), nil, "NaN is not a JSON number"},
		{"+Inf", math.Inf(1), nil, "+Inf is not a JSON number"},
		{"-Inf float32", float32(math.Inf(-1)), nil, "-Inf is not a JSON number"},
		{"nested NaN", map[string]any{"k": []float64{1, math.NaN()}}, nil, `operand["k"][1]: NaN`},
		{"json.Number", json.Number("1"), nil, "json.Number"},
		{"nested json.Number", []any{json.Number("1")}, nil, "operand[0]: json.Number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toJSON(tt.in, true)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestToJSONDeepCopies(t *testing.T) {
	inner := []any{"x"}
	in := map[string]any{"a": inner}
	f := SameObject(Path{"o"}, in).(CompareNode)
	inner[0] = "changed"
	in["b"] = 1
	want := map[string]any{"a": []any{"x"}}
	if !reflect.DeepEqual(f.Operand, want) {
		t.Errorf("operand changed with caller's value: %#v", f.Operand)
	}
}

func TestToJSONNoArrays(t *testing.T) {
	tests := []struct {
		name    string
		in      any
		want    any
		wantErr string // substring; "" means no error
	}{
		{"scalar", 1, 1.0, ""},
		{"object", map[string]any{"a": map[string]int{"b": 1}, "n": nil},
			map[string]any{"a": map[string]any{"b": 1.0}, "n": nil}, ""},
		{"top-level slice", []int{1}, nil, "operand: array not allowed"},
		{"nested slice", map[string]any{"a": []int{1}}, nil, `operand["a"]: array not allowed`},
		{"deeply nested array", map[string]any{"a": map[string]any{"b": [1]string{"x"}}}, nil,
			`operand["a"]["b"]: array not allowed`},
		{"nested nil slice", map[string][]int{"a": nil}, nil, `operand["a"]: array not allowed`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toJSON(tt.in, false)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestValueRejected(t *testing.T) {
	p := Path{"x"}
	nan := math.NaN()
	tests := []struct {
		name    string
		f       Filter
		wantMsg string
	}{
		{"Eq NaN", Eq(p, nan), "Eq([\"x\"]): NaN is not a JSON number"},
		{"Ne +Inf", Ne(p, math.Inf(1)), "+Inf"},
		{"Lt -Inf", Lt(p, math.Inf(-1)), "-Inf"},
		{"Lte float32 NaN", Lte(p, float32(nan)), "NaN"},
		{"Gt named float NaN", Gt(p, myFloat(nan)), "NaN"},
		{"Gte NaN", Gte(p, nan), "NaN"},
		{"Contains NaN", Contains(p, nan), "Contains"},
		{"In NaN element", In(p, 1, nan), "In([\"x\"]): element 1: NaN"},
		{"NotIn NaN element", NotIn(p, nan), "element 0: NaN"},
		{"Eq json.Number", Eq(p, json.Number("1")), "json.Number"},
		{"In json.Number", In(p, json.Number("1")), "json.Number"},
		{"Has NaN", Has(p, nan), "NaN"},
		{"Has nested Inf", Has(p, map[string]float64{"a": math.Inf(1)}), `operand["a"]: +Inf`},
		{"HasAny NaN element", HasAny(p, 1, nan), "element 1"},
		{"SameArray NaN", SameArray(p, []float64{nan}), "operand[0]: NaN"},
		{"SameObject Inf", SameObject(p, map[string]any{"a": math.Inf(1)}), `operand["a"]: +Inf`},
		{"Has json.Number", Has(p, json.Number("1")), "json.Number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FilterErr(tt.f)
			if !errors.Is(err, ErrInvalidOperand) {
				t.Fatalf("FilterErr = %v, want ErrInvalidOperand", err)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err, tt.wantMsg)
			}
		})
	}
}

// TestToJSONErrorDeterministic checks that the first error reported is the
// one under the smallest key, whatever the map's iteration order.
func TestToJSONErrorDeterministic(t *testing.T) {
	ch := make(chan int)
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"several bad keys", map[string]any{"z": ch, "m": func() {}, "b": struct{}{}, "a": 1},
			`qpdb: invalid operand: SameObject(["f"]): operand["b"]: struct {} is not a JSON kind`},
		{"nested bad keys", map[string]any{"y": map[string]any{"q": ch, "p": math.NaN()}, "x": []any{1, complex(1, 1)}},
			`qpdb: invalid operand: SameObject(["f"]): operand["x"][1]: complex128 is not a JSON kind`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for range 50 {
				err := FilterErr(SameObject(Path{"f"}, tt.in))
				if err == nil || err.Error() != tt.want {
					t.Fatalf("err = %v, want %s", err, tt.want)
				}
			}
		})
	}
}

func TestOperandErrorLocationTruncated(t *testing.T) {
	deep := func(n int) any {
		var v any = math.NaN()
		for range n {
			v = []any{v}
		}
		return v
	}
	cyclic := map[string]any{}
	cyclic["k"] = cyclic
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"at limit", deep(maxLocation), "operand" + strings.Repeat("[0]", maxLocation) + ": NaN is not a JSON number"},
		{"over limit", deep(maxLocation + 1), "operand…" + strings.Repeat("[0]", maxLocation) + ": NaN is not a JSON number"},
		{"cycle", cyclic, "operand…" + strings.Repeat(`["k"]`, maxLocation) + ": nested more than 1000 deep"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := toJSON(tt.in, true)
			if err == nil || err.Error() != tt.want {
				t.Errorf("err = %v, want %s", err, tt.want)
			}
		})
	}
}
