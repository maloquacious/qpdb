// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"errors"
	"fmt"
	"slices"
)

// Build errors recorded by [Query.Offset] and [Query.Limit]. A recorded
// error wraps one of these, so callers can test for them with [errors.Is].
var (
	// ErrNegativeOffset reports a negative argument to [Query.Offset].
	ErrNegativeOffset = errors.New("qpdb: negative offset")
	// ErrNegativeLimit reports a negative argument to [Query.Limit].
	ErrNegativeLimit = errors.New("qpdb: negative limit")
)

// Query is an immutable description of a read: a filter, an order, and an
// offset and limit. It has no connection to a collection; pass it to the
// methods of a Querier implementation.
//
// Every builder method ([Query.Where], [Query.OrderBy], [Query.Offset],
// [Query.Limit]) returns a new *Query and leaves the receiver unchanged, so
// one query can safely be the parent of several others. The builder methods
// accept a nil receiver: a nil *Query is a valid query that matches every
// record, as is a query with no filters.
//
// Filtering runs first, then ordering, then offset and limit, regardless of
// the order in which the methods were called.
//
// A query records the first build error made while building it (see
// [Query.Err]). Once it holds an error, every later builder method returns a
// new query carrying the same error, and the same filters, order, offset,
// and limit, and does nothing else.
//
// Implementations read a query through [Query.Err], [Query.Filter],
// [Query.Order], and [Query.Page].
type Query struct {
	// filters are AND-ed. The backing array is never modified after the
	// query is built, so queries may share it.
	filters []Filter
	// order is the sort keys. As with filters, it is never modified.
	order    []OrderKey
	offset   int
	limit    int
	hasLimit bool
	err      error
}

// Where starts a query whose filters are AND-ed. Where() matches every
// record. It is equivalent to (*Query)(nil).Where(filters...).
func Where(filters ...Filter) *Query {
	var q *Query
	return q.Where(filters...)
}

// Where returns a query that ANDs filters onto q's filters.
//
// The first build error among filters, as reported by [FilterErr], becomes
// the query's error, and none of filters is added. A nil filter is an error
// wrapping [ErrNilFilter].
func (q *Query) Where(filters ...Filter) *Query {
	c := q.clone()
	if c.err != nil {
		return c
	}
	for i, f := range filters {
		if f == nil {
			c.err = fmt.Errorf("%w: Where: filter %d", ErrNilFilter, i)
			return c
		}
		if err := FilterErr(f); err != nil {
			c.err = err
			return c
		}
	}
	// Concat always allocates, so queries derived from the same parent
	// never share a backing array they could both append to.
	c.filters = slices.Concat(c.filters, filters)
	return c
}

// OrderBy returns a query sorted by keys, applied left to right as
// tie-breakers. It replaces any previous order; OrderBy() with no keys
// removes the order, so results come back in the default order (design §7).
//
// A key with an empty or nil Path is a build error wrapping [ErrEmptyPath],
// and the order is left unchanged.
func (q *Query) OrderBy(keys ...OrderKey) *Query {
	c := q.clone()
	if c.err != nil {
		return c
	}
	var order []OrderKey
	if len(keys) > 0 {
		order = make([]OrderKey, len(keys))
	}
	for i, k := range keys {
		if len(k.Path) == 0 {
			c.err = fmt.Errorf("%w: OrderBy: key %d (%s)", ErrEmptyPath, i, k)
			return c
		}
		order[i] = k.clone()
	}
	c.order = order
	return c
}

// Offset returns a query that skips the first n results. It replaces any
// previous offset. A negative n is a build error wrapping
// [ErrNegativeOffset].
func (q *Query) Offset(n int) *Query {
	c := q.clone()
	if c.err != nil {
		return c
	}
	if n < 0 {
		c.err = fmt.Errorf("%w: Offset(%d)", ErrNegativeOffset, n)
		return c
	}
	c.offset = n
	return c
}

// Limit returns a query that keeps at most n results. It replaces any
// previous limit. Limit(0) returns no results; a query without Limit has no
// limit. A negative n is a build error wrapping [ErrNegativeLimit].
func (q *Query) Limit(n int) *Query {
	c := q.clone()
	if c.err != nil {
		return c
	}
	if n < 0 {
		c.err = fmt.Errorf("%w: Limit(%d)", ErrNegativeLimit, n)
		return c
	}
	c.limit, c.hasLimit = n, true
	return c
}

// Err returns the first build error recorded while building q, or nil. A nil
// q has no error.
func (q *Query) Err() error {
	if q == nil {
		return nil
	}
	return q.err
}

// Filter returns q's filters as a single filter: [And]() if q is nil or has
// no filters, the filter itself if q has exactly one, and an [AndNode] of
// them otherwise. It never returns nil.
//
// The returned tree shares its nodes with q, and [Query.Where] stores the
// filters it is given without copying them. Do not modify a filter after
// passing it to Where, or any tree returned by Filter; doing so changes the
// query.
func (q *Query) Filter() Filter {
	if q == nil {
		return And()
	}
	if len(q.filters) == 1 {
		return q.filters[0]
	}
	return And(q.filters...)
}

// Order returns a copy of q's sort keys, or nil if q is nil or has no order.
func (q *Query) Order() []OrderKey {
	if q == nil || len(q.order) == 0 {
		return nil
	}
	order := make([]OrderKey, len(q.order))
	for i, k := range q.order {
		order[i] = k.clone()
	}
	return order
}

// Page returns q's offset and limit. limited reports whether q has a limit;
// when it is false, limit is 0 and there is no limit. Without [Query.Offset]
// the offset is 0. A nil q has offset 0 and no limit.
func (q *Query) Page() (offset, limit int, limited bool) {
	if q == nil {
		return 0, 0, false
	}
	return q.offset, q.limit, q.hasLimit
}

// clone returns a shallow copy of q, or a new empty query if q is nil. The
// copy shares q's filter and order slices, which are never modified in
// place.
func (q *Query) clone() *Query {
	if q == nil {
		return &Query{}
	}
	c := *q
	return &c
}
