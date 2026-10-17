// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"context"
	"errors"
)

// ErrUnsupported reports a filter node that a [Querier] implementation, or
// [Match], cannot evaluate, or a value that Match cannot evaluate, such as a
// field value from [Querier.Lookup] that is not a JSON kind. Errors for
// such nodes and values wrap it, so callers can test for it with
// [errors.Is].
var ErrUnsupported = errors.New("qpdb: unsupported")

// Querier is the read interface a client implements over its own collection
// of records of type R. The rest of the read API is built on it.
//
// A query is evaluated in a fixed order, regardless of the order in which
// its builder methods were called: filtering first (see [Query.Filter]),
// then ordering (see [Query.Order]), then offset and limit (see
// [Query.Page]). A nil *Query matches every record.
//
// Implementations must:
//   - return copies: every record Get returns is the caller's own, and
//     changing it never changes the collection. For record types that hold
//     maps or slices a shallow copy is not enough; the implementation must
//     deep copy.
//   - honor context cancellation, returning an error that wraps ctx.Err()
//     once ctx is done.
//   - return q.Err() before doing any work when it is not nil.
//   - return an error wrapping [ErrUnsupported] for any filter node they
//     cannot evaluate.
//
// In-memory implementations evaluate filters with [Match] rather than
// writing their own evaluator.
//
// Implementations are not required to be safe for concurrent use until
// v1.0.0.
type Querier[R any] interface {
	// Get returns copies of the records matching q, in q's order, after
	// q's offset and limit.
	Get(ctx context.Context, q *Query) ([]R, error)

	// Count returns the number of records Get would return for q, after
	// Offset and Limit: when both succeed, Count(ctx, q) equals
	// len(Get(ctx, q)). Unlike SQL's COUNT(*), a limit caps the count.
	//
	// For the total behind a paged view, count the query without Offset and
	// Limit. A Query cannot remove its limit, so build the unpaged query
	// first and derive the paged one from it:
	//
	//	all := qpdb.Where(f)
	//	total, err := db.Count(ctx, all)
	//	page, err := db.Get(ctx, all.Offset(20).Limit(10))
	Count(ctx context.Context, q *Query) (int, error)

	// Lookup returns the value at path p in r; ok is false if the field is
	// missing. A field that is present and null is (nil, true).
	//
	// The value is one of the kinds produced by encoding/json: nil, bool,
	// float64, string, []any, or map[string]any, with nested values of
	// those kinds too. [Match] cannot evaluate any other Go type, such as
	// an int, a []string, or a typed nil pointer, and returns an error
	// wrapping [ErrUnsupported] when it reaches one.
	//
	// Lookup resolves [IDPath] to r's ID as a string. Each segment of any
	// other path after the first descends into an object; if a segment
	// before the last is missing or not an object, the field is missing.
	//
	// Lookup must not modify r. The value it returns may share memory with
	// r, so callers must not modify it.
	Lookup(r R, p Path) (v any, ok bool)
}
