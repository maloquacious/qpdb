// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"fmt"
	"slices"
)

// Path names a field by segments.
//
//	qpdb.Path{"name"}         // top-level field "name"
//	qpdb.Path{"addr", "city"} // field "city" inside the object in field "addr"
//
// Segments are never split, so Path{"a.b"} names a field literally called
// "a.b". Each segment after the first descends into an object. An empty
// segment is allowed and names the key "", which JSON permits.
//
// An empty or nil Path is a build error, reported by the filter constructed
// with it (see [FilterErr]).
//
// Filter constructors copy the Path, so changing a Path after building a
// filter does not change the filter.
type Path []string

// idSegment is the single segment of [IDPath].
//
// It is not valid UTF-8. JSON text is UTF-8, and encoding/json replaces
// invalid bytes with U+FFFD when decoding, so no key decoded from JSON can
// equal it. That keeps IDPath distinct from every field path, including a
// field literally named "___id".
const idSegment = "\xff___id"

// IDPath is the reserved path for a record's ID.
//
// Each implementation's Lookup resolves IDPath to that record's ID as a
// string; use [Path.IsID] to recognize it. IDPath is distinct from every path
// that names a JSON field, including Path{"___id"}.
//
// IDPath is meaningful only within a process. Its segment is not valid
// UTF-8, so it does not survive a JSON round trip (encoding/json replaces the
// invalid byte with U+FFFD, and the result is no longer an ID path), and it
// prints as an escaped byte string rather than anything readable.
//
// IDPath is a variable only because Go has no constant slices. Do not modify
// it; [Path.IsID] compares segment values, so copies of IDPath, such as the
// one a filter constructor stores, remain ID paths.
var IDPath = Path{idSegment}

// IsID reports whether p is [IDPath] (or a copy of it).
func (p Path) IsID() bool {
	return len(p) == 1 && p[0] == idSegment
}

// checkPath validates p for the constructor named name and returns a copy.
func checkPath(name string, p Path) (Path, error) {
	if len(p) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrEmptyPath, name)
	}
	return slices.Clone(p), nil
}
