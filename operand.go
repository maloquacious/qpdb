// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Value constrains scalar operands, so non-JSON operands do not compile.
//
// Numeric operands are stored in the filter tree as float64. Two consequences
// are documented rather than handled:
//   - a float32 operand does not equal the same decimal decoded from JSON:
//     float32(0.1) widens to 0.10000000149011612, not 0.1;
//   - integers with magnitude above 2^53 lose precision when converted.
//
// These are build errors: NaN and ±Inf, which are not JSON numbers, and a
// [json.Number], which would otherwise be stored as a string and never equal
// a number.
//
// Null is never a Value operand; use [IsNull].
type Value interface {
	~bool | ~string |
		~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

var jsonNumberType = reflect.TypeFor[json.Number]()

// value normalizes a Value operand to bool, string, or float64.
// The error describes why v is not a valid operand.
func value[V Value](v V) (any, error) {
	switch x := any(v).(type) {
	case bool, string:
		return x, nil
	case int:
		return float64(x), nil
	}
	// float64, named types, and the remaining numeric types.
	return scalar(reflect.ValueOf(v))
}

// scalar converts a bool, string, or numeric reflect.Value. Any other kind
// is an error.
func scalar(rv reflect.Value) (any, error) {
	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool(), nil
	case reflect.String:
		if rv.Type() == jsonNumberType {
			return nil, fmt.Errorf("json.Number %q is not allowed; convert it to float64", rv.String())
		}
		return rv.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint()), nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("%v is not a JSON number", f)
		}
		return f, nil
	}
	return nil, fmt.Errorf("%s is not a JSON kind", rv.Type())
}

// maxDepth bounds the nesting of a JSON-kind operand and of a filter tree.
// It stops a cyclic map, slice, or tree from recursing forever.
const maxDepth = 1000

// maxLocation is the number of location segments an operandError shows;
// deeper locations show only their deepest segments.
const maxLocation = 10

// operandError reports a problem at a location inside an operand. The
// location is built only on the error path, as the error unwinds.
type operandError struct {
	// rev holds location segments such as `[1]` and `["k"]`, deepest
	// first; it is empty for the operand itself.
	rev []string
	msg string
}

// Error returns, for example, `operand[1]["k"]: msg`. A location deeper
// than maxLocation segments is shown as `operand…` and its deepest segments.
func (e *operandError) Error() string {
	var b strings.Builder
	b.WriteString("operand")
	segs := e.rev
	if len(segs) > maxLocation {
		segs = segs[:maxLocation]
		b.WriteString("…")
	}
	for _, seg := range slices.Backward(segs) {
		b.WriteString(seg)
	}
	b.WriteString(": ")
	b.WriteString(e.msg)
	return b.String()
}

// toJSON deep-copies x into the kinds produced by encoding/json: nil, bool,
// float64, string, []any, and map[string]any.
//
// It accepts any Go value whose kind maps onto a JSON kind: bool, string,
// and numeric kinds (including named types), slices and arrays (including
// typed ones such as []string), maps with string-kind keys, and interfaces
// holding any of these. A nil slice becomes an empty array and a nil map an
// empty object. Conversion is by kind, so a []byte or [json.RawMessage]
// becomes an array of numbers, not a string or decoded JSON.
//
// These are errors: structs, pointers, channels, functions, complex numbers,
// maps with non-string keys, NaN and ±Inf, and [json.Number]. If arrays is
// false, a slice or array anywhere in x is also an error.
//
// The result shares no memory with x.
func toJSON(x any, arrays bool) (any, error) {
	if x == nil {
		return nil, nil
	}
	v, err := convert(reflect.ValueOf(x), arrays, 0)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// convert implements toJSON.
func convert(rv reflect.Value, arrays bool, depth int) (any, *operandError) {
	if depth > maxDepth {
		return nil, &operandError{msg: fmt.Sprintf("nested more than %d deep", maxDepth)}
	}
	switch rv.Kind() {
	case reflect.Interface:
		if rv.IsNil() {
			return nil, nil
		}
		return convert(rv.Elem(), arrays, depth)
	case reflect.Slice, reflect.Array:
		if !arrays {
			return nil, &operandError{msg: "array not allowed inside a Has operand"}
		}
		out := make([]any, rv.Len())
		for i := range rv.Len() {
			v, err := convert(rv.Index(i), arrays, depth+1)
			if err != nil {
				err.rev = append(err.rev, "["+strconv.Itoa(i)+"]")
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return nil, &operandError{msg: rv.Type().String() + " does not have string keys"}
		}
		// Visit keys in sorted order so the first error reported is
		// deterministic.
		keys := rv.MapKeys()
		slices.SortFunc(keys, func(a, b reflect.Value) int { return cmp.Compare(a.String(), b.String()) })
		out := make(map[string]any, len(keys))
		for _, key := range keys {
			k := key.String()
			v, err := convert(rv.MapIndex(key), arrays, depth+1)
			if err != nil {
				err.rev = append(err.rev, "["+strconv.Quote(k)+"]")
				return nil, err
			}
			out[k] = v
		}
		return out, nil
	}
	v, err := scalar(rv)
	if err != nil {
		return nil, &operandError{msg: err.Error()}
	}
	return v, nil
}
