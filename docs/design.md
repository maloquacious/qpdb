# qpdb design

This document is the specification for qpdb.
It records the decisions made while reviewing §14 of the TaffyDB reference ([`taffydb/api.md`](taffydb/api.md)).
Where this document and the TaffyDB reference disagree, this document wins.
The TaffyDB reference remains an accurate description of what `taffy.js` does and is background only.

## 1. Scope

qpdb is a Go package that applies a TaffyDB-style query surface to a collection of records.
It is not a TaffyDB clone and does not aim for compatibility with `taffy.js` or its clients.

qpdb assumes the whole collection is in memory.
It is meant for small collections, such as the state of a turn-based strategy game, not for large data sets.
Helpers therefore work on materialized results, and nothing is designed for pushing work down to a database engine.

The package provides:

- a query model: an immutable `Query` built from an exported filter tree;
- a generic `Querier[R]` interface that clients implement against their own collections;
- helper functions that build the rest of the read API on top of `Querier[R]`;
- an in-memory evaluator, so implementations do not write their own filter logic;
- an internal reference implementation over a fixture dataset, used by the tests and the examples (§9).

The package does not provide:

- a wire format, HTTP layer, or any other transport;
- a parser for a filter language, order specs, or any other query text;
- loading or saving files (clients load their collection and pass it in);
- mutation (`Insert`, `Update`, `Remove`, `Merge`, `Sort`) or events before v1.0.0 (§10).

Clients that want a JSON filter language build it on top of the query builder.

## 2. Records and values

### 2.1 Records

`R` is the client's record type.
qpdb never inspects an `R` directly; it reads fields through `Querier[R].Lookup` (§5).

Every record a `Querier[R]` returns must be the caller's own copy.
Changing it must never change the collection.
For record types that hold maps or slices, a shallow copy does not satisfy this; the implementation must deep copy.
The type system cannot enforce this, so it is part of the interface contract, and the tests check it against the reference implementation (§9).

### 2.2 Values

A field value is one of the kinds produced by `encoding/json`:

| Kind | Go type in the reference implementation |
|---|---|
| null | `nil` |
| bool | `bool` |
| number | `float64` |
| string | `string` |
| array | `[]any` |
| object | `map[string]any` |

A field can also be **missing**, which is distinct from null.

The reference implementation assumes its collection was decoded by `encoding/json` and rejects other Go types when it is constructed.
This assumption is expected to cause friction because Go is strongly typed; revisit it when real queries start failing.
Other implementations decide how they store values; they only need to report values through `Lookup` as the kinds above.

### 2.3 Operands

Operands are typed with generics, so non-JSON values do not compile:

```go
type Value interface {
    ~bool | ~string |
        ~int | ~int8 | ~int16 | ~int32 | ~int64 |
        ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
        ~float32 | ~float64
}
```

Numeric operands are stored in the filter tree as `float64`.
Two consequences are documented rather than handled: a `float32` operand does not equal the same decimal decoded from JSON (`float32(0.1)` is not `0.1`), and integers above 2^53 lose precision.
Null is never an operand; use `IsNull`.

Operators whose operand is a JSON array or object (`HasAny`, `HasAll`, `SameArray`, `SameObject`, and `Has` with an object) accept typed slices and maps, such as `[]string` or `map[string]int`, and convert them to `[]any` and `map[string]any` at build time.
Elements that are not JSON kinds are a build error.

### 2.4 Regular expressions

Regular expressions use Go's `regexp` package (RE2 syntax) only.
JavaScript regex syntax and flags are not translated.
Case-insensitivity uses an inline `(?i)` flag.

## 3. Paths

A `Path` names a field by segments:

```go
qpdb.Path("name")           // top-level field "name"
qpdb.Path("addr", "city")   // field "city" inside the object in field "addr"
```

Segments are never split, so `Path("a.b")` names a field literally called `a.b`.
Each segment after the first descends into an object.
If any segment before the last is missing, or is not an object, the field is missing.
Array indexes are not supported (see §11).
An empty path is a build error.

`IDPath` is the reserved path for a record's ID.
Each implementation's `Lookup` resolves `IDPath` to that record's ID as a string.
The reference implementation stores the ID in the `___id` field.

## 4. Queries

### 4.1 Building

A `Query` is an immutable value with no connection to a collection.
Every builder method returns a new `*Query`; the receiver is unchanged.

```go
q := qpdb.Where(
        qpdb.Eq(qpdb.Path("state"), "NY"),
        qpdb.Or(qpdb.Gt(qpdb.Path("age"), 40), qpdb.IsNull(qpdb.Path("age"))),
    ).
    OrderBy(qpdb.Asc(qpdb.Path("name"))).
    Offset(10).
    Limit(10)

recs, err := people.Get(ctx, q)
```

| Method | Meaning |
|---|---|
| `Where(filters...)` | Start a query; the filters are AND-ed. |
| `q.Where(filters...)` | AND more filters onto the query. |
| `q.OrderBy(keys...)` | Set the order; replaces any previous `OrderBy`. |
| `q.Offset(n)` | Skip the first `n` results (0-based). |
| `q.Limit(n)` | Keep at most `n` results. `Limit(0)` returns no results. Without `Limit`, there is no limit. |
| `q.Err()` | The first build error, or nil. |

A nil `*Query`, or a query with no filters, matches every record.
Calling `Offset` or `Limit` again replaces the earlier value.

Filtering runs first, then ordering, then offset and limit, regardless of the order the methods were called in.

### 4.2 Build errors

A query records the first error made while building it.
Once a query holds an error, every later builder method returns a query carrying the same error and does nothing else.
Every `Querier[R]` method and every helper returns the query's error before doing any work.

Build errors include an empty path, a nil regular expression, a negative offset or limit, and an empty `OrderBy` key.

## 5. The `Querier[R]` interface

```go
type Querier[R any] interface {
    // Get returns copies of the records matching q.
    Get(ctx context.Context, q *Query) ([]R, error)
    // Count returns the number of records Get would return, after Offset and Limit.
    Count(ctx context.Context, q *Query) (int, error)
    // Lookup returns the value at path p in r; ok is false if the field is missing.
    Lookup(r R, p Path) (v any, ok bool)
}
```

`Count` applies `Offset` and `Limit`, unlike SQL's `COUNT(*)`; for the total behind a paged view, count the query without them.
`Lookup` lets helpers (§7) and the in-memory evaluator (§6.5) read fields from any record type.

Implementations must:

- return copies (§2.1);
- honor context cancellation;
- return `q.Err()` before doing any work;
- return an error wrapping `ErrUnsupported` for any filter node they cannot evaluate.

Concurrency: the reference implementation carries a `sync.RWMutex` but does not lock yet.
Until mutation exists, its collection is read-only after construction.
The package documentation states that implementations are not required to be safe for concurrent use until v1.0.0.

## 6. Filters

### 6.1 The filter tree

Filters are exported data types, not closures, so they can be inspected, printed, and compared, and so an implementation can evaluate them some other way if it needs to.
Most implementations evaluate them with `Match` (§6.5).
The tree has these node kinds:

| Node | Constructor | Meaning |
|---|---|---|
| AND | `And(filters...)` | Every child matches. `And()` matches every record. |
| OR | `Or(filters...)` | Any child matches. `Or()` matches no record. |
| NOT | `Not(filter)` | See §6.2. |
| Comparison | the operator constructors in §6.3 | One test on one path. |
| ID | `ID(ids...)` | The record's ID is one of `ids`. `ID()` matches no record. |
| Predicate | `Func(func(any) bool)` | A Go predicate. It receives the record as `any`; the predicate asserts it to the record type. |

Implementations walk the tree with a type switch.
The package exports the node types and their fields.

### 6.2 Three-valued logic

Evaluation follows SQL's three-valued logic.
A comparison is **unknown** when:

- the field is missing;
- the field is null (except for `IsNull`, which is true or false);
- the field's kind does not suit the operator (for example `Lt` on a string field with a number operand, or `Like` on a number or array field).

`NOT unknown` is unknown.
`AND` and `OR` follow Kleene logic: `false AND unknown` is false, `true OR unknown` is true, and otherwise unknown propagates.
A record matches only if the whole filter is **true**.

Consequences:

- A comparison on a missing or null field never matches, even under `Not`.
- A kind mismatch never matches and is never an error.
- `Not(Eq(p, 1))` does not match records where `p` is missing or null.

### 6.3 Operators

`p` is a `Path`; `v` is a `Value` (§2.3); "field" is the value at `p`.

Equality is strict: the field and operand must be the same kind and the same value.
Numbers compare as `float64`.

| Constructor | True when |
|---|---|
| `Eq(p, v)` | field equals `v` |
| `Ne(p, v)` | field and `v` are the same kind and not equal |
| `In(p, vs...)` | field equals any of `vs`; `In(p)` is false |
| `NotIn(p, vs...)` | `Not(In(p, vs...))` |
| `Lt`, `Lte`, `Gt`, `Gte` `(p, v)` | number vs number numerically, or string vs string by byte order (which equals Unicode code point order) |
| `EqFold(p, s)` | field is a string equal to `s` under Unicode case folding |
| `Like(p, s)` | field is a string containing `s` |
| `LikeFold(p, s)` | as `Like`, case-folded |
| `Prefix(p, s)`, `PrefixFold(p, s)` | field is a string starting with `s` |
| `Suffix(p, s)`, `SuffixFold(p, s)` | field is a string ending with `s` |
| `Regex(p, re)` | field is a string and `re.MatchString(field)` |
| `Contains(p, v)` | field is an array with an element equal to `v` |
| `Has(p, x)` | field contains `x` (§6.4); `x` is a scalar or an object, and an array is a build error |
| `HasAny(p, xs...)` | field contains any of `xs` |
| `HasAll(p, xs...)` | field contains every one of `xs`; `HasAll(p)` is true for any present, non-null field |
| `SameArray(p, a)` | field is an array deep-equal to `a` (same length, same order) |
| `SameObject(p, o)` | field is an object deep-equal to `o` (same key set) |
| `IsNull(p)` | field is present and null; false if missing |
| `IsBool`, `IsNumber`, `IsString`, `IsArray`, `IsObject` `(p)` | field is present and of that kind |

String operators take a `string` operand, `Regex` takes a `*regexp.Regexp`, and `Has`, `HasAny`, `HasAll`, `SameArray`, and `SameObject` take JSON-kind values (`any`, validated at build time).
String operators on non-strings, including arrays, are unknown (§6.2).

### 6.4 Containment

`Has(p, x)` implements the containment that TaffyDB intended (TaffyDB reference §11.3), not its defects:

| field | `x` | True when |
|---|---|---|
| object | string | `x` is a key of the field, even if its value is null |
| object | object | every key of `x` is in the field and the field's value `Has` the value in `x` |
| array | scalar | some element equals `x`, or some element that is an array `Has` `x` |
| array | object | some element `Has` `x` |
| scalar | scalar | same kind and equal |

`Has` never takes an array; use `HasAny` or `HasAll`, which apply `Has` to each element.

### 6.5 In-memory evaluation

```go
func Match[R any](db Querier[R], f Filter, r R) (bool, error)
```

`Match` evaluates a filter against one record through `db.Lookup`.
In-memory implementations use it instead of writing their own evaluator.
It returns an error only for nodes it cannot evaluate.

## 7. Ordering

Order keys are built, not parsed:

| Constructor | Order |
|---|---|
| `Asc(p)`, `Desc(p)` | By value (§7.1). |
| `Natural(p)`, `NaturalDesc(p)` | Natural sort (§7.2). |

Keys are applied left to right as tie-breakers.
The sort is stable.
A query with no `OrderBy` returns records in collection order, except that a query whose only filter is `ID(...)` returns records in the order the IDs were given, with duplicates removed.

### 7.1 Value order

Values of different kinds order by kind: missing, null, bool, number, string, array, object.
Within a kind: `false` before `true`; numbers numerically; strings by byte order; arrays element by element, then by length; objects by sorted key set, then values in key order.
`Desc` reverses the whole order, so missing and null sort last.

### 7.2 Natural order

Strings are split into runs of ASCII digits and runs of non-digits.
A `-` immediately before a digit run belongs to the number, and a single `.` between digit runs makes a decimal.
Number runs compare numerically; other runs compare case-folded.
When run kinds differ at the same position, the number sorts first.
If all compared runs are equal, the shorter run list sorts first, then the original strings compare by byte order so the order is total (this separates `"007"` from `"7"`).
Non-string values use value order (§7.1).

## 8. Read helpers

Everything except `Get` and `Count` is a package function over `Querier[R]`.
Helpers call `Get` and work on the results in memory.

| Helper | Returns | Notes |
|---|---|---|
| `First(ctx, db, q)` | `(R, bool, error)` | `false` if no record matches. Uses `q.Limit(1)`. |
| `Last(ctx, db, q)` | `(R, bool, error)` | |
| `Select(ctx, db, q, paths...)` | `([][]any, error)` | Columns in the requested order. Missing fields are `nil`. |
| `Distinct(ctx, db, q, paths...)` | `([][]any, error)` | Unique tuples by deep equality, first-seen order. Missing and null are both `nil` and dedupe together. |
| `Sum(ctx, db, q, paths...)` | `(float64, error)` | Skips missing and null. Any other non-number is an error. No records gives 0. |
| `Min`, `Max` `(ctx, db, q, p)` | `(any, bool, error)` | Skip missing and null, as SQL aggregates do. `false` if no values remain. |
| `Map(ctx, db, q, fn func(R) T)` | `([]T, error)` | |
| `All(ctx, db, q)` | `(iter.Seq2[int, R], error)` | Index and record for each result; replaces TaffyDB's `each`. |
| `Supplant(ctx, db, q, tpl)` | `([]string, error)` | One string per record. `{name}` is replaced by the top-level field `name` if it is a string or number; otherwise the placeholder is left as is. Plain substitution; no escaping. |
| `Stringify(ctx, db, q)` | `([]byte, error)` | `encoding/json` of the results. |
| `Join(ctx, left, lq, right, rq, on)` | `([]Pair[L, R], error)` | Inner join by nested loop; `on` is `func(L, R) bool`. No `on` gives the cartesian product. |

`Join`'s condition is a Go function, not a filter tree, because the whole collection is in memory (§1).
Merging a joined pair into one record is up to the client.

## 9. Reference implementation and tests

The reference implementation lives in an internal package, `internal/refdb`.
It implements `Querier[map[string]any]` over a fixture dataset decoded by `encoding/json`.
The tests and the examples both use it, and its documentation presents it as the reference for clients writing their own `Querier[R]`.
Clients cannot import it; they read it and copy what they need.

- The constructor deep-copies the collection and rejects values that are not JSON kinds.
- Records keep the `___id` field if present; records without one get an opaque, unique ID.
  Duplicate or non-string `___id` values are an error.
  No `___s` field exists.
- `Lookup` resolves `IDPath` to `___id` and descends nested objects.
- It evaluates filters with `Match` (§6.5).
- It provides a helper that merges joined `map[string]any` pairs in TaffyDB's shape (left fields, then right fields, with a `right_` prefix on collisions).

The fixture dataset starts from Fixture A of the TaffyDB reference (§15) and grows until every test can run against it.
It uses `map[string]any` records because the tests need both missing and null fields, which a Go struct cannot tell apart.

The package's tests are its conformance vectors.
They follow this document, not TaffyDB's results.

## 10. Deferred until after v1.0.0

- A `Mutator[R]` interface: `Insert`, `Update`, `Remove`, `Merge`, `Sort`.
- Events (`OnInsert`, `OnUpdate`, `OnRemove`, `OnChange`), and handlers that call back into the collection while it holds its lock.
- Locking in the reference implementation.
- Templates and forced field-name case.
- Persistence.

## 11. Open questions

- Should there be an `Exists(p)` operator that is true when a field is present, including null?
- Should paths support array indexes?
- Should `Min` and `Max` error on mixed kinds, as `Sum` does, or use value order (§7.1)?
- Should a lone `ID(...)` filter really change the default order (§7), or should collection order always apply?
