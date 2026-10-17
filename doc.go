// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package qpdb applies a TaffyDB-style query surface to a client's own
// in-memory collection of records.
//
// Filters are an exported tree of data types (see [Filter]) built with
// constructors such as [Eq], [And], and [Has]. Fields are named by a [Path];
// [IDPath] names a record's ID. A constructor given bad input never panics;
// it returns a node that records the error, which [FilterErr] reports.
//
// Clients implement [Querier] over their own collections. In-memory
// implementations evaluate filters with [Match], which follows SQL's
// three-valued logic: a comparison on a missing or null field, or a field of
// the wrong kind, is unknown and never matches, even under [Not]. The type
// tests, such as [IsNull] and [IsString], are the exception: they are true
// or false, and false on a missing field.
//
// The package has no parser, wire format, or transport. The specification is
// docs/design.md in the source repository.
//
// Implementations of the package's interfaces are not required to be safe
// for concurrent use until v1.0.0.
package qpdb
