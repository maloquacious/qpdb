// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"fmt"
	"slices"
)

// OrderKey is one sort key of a [Query]. Build keys with [Asc], [Desc],
// [Natural], and [NaturalDesc], and pass them to [Query.OrderBy].
//
// An OrderKey is plain data: implementations and the sort helpers read its
// fields. An empty or nil Path is a build error, recorded by
// [Query.OrderBy] rather than by the constructor.
type OrderKey struct {
	// Path names the field to sort by.
	Path Path
	// Desc reverses the order, so missing and null sort last.
	Desc bool
	// Natural selects natural order (design §7.2) instead of value order
	// (design §7.1).
	Natural bool
}

// Asc returns a key that sorts by the value at p in ascending value order.
func Asc(p Path) OrderKey { return OrderKey{Path: slices.Clone(p)} }

// Desc returns a key that sorts by the value at p in descending value order.
func Desc(p Path) OrderKey { return OrderKey{Path: slices.Clone(p), Desc: true} }

// Natural returns a key that sorts by the value at p in ascending natural
// order.
func Natural(p Path) OrderKey { return OrderKey{Path: slices.Clone(p), Natural: true} }

// NaturalDesc returns a key that sorts by the value at p in descending
// natural order.
func NaturalDesc(p Path) OrderKey {
	return OrderKey{Path: slices.Clone(p), Desc: true, Natural: true}
}

// String returns k as the constructor call that builds it, such as
// `NaturalDesc(["addr" "city"])`.
func (k OrderKey) String() string {
	name := "Asc"
	switch {
	case k.Natural && k.Desc:
		name = "NaturalDesc"
	case k.Natural:
		name = "Natural"
	case k.Desc:
		name = "Desc"
	}
	return fmt.Sprintf("%s(%q)", name, []string(k.Path))
}

// clone returns a copy of k that shares no memory with it.
func (k OrderKey) clone() OrderKey {
	k.Path = slices.Clone(k.Path)
	return k
}
