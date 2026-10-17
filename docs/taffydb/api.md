# TaffyDB API Reference

Complete reference for TaffyDB 2.7.3 (`taffy.js`, repository
`typicaljoe/taffydb`, commit `5e20824`, 2023-06-15).

This document is written for agents implementing a TaffyDB-style query package
in Go. It specifies what the JavaScript implementation *actually does*, not what
its website says it does. Where the two differ, or where the implementation is
plainly defective, the entry says so and gives a recommendation for a port.

## How to read this document

- **Source of truth.** Every statement was derived from `taffy.js` and checked by
  running it under Node.js 22. Line references (`taffy.js:NNN`) point at that
  file.
- **Defect** marks behavior that is almost certainly unintended. Do not copy it
  unless you need bug-for-bug compatibility.
- **Port note** marks a recommendation for a Go implementation.
- JavaScript semantics matter here. "Strict equality" means JS `===`; "loose
  equality" means JS `==`; "relational comparison" means JS `<`, `>`, `<=`, `>=`
  (string vs string compares UTF-16 code units; any other pairing converts both
  sides to numbers, `null` → `0`, `true` → `1`, non-numeric strings → `NaN`, and
  any comparison involving `NaN` is false).

## Contents

1. [Object model](#1-object-model)
2. [Argument sorting (affects every query method)](#2-argument-sorting-affects-every-query-method)
3. [Creating a database: `TAFFY(data)`](#3-creating-a-database-taffydata)
4. [Running a query: `db(...filters)`](#4-running-a-query-dbfilters)
5. [Filter language](#5-filter-language)
6. [Query object methods](#6-query-object-methods)
7. [Ordering](#7-ordering)
8. [Pagination](#8-pagination)
9. [Database (root) methods](#9-database-root-methods)
10. [Settings and events](#10-settings-and-events)
11. [Static helpers on `TAFFY`](#11-static-helpers-on-taffy)
12. [Execution model](#12-execution-model)
13. [Errors in the official documentation](#13-errors-in-the-official-documentation)
14. [Porting notes for Go](#14-porting-notes-for-go)
15. [Appendix: conformance vectors](#15-appendix-conformance-vectors)

---

## 1. Object model

TaffyDB has four kinds of object.

| Object | How you get it | What it is |
|---|---|---|
| `TAFFY` | Global (browser) or `require('taffydb').taffy` (Node; `package.json` name is `taffydb`, main is `taffy.js`) | Constructor function plus static helpers (§11). |
| Database root, `db` | `db = TAFFY(data)` | A **function**. Calling it starts a query. It also carries the methods `insert`, `merge`, `sort`, `settings`, `store`, and the marker property `TAFFY === true` (§9). |
| Query object, `q` | `q = db(...)` | A lazy, chainable description of a query: filters, order, start, limit. Methods either refine it (return a new query object) or execute it (§6). |
| Record | Elements of `q.get()` | A plain JS object holding the user's fields plus two system fields (below). |

### System fields on every record

| Field | Type | Meaning |
|---|---|---|
| `___id` | string | Record ID, format `T` + 6 digits + `R` + 6 digits, e.g. `T000002R000002`. |
| `___s` | boolean | Liveness flag; `true` while stored. Set to `false` transiently during removal. |

Three underscores each. They are ordinary enumerable properties: they appear in
`get()`, `stringify()`, `select("___id")`, `supplant("{___id}")`, and join output.

**Record ID generation** (`taffy.js:1333-1335`):

- The `R` part is a per-database counter that starts at 1 and is incremented
  *before* use, so the first record in every database is `R000002`.
- The `T` part is a **global** counter shared by all databases on the page.
  It is incremented each time *any* database is created (including the temporary
  database `join` creates), and is read at *insert* time. Records inserted into
  the same database at different times can therefore carry different `T`
  prefixes.
- Both parts are zero-padded to six digits with `String('000000' + n).slice(-6)`,
  so they silently wrap after 999 999.

**Port note:** treat IDs as opaque, unique strings. Do not reproduce the
global-counter behavior.

### Data model

- Records are schemaless JS objects. Field values may be any JS value: string,
  number, boolean, `null`, array, object, function, `Date`, `RegExp`.
- There is no column type system. Comparison semantics come from JS operators.
- Field lookup is **top-level only**. `{"a.b": 1}` looks for a field literally
  named `a.b`; there is no dotted-path syntax. (One level of nesting can be
  matched through the property-match fallback, §5.6.)
- A field whose value is `undefined` is indistinguishable from a missing field.

---

## 2. Argument sorting (affects every query method)

Every query method is registered through `API.extend` (`taffy.js:163-173`), which
wraps the implementation like this:

```js
API[m] = function () { return f.apply(this, sortArgs(arguments)); };
// sortArgs = Array.prototype.slice.call(args).sort()   (taffy.js:52-55)
```

`Array.prototype.sort()` with no comparator converts each argument to a string
and sorts by UTF-16 code unit. **The arguments a method receives are therefore
reordered before it sees them.** Methods that read arguments by position
misbehave whenever the caller's order is not already sorted. The same wrapper
applies to `TAFFY.extend` user methods and to the arguments of `db(...)` itself
(`taffy.js:1608`).

Useful string forms: numbers → their digits (`"10"`, `"9"`); `true`/`false` →
`"true"`/`"false"`; plain objects and query objects → `"[object Object]"`; arrays
→ elements joined by commas (`"state,abbreviation"`); functions → their source
text (`"function () {…"` or, for arrow functions, `"(l, r) => …"`). Code unit
order puts digits < uppercase < `[` < `_` < lowercase.

| Method | Effect of sorting | Severity |
|---|---|---|
| `select(...cols)`, `distinct(...cols)` | Output columns come back in **sorted column-name order**, not the order requested. `select("name","age")` returns `[age, name]` rows. | **Defect**, silent |
| `update(field, value[, runEvent])` | Works only if `String(field) <= String(value)`. Otherwise: a string value becomes the *field name* (`update("state","NY")` sets `NY: "state"`); a non-string value makes the call a silent no-op (`update("age", 99)` does nothing). | **Defect**, silent data corruption |
| `update(fn, false)` | `"false"` sorts before `"function…"`, so `false` is taken as the update object and the call is a silent no-op. `update(fn, true)` and arrow functions happen to work. | **Defect** |
| `update(obj[, runEvent])` | Works: `"[object Object]"` sorts before `"true"`/`"false"`. | OK |
| `supplant(template, true)` | Works only if `template` sorts before the string `"true"`. Otherwise (for example, a template beginning with `{` or with a lowercase letter after `t`) it throws `template.replace is not a function`. | **Defect** |
| `callback(fn, delay)` | Any numeric delay sorts before the function source, so `fn` becomes the number and the timer throws `f.call is not a function`. Only `callback(fn)` works. | **Defect** |
| `join(table, ...conditions)` | The right-hand table must still be first after sorting. See §6.4 for which combinations break. | **Defect** |
| `sum(...cols)`, `filter(...)`, `db(...)` | Order-insensitive in practice. | OK |
| `order`, `limit`, `start`, `each`, `map`, `min`, `max`, `remove`, `count`, `get`, `first`, `last`, `stringify` | Take at most one meaningful argument. | OK |

**Port note:** do not replicate. Positional parameters in Go make this moot.

---

## 3. Creating a database: `TAFFY(data)`

```js
const db = TAFFY();                                   // empty
const db = TAFFY({id: 1, text: "x"});                 // one record
const db = TAFFY([{id: 1}, {id: 2}]);                 // many records
const db = TAFFY('[{"id":1},{"id":2}]');              // JSON text (array or object)
const db = TAFFY([["name","age"], ["Ann",30], ["Bo",25]]);  // header-row form
```

`data` is passed to `db.insert` (§9.1) if it is truthy. Returns the database root
function. Each call creates an independent database with its own records,
settings, and query cache.

---

## 4. Running a query: `db(...filters)`

```js
db()                                   // all records
db({status: "Active"})                 // filter object
db({role: "Admin"}, [{status: "A"}, {status: "P"}])   // AND of args
db("T000002R000003")                   // record ID
db(recordObject)                       // a record previously read from this DB
db(function () { return this.age > 21; })
```

Returns a new query object. **Nothing executes yet** (§12.1).

### 4.1 Argument classification (`taffy.js:1608-1617`, `175-200`)

Each argument is classified as either an **index** (a direct record lookup) or a
**predicate**.

An argument is an index if it is:

1. a string matching the regex `/[t][0-9]*[r][0-9]*/i` *anywhere* in the string
   (so `"T000002R000003"`, but also `"street"`, `"tr"`, `"Matrix"`); or
2. an object with truthy `___id` and `___s` (a record); or
3. an array whose elements are all indexes (an **empty array qualifies**).

Everything else becomes a predicate via the filter compiler (§5).

### 4.2 Index resolution (`taffy.js:1217-1260`)

If there is at least one index argument, the candidate set is built from the
indexes instead of the whole database:

- An ID string that exists in this database contributes that record. Unknown IDs
  are silently dropped (so `db("street")` matches nothing).
- A record object contributes the *current* stored version of the record with
  the same `___id`.
- An array contributes each element, resolved recursively.
- **Defect:** if any top-level index argument was a single ID or record (not an
  array) and the total candidate count is greater than 1, the candidate set is
  emptied. `db(id1)` → 1 record; `db([id1, id2])` → 2 records;
  `db(id1, id2)` → **0 records**.
- Candidates are returned in the order of the index arguments (after argument
  sorting, §2), not in database order.

All predicates are then applied to the candidates (logical AND). With no index
arguments, the candidates are all records in storage order.

### 4.3 Non-filter arguments

- A string that does **not** match the ID regex (e.g. `db("abc")`) compiles to
  nothing and is ignored: it matches **all** records.
- `null`, numbers, booleans, and `undefined` are likewise ignored (match all).

**Port note:** make record-ID lookup an explicit API (e.g. `ByID(ids ...string)`)
instead of sniffing strings. Reject non-filter arguments.

---

## 5. Filter language

This section is the core of any port.

### 5.1 Grammar

```
Filter        := FilterObject | FilterArray | Function | RecordID | Record
FilterArray   := "[" Filter ("," Filter)* "]"         ; logical OR of elements
FilterObject  := "{" (Field ":" Condition)* "}"        ; logical AND of entries
Condition     := Scalar                                 ; shorthand for {is: Scalar}
               | ValueArray                             ; shorthand for {is: ValueArray}
               | OperatorObject
OperatorObject:= "{" (Operator ":" Operand)* "}"       ; logical AND of entries
Operand       := Scalar | ValueArray | Object | RegExp
ValueArray    := "[" Operand ("," Operand)* "]"        ; logical OR over elements (see 5.4)
Scalar        := string | number | boolean | null | undefined | function
```

How the compiler decides between "Scalar" and "OperatorObject" (`taffy.js:287-291`):
a condition is an operator object if `TAFFY.typeOf(condition) === "object"`.
That is true for plain objects **and also for `Date` and `RegExp` instances**.
Consequences:

- `{name: /^A/}` does **not** do a regex match. The `RegExp` is read as an
  operator object with no own enumerable properties, i.e. no operators, so it
  matches **every** record. Use `{name: {regex: /^A/}}`.
- `{created: someDate}` is an operator object with no operators, which matches
  **every** record. Compare dates with a function filter or by converting to
  numbers.

### 5.2 Combination rules

| Construct | Logic |
|---|---|
| Multiple arguments to `db(...)` or `.filter(...)` | AND |
| Chained `.filter()` calls | AND |
| Multiple fields in one filter object `{a: 1, b: 2}` | AND |
| Multiple operators on one field `{age: {gte: 18, lt: 65}}` | AND |
| Array of filters `[{...}, {...}]` (top-level or nested) | OR |
| Array as an operand `{status: ["A", "P"]}`, `{age: {lt: [18, 65]}}` | OR over elements (except `hasAll`) |
| `!` prefix on an operator | NOT of that single test (with defects, §5.5) |
| Empty filter object `{}` | matches every record |
| Empty array of filters `[]` or empty operand array `{status: []}` | matches **no** record |

There is no grouped NOT and no AND-inside-OR except by nesting filter objects in
an array. Anything else needs a function filter.

### 5.3 Evaluating one test

A test is the triple (field, operator, operand). Against a record, in this order
(`taffy.js:312-385`):

1. `v = record[field]`.
2. **If `v === undefined`, the test is false.** This happens before negation, so
   `{x: {"!is": 1}}` does *not* match records lacking `x`, and
   `{x: {isUndefined: true}}` can never match.
3. If the operator starts with `!` and is not `!=` or `!==`, strip the `!` and
   negate the result (§5.5).
4. Dispatch on the operator name (§5.4). Names are **case-sensitive**.
5. Any JS exception (e.g. calling a string method on a number) propagates and
   **aborts the whole query**.

### 5.4 Operators

`v` is the record's field value; `x` is the operand. Each row describes one
element of the operand. When the operand is an array, the test is evaluated once
per element and the results are OR-ed (`hasAll` is the exception).

#### Equality

| Operator | Aliases | Matches when | Notes |
|---|---|---|---|
| `is` | `===` | `v === x` | Default when the condition is a scalar or array. Strict: `30` ≠ `"30"`. Objects and arrays compare by identity, so a literal object or array never matches. `NaN` never matches. |
| `==` | — | `v == x` | JS loose equality: `30 == "30"`, `0 == ""`, `1 == true`, `null == undefined`. |
| `!==` | — | `v !== x` | Distinct operator, not a negated `===`. Still false for missing fields. |
| `!=` | — | `v != x` | Loose inequality. |
| `isnocase` | — | if `v` has `toLowerCase` (i.e. is a string): `v.toLowerCase() === x.toLowerCase()`; otherwise `v === x` | **Throws** if `v` is `null`, or if `v` is a string and `x` is not. Lowercase operator name only. |

#### Ordering

| Operator | Aliases | Matches when |
|---|---|---|
| `lt` | `<` | `v < x` |
| `lte` | `<=` | `v <= x` |
| `gt` | `>` | `v > x` |
| `gte` | `>=` | `v >= x` |

JS relational semantics. `null` acts as `0`, so `{age: {lt: 30}}` matches
`age: null`. A string vs a number compares numerically (`"abc"` → `NaN` → false).

#### String

All of these call string methods on `v`. If `v` is a number, boolean, `null`, or
object, the query **throws** (`mvalue.indexOf is not a function`). Arrays have
`indexOf`, so `like`/`left` on an array field do not throw (see the notes).

| Operator | Matches when | Notes |
|---|---|---|
| `like` | `v.indexOf(x) >= 0` | Case-sensitive substring. `x` is converted to a string. On an array field: strict element membership. |
| `likenocase` | `v.toLowerCase().indexOf(x.toLowerCase()) >= 0` | `x` must be a string. |
| `left` | `v.indexOf(x) === 0` | Prefix. `left: ""` matches every string. On an array field: first element `=== x`. |
| `leftnocase` | `v.toLowerCase().indexOf(x.toLowerCase()) === 0` | |
| `right` | `v.substring(v.length - x.length) === x` | Suffix. If `x` is longer than `v`, compares all of `v` with `x` (false unless equal). `x` must be a string (a number has no `.length`, so the test is false). |
| `rightnocase` | Same on lowercased values. | |
| `regex` | `x.test(v)` | `x` must be a `RegExp` (otherwise it throws). `v` is converted to a string, so `{age: {regex: /^3/}}` matches `30`, and `null` becomes `"null"`. A RegExp with the `g` or `y` flag is stateful (`lastIndex`) and gives order-dependent results. |

#### Collections

| Operator | Matches when | Notes |
|---|---|---|
| `contains` | `Array.isArray(v) && v.indexOf(x) > -1` | Strict element membership. False for non-arrays. Not mentioned in the official documentation. |
| `has` | `TAFFY.has(v, x)` | Recursive containment; see §11 for exact semantics. **Defect:** never matches when `x` is an object (`{addr: {has: {city: "Albany"}}}` is always false). An operand array is OR-ed per element, so `{tags: {has: ["a","c"]}}` means "has a **or** c". |
| `hasAll` | `TAFFY.hasAll(v, x)` with the **whole** operand | The only operator that does not split an array operand. `{tags: {hasAll: ["a","b"]}}` means every element is contained. |
| `hasall` | per element: `TAFFY.hasAll(v, x_i)`, OR-ed | Lowercase spelling. The code checks for `'hasall'` while only `'hasAll'` gets whole-array handling, so `hasall` behaves like `has` (any element). |

#### Type tests

| Operator | Matches when |
|---|---|
| `isString`, `isNumber`, `isBoolean`, `isArray`, `isObject`, `isNull`, `isFunction`, `isUndefined` | `TAFFY.isX(v) === x` |

`x` should be `true` or `false`; `{age: {isNumber: false}}` matches non-numbers.
Type names come from `TAFFY.typeOf` (§11): `isObject` is false for arrays and
`null` and true for `Date` and `RegExp`; `isNumber` is true for `NaN`.
`isUndefined: true` never matches (step 2 of §5.3).

#### Broken comparison operators

| Operator | Documented as | Actual behavior |
|---|---|---|
| `isSameArray` | v is the same array as x | **Defect:** evaluates `TAFFY.isSameArray(v) === x` with one argument, which is always `false === x`. With an array operand (the documented usage) it never matches. `{isSameArray: false}` matches every record with the field. |
| `isSameObject` | v is the same object as x | **Defect:** same failure. Never matches with an object operand. |

**Port note:** implement these as deep equality on arrays and objects
(order-sensitive for arrays, key-set equality for objects).

### 5.5 Negation (`!` prefix)

Any operator can be prefixed with `!` (quote the key): `{"!is": x}`,
`{"!like": x}`, `{"!hasAll": [...]}`, `{"!city": "Reno"}`. `!=` and `!==` are their
own operators and are not treated as negations.

Correct cases:

- With a single operand, the test is the logical NOT of the base test.
- Records where the field is missing or `undefined` are still excluded (§5.3
  step 2).

**Defect: negation with an array operand.** Stripping the `!` mutates the
operator name in a variable shared by all the per-element tests generated from
one `field: {op: [...]}` entry, while the "negate" flag is per test. Only the
**first** element's test is negated; the rest run un-negated. The OR then gives:

```
{x: {"!is": [a, b, c]}}   ≡   x !== a  OR  x === b  OR  x === c
```

Verified: on `age` values `30, 25, null, 41`, `{age: {"!is": [30, 25]}}` returns
`25, null, 41`, and swapping the operand order to `[25, 30]` returns
`30, null, 41`.

**Port note:** implement `!op` with an array operand as NOT(any element matches),
i.e. NOT IN.

### 5.6 Fallback: property match and helper dispatch

Any operator name not listed above, call it `k`, is resolved like this
(`taffy.js:367-379`):

1. **Property match.** If `k` does *not* contain the substring `"is"`, `v` is not
   `null` or `undefined`, and `x` is neither a plain object nor an array, the test
   is `v[k] === x`. This is what makes one-level nested matching work:

   ```js
   db({addr: {city: "Reno"}})        // record.addr.city === "Reno"
   db({name: {length: 3}})           // record.name.length === 3
   ```

   Limits:
   - One level only. `{a: {b: {c: 1}}}` does not work.
   - A nested key containing `"is"` anywhere (`district`, `list`, `visible`,
     `isActive`) skips this step and normally ends up false.
   - Because operator names are tried first, a nested key that collides with an
     operator name (`like`, `left`, `has`, `lt`, …) is read as that operator.
   - An array operand is OR-ed per element, as with any operator.
2. **Helper with an `is` prefix.** Otherwise, if `TAFFY[k]` is a function and `k`
   starts with `"is"`, the test is `TAFFY[k](v) === x`. This is how the type
   tests work.
3. **Helper.** Otherwise, if `TAFFY[k]` is a function, the test is
   `TAFFY[k](v, x)` (truthy). This is how `hasAll` works.
4. Otherwise false. Unknown operators (`foo`, `IS`) match nothing; they do not
   raise an error.

**Port note:** support nested matching with an explicit path syntax and reject
unknown operators with an error.

### 5.7 Function filters

```js
db(function () { return this.status === "Active" && this.age > 21; })
```

Called once per candidate record with `this` bound to the record and **no
arguments** (arrow functions therefore cannot see the record). A truthy return
value means match. Any function argument disables the query cache for that
query (§12.3).

### 5.8 Record and ID filters

- A record ID string (per the regex in §4.1) or a record object passed to
  `db(...)` is an index lookup (§4.2).
- The same values passed to `.filter(...)` or nested inside a filter array
  compile to the predicate `{___id: id}`, an ordinary equality filter, so the
  §4.2 "multiple single IDs → empty" defect does not apply there.

---

## 6. Query object methods

Every method below is available on a query object `q = db(...)`. Methods marked
**executes** run the query (§12.1). Where "records" is the return value, they are
**live references** to stored objects (§12.4).

The query object also exposes `context`, `getDBI`, and `getroot`. These are
internal plumbing used by extensions (§11.4). It has `insert` and `extend`
properties set to `undefined`.

### 6.1 Refinement (return a new query object)

#### `q.filter(...filters) → Query`

Adds predicates; AND-ed with the existing ones. Arguments use the filter language
(§5). ID strings and records become `___id` predicates (§5.8), not index lookups.

- Returns a new query; the original's predicates are unchanged.
- **Defect (cache poisoning):** the new query shares the original's raw-filter
  list, which is used as the cache key. If the original has **not yet executed**
  and caching is on (the default), the original will afterwards return the
  child's cached results:

  ```js
  const q = db({x: {gt: 0}});      // 3 matching records
  q.filter({x: 1}).count();        // 1
  q.count();                       // 1   (should be 3)
  ```

  Running the original first, or setting `cacheSize: 0`, avoids it.

#### `q.order(spec: string) → Query`

Sets the result order. Replaces any previous `order()` on this chain; calls do
not accumulate. Grammar and comparison semantics are in §7.

#### `q.limit(n: number) → Query`

Keeps at most `n` results. §8.

#### `q.start(n: number) → Query`

Skips to the `n`th result (**1-based**). §8.

### 6.2 Reading results (execute)

| Method | Returns | Semantics |
|---|---|---|
| `get()` | `Record[]` | All results. The array may be shared with the query cache; do not mutate it. |
| `first()` | `Record \| false` | First result, or **`false`** (not `null`/`undefined`) if none. |
| `last()` | `Record \| false` | Last result, or `false`. |
| `count()` | `number` | Number of results. |
| `stringify()` | `string` | `JSON.stringify(get())`. Includes `___id` and `___s`. |
| `select(col)` | `any[]` | `record[col]` for each result; `undefined` for missing fields. |
| `select(c1, c2, …)` | `any[][]` | One row per result. **Columns come back in sorted name order** (§2). `select("b","a","C","_z")` returns `[[C, _z, a, b]]`. |
| `distinct(col)` | `any[]` | Unique values of `col`, in first-seen order, deduplicated with `===` (objects and arrays by identity). **Defect:** the dedupe loop skips `undefined` entries, so the output contains one `undefined` *per* record lacking the field (values `1, –, 1, –, –, 2` → `[1, undefined, undefined, undefined, 2]`). |
| `distinct(c1, c2, …)` | `any[][]` | Unique tuples, element-wise `===`. Columns in sorted name order (§2). |
| `sum(...cols)` | `number \| string` | Starting from `0`, adds `record[c] \|\| 0` for every column and every result. Falsy values count as 0. **A string value turns the sum into string concatenation** (`sum("name")` → `"0Alicebob…"`). |
| `min(col)` | `any \| null` | Smallest value by `<`. `null` if there are no results. **Defect:** the "no value yet" sentinel is `null`, so a record whose value is `null` resets the scan. With values `30, 25, null, 41`, `min` returns `41`. If the first result lacks the field, the result is `undefined`. |
| `max(col)` | `any \| null` | Largest by `>`; same sentinel defects. |
| `map(fn)` | `any[]` | `fn(record)` for each result. **One argument only**; the official documentation's `(record, index)` is wrong. |
| `supplant(tpl)` | `string` | For each result, replaces every `{key}` in `tpl` with `record[key]` if that value is a string or number; otherwise the `{key}` placeholder is left as-is. Concatenates the per-record strings. |
| `supplant(tpl, true)` | `string[]` | Same, one string per record. Subject to the argument-sorting defect (§2). |

`{key}` matches the regex `/\{([^{}]*)\}/g`. Keys are not trimmed, and there is no
escaping or HTML encoding.

#### `q.each(fn) → Query`

Calls `fn(record, index)` for each result, in order. Returning `TAFFY.EXIT` (the
string `"TAFFYEXIT"`, so returning that string also stops the loop) stops
iteration. Returns the same query object.

#### `q.callback(fn) → null`

Schedules `setTimeout(…, 0)`, then runs the query and calls `fn` with `this` set
to a query object holding the results. `callback(fn, delay)` is broken by
argument sorting (§2) and throws asynchronously.

### 6.3 Mutating (execute)

#### `q.update(changes[, runEvent]) → Query`

Applies changes to every result, one record at a time, via the internal update
(§9.6). Accepted forms:

| Form | Meaning | Works? |
|---|---|---|
| `update({f: v, …})` | Shallow-merge the object into each record. | Yes |
| `update({…}, false)` | Same, without calling `onUpdate`. | Yes |
| `update(fn)` | `fn` is called with `this` = a **shallow copy** of the record and no arguments. If it returns a plain object, that object is merged in. Any other return value (including `undefined`) means no change. Mutating `this` has no effect unless you `return this`. | Yes |
| `update(fn, true)` | Same. | Yes |
| `update(fn, false)` | Intended: no event. | **No**: silent no-op (§2). Works with an arrow function. |
| `update("field", value[, runEvent])` | Intended: set one field. | **Only if `String("field") <= String(value)`**; otherwise it corrupts data or is a no-op (§2). |

Notes:

- `runEvent`: `onUpdate` runs if `runEvent` is truthy or omitted.
  `false`, `0`, `null`, and `""` suppress it.
- Records are replaced, not mutated: each updated record is a new object.
  References obtained earlier keep the old values.
- Setting a field to `undefined` keeps the key with value `undefined`, so filters
  on that field stop matching. A field cannot be deleted.
- Returns the same query object (marked for re-execution).

#### `q.remove([runEvent]) → number`

Deletes every result and returns how many were removed. `onRemove` fires once
per removed record, **in reverse storage order**, unless `runEvent` is falsy
(not `undefined`). The official documentation says `remove(true)` suppresses
events; it is `remove(false)`.

### 6.4 `q.join(right, ...conditions) → Query`

Inner join (nested-loop, cartesian with pruning) of this query's results (left)
against `right`.

- `right`: a database root (`db2`) or a query object (`db2({...})`). A plain
  array is not accepted. If you pass a query object, its filters apply.
- Each condition is either:
  - `[leftCol, rightCol]`: `left[leftCol] === right[rightCol]`;
  - `[leftCol, op, rightCol]` with `op` one of `===`, `!==`, `<`, `>`, `<=`, `>=`,
    `==`, `!=`. Any other `op` throws the **string** `"<op> is not supported"`.
    A one-element array throws `"undefined is not supported"`;
  - `function (l, r) { return bool; }`.
- Conditions are AND-ed and short-circuit in argument order (after sorting).
  With **no** conditions, the result is the full cartesian product.
- Output row: all left fields, then every right field except `___id`/`___s`. A
  right field whose name already exists in the left row is renamed with a
  `right_` prefix (only one level: a second collision overwrites).
- The result is a query over a **new temporary database** of joined rows, with
  fresh `___id` values. Mutating it does not affect either source.

**Argument sorting defect (§2).** `right` must still sort first. A root sorts as
`"function () {…"` and a query object as `"[object Object]"`. Verified failures:

| Call | Result |
|---|---|
| `q.join(db2, ['state','abbreviation'])` | works (`"function"` < `"state,…"`) |
| `q.join(db2, ['a','abbreviation'])` | throws `right_table.each is not a function` |
| `q.join(db2(), ['a','abbreviation'])` | works (`"[object"` < `"a,…"`) |
| `q.join(db2(), ['A','B'])` or `q.join(db2, ['A','B'])` | throws (uppercase sorts first) |
| `q.join(db2, function (l, r) {…})` | works |
| `q.join(db2, (l, r) => …)` | throws `TAFFY DB or result not supplied` |

The official documentation marks `join` as experimental.

---

## 7. Ordering

Used by `q.order(spec)` (result order) and `db.sort(spec)` (storage order).

### 7.1 Spec grammar

```
spec      := term ("," term)*
term      := column [" " direction]          ; each term is trimmed of outer whitespace
direction := "logical" | "logicaldesc" | "asec" | "desc"
```

- The column name is the text before the first space; the direction is the text
  after it and up to the *next* space. Exactly one space is expected:
  `"col  desc"` (two spaces) yields an empty direction.
- No direction means `logical`.
- Ascending is spelled **`asec`** (sic). `asc`, `ASC`, `DESC`, and any other
  unrecognized direction make that term **compare as equal for every pair**: it
  silently has no effect.
- Terms are applied left to right as tie-breakers. Remaining ties keep their
  previous relative order (modern JS `Array.prototype.sort` is stable).
- Calling `order()` again replaces the previous spec.

### 7.2 `asec` / `desc`

Compare raw values with JS `<` and `>` (see "How to read this document"). If
neither is true (equal, `undefined`, `NaN`, incomparable mixed types), the term
is a tie. String comparison is case-sensitive code-unit order (`"File1"` <
`"a"`). `null` behaves as `0`.

### 7.3 `logical` / `logicaldesc` (natural sort; the default)

Each value is tokenized (`numcharsplit`, `taffy.js:554-610`):

1. Convert to a string with `String(v)`: `null` → `"null"`, `undefined` →
   `"undefined"`, `30` → `"30"`, objects → `"[object Object]"`.
2. Split into maximal runs of characters that are in `[0-9.]` and runs that are
   not.
3. Numeric runs become numbers via `parseFloat` (`"1.2.3"` → `1.2`, `"."` →
   `NaN`). Other runs are lowercased strings. The `-` sign is **not** numeric:
   `"-5"` → `["-", 5]`.
4. The empty string tokenizes to `[]`.

Comparison: walk both token lists up to the shorter length; the first position
where `a < b` or `a > b` decides. If none decides, the shorter list sorts first.
`logicaldesc` inverts both rules.

Properties:

- Case-insensitive: `["File1", "file2", "file10"]` sorts naturally.
- Mixed number/string tokens never compare as less or greater, so they tie. The
  comparator is not a total order on mixed data, and the output for a column
  mixing numbers and words (or containing negative numbers, `null`, or missing
  values) depends on the sort algorithm. Verified example, ascending:
  `[10, 9, -1, 2.5, 100]` → `[2.5, 9, 10, 100, -1]`.
- Verified string example, ascending:
  `["x2","x10","X3","x2a","x2.5","","y"]` → `["","x2","x2a","x2.5","X3","x10","y"]`.

**Port note:** define a total order. Suggested: compare token lists element-wise;
number < string when token kinds differ; parse a leading `-` before digits as
part of the number; `null`/missing sort first (or last), consistently.

---

## 8. Pagination

`start` and `limit` are stored on the query and applied **after filtering and
ordering**, regardless of the order in which they were called
(`taffy.js:1507-1532`):

1. If `start` is truthy, drop results at 1-based positions `< start`.
2. If `limit` is truthy, keep the first `limit` of the remainder.

| Call | Records (from 1..6) |
|---|---|
| `limit(2)` | 1, 2 |
| `start(1)` | 1–6 (no offset) |
| `start(3)` | 3–6 |
| `start(3).limit(2)` or `limit(2).start(3)` | 3, 4 |
| `order("i desc").start(2).limit(2)` | 5, 4 |
| `limit(2).filter({i: {gt: 3}})` | 4, 5 (the limit applies after the filter) |
| `limit(0)`, `start(0)` | no effect (`0` is falsy) |
| `start(9)` | none |
| `limit(-1)` | none |
| `start(-1)` | no effect |
| `limit(1.5)` | 2 records |

**Port note:** expose 0-based `Offset` and `Limit`, validate non-negative
integers, and document that 0 means "no limit".

---

## 9. Database (root) methods

`db` is the function returned by `TAFFY(...)`.

### 9.1 `db.insert(data[, runEvent]) → Query`

Adds records. Returns a query over exactly the inserted records (index lookup by
their new IDs).

`data` may be:

| Input | Effect |
|---|---|
| object | One record. |
| array of objects | One record per element, in order. `undefined` elements are skipped. |
| JSON string | Parsed with `JSON.parse`, then treated as above. Invalid JSON throws. |
| array whose **first element is an array** | Header-row form: element 0 lists column names, and each later array element is a row mapped by position. `[["name","age"],["Ann",30]]` → `{name:"Ann", age:30}`. Not mentioned in the official documentation. |
| array of primitives, or a primitive | Throws `TypeError` (cannot add `___id` to a primitive). |

For each record, in order:

1. Apply `forcePropertyCase` to its keys (this produces a copy), if set.
2. Assign `___id` and `___s = true`. **This mutates the caller's object** unless
   step 1 made a copy.
3. If `template` is set, shallow-merge as `{...template, ...record}`; record
   values win and the result is a copy.
4. Append to storage. Records are stored **by reference** when no copy was made:
   later mutation of the caller's object changes the database without any
   events.
5. Call `onInsert` with `this` = the record, unless `runEvent` is falsy and not
   `undefined`.
6. Mark the database changed (§12.3), which also fires `onDBChange` and the
   `localStorage` sync asynchronously.

There is no uniqueness constraint on any field.

### 9.2 `db.merge(records, key = "id", runEvent = false) → Query`

Upsert. `records` is an array (or a single object). For each record `o`:

- Find the first stored record where `stored[key] === o[key]`.
- If found: update it with `o` (shallow merge, §9.6). Otherwise insert `o`.
- Events default to **off** here (the opposite of `insert`). Pass `true` to fire
  `onInsert`/`onUpdate`.

Returns `db({[key]: [all o[key] values]})`: every record whose key matches any
merged key, including pre-existing duplicates.

Not wrapped by argument sorting, so the parameter order is reliable.

### 9.3 `db.sort(spec) → true`

Reorders **storage** using the §7 rules. This changes the default order of every
later query. Unlike `q.order()`, it is not query-scoped. The official
documentation calls this `db.order()`; that name does not exist on the root.

### 9.4 `db.settings([patch]) → Settings`

With no argument, returns the live settings object. With an object, shallow-merges
it into the settings and returns the result. **Side effect:** if
`patch.template` is set, every existing record is immediately updated with the
template via `db().update(template)`, which **overwrites** existing values (the
template wins here, unlike on insert). See §10 for the fields.

### 9.5 `db.store(name) → db`

Browser only. Loads the `localStorage` key `"taffy_" + name` (if present and
non-empty) by inserting its JSON into the database. It then persists the
database under that key and sets `settings.storageName = name`, so every later
change re-serializes the whole database asynchronously. Returns the root (the
official documentation's "returns true/false" is wrong).

In Node, it throws `ReferenceError: localStorage is not defined`.

### 9.6 Internal update semantics

Shared by `q.update` and `db.merge` (`taffy.js:1366-1403`):

1. Apply `forcePropertyCase` to the change keys.
2. `new = {...old, ...changes}` (shallow).
3. `changed` = the keys of `new` where `old[k] === undefined || old[k] !== new[k]`.
   Object and array values in `changes` always count as changed, even when
   deep-equal.
4. If nothing changed: stop (no event, no change marker).
5. Call `onUpdate` with `this` = `new` and arguments `(old, changed)`, unless
   suppressed.
6. Replace the stored record with `new`.

### 9.7 `db.TAFFY`

Always `true`. Used internally to recognize a database root.

---

## 10. Settings and events

| Setting | Default | Meaning |
|---|---|---|
| `template` | `false` | Object of default fields. On insert, it is merged under the record (record wins). When set via `settings()`, it is applied **over** all existing records (template wins). |
| `forcePropertyCase` | `null` | `"lower"` or `"upper"`: rewrite field names on insert, on update, and in header rows. Any other value (including the documented `"higher"`) is ignored. Filters are **not** case-folded, so filter field names must already use the forced case. |
| `onInsert` | `false` | `function () {}`, `this` = the new record. Synchronous. |
| `onUpdate` | `false` | `function (oldRecord, changes) {}`, `this` = the new record. Synchronous, before storage. |
| `onRemove` | `false` | `function () {}`, `this` = the removed record. Synchronous, in reverse storage order. |
| `onDBChange` | `false` | `function () {}`, `this` = the **internal storage array**. Asynchronous (`setTimeout 0`): once per inserted or updated record, once per `remove()` call, once per `db.sort()`. **Defect:** it also fires whenever an already-executed query object is executed again, even with no change. |
| `storageName` | `false` | `localStorage` key suffix for automatic persistence (set by `store()`). |
| `cacheSize` | `100` | Query cache size; `0` disables it. Eviction is broken: the whole cache is dropped after every `2 × cacheSize` cache writes. |
| `name` | `''` | Unused. |

Event suppression flags (`runEvent`) are honored by `insert`, `update`, `merge`,
and `remove`. Only a falsy value other than `undefined` suppresses.

---

## 11. Static helpers on `TAFFY`

### 11.1 Iteration

#### `TAFFY.each(a, fn)`

- If `a` is a non-array, calls `fn(a, 0)` once.
- If `a` is a one-element array, calls `fn(a[0], 0)`, even when `a[0]` is
  `undefined`.
- Otherwise calls `fn(item, index)` for each item, **skipping `undefined`**.
- `null` → `fn(null, 0)`.
- If `fn` returns `TAFFY.EXIT`, iteration stops.

#### `TAFFY.eachin(obj, fn)`

Calls `fn(value, key, ordinal)` for each own enumerable property of `obj`.
Returning `TAFFY.EXIT` stops iteration.

#### `TAFFY.EXIT`

The string `"TAFFYEXIT"`.

### 11.2 Type tests

`TAFFY.typeOf(v)` returns one of `"string" | "number" | "boolean" | "undefined" |
"function" | "null" | "array" | "object" | "symbol" | "bigint"`. An object is
`"array"` if it has a numeric `length` that is not enumerable (true arrays); so
`{length: 1}` is `"object"`. `Date` and `RegExp` are `"object"`. `NaN` is
`"number"`.

`TAFFY.isString`, `isNumber`, `isObject`, `isArray`, `isBoolean`, `isNull`,
`isFunction`, `isUndefined` each return `typeOf(v) === "<type>"`.

### 11.3 Comparison and merging

#### `TAFFY.mergeObj(a, b)`

Returns a new object with the own enumerable properties of `a`, then of `b`
(`b` wins). Shallow.

#### `TAFFY.getObjectKeys(o)`

Returns the own enumerable keys of `o`, sorted by code unit (`["C","a","b"]`).

#### `TAFFY.isSameArray(a, b)`

Both are arrays and `a.join(",") === b.join(",")`. Quirks:
- `[1,2]` equals `["1","2"]`.
- `["a,b"]` equals `["a","b"]`.
- Any two arrays of plain objects are equal.

#### `TAFFY.isSameObject(a, b)`

Both are plain objects (per `typeOf`), they have the same sorted key set, and for
each key one of these holds:
- both values are objects and `isSameObject`;
- both are arrays and `isSameArray`;
- the values are `===`.

#### `TAFFY.has(container, item)`

Recursive containment.

| `container` | `item` | Result |
|---|---|---|
| database root | any | Intended: query non-empty. **Defect:** always `false`. |
| object | string | `container[item] !== undefined` (key present with a defined value). |
| object | object | Intended: every key of `item` is in `container` with `has` recursively. **Defect:** always `false` (the accumulator starts as `false` and the loop requires `true`). |
| object | array | `true` if `has(container, element)` for **any** element. |
| array | object | `true` if any element `has` the item (so always `false` in practice, given the row above). |
| array | array | `true` if any `item` element is contained in any `container` element. |
| array | string or number | `true` if any element `has` it, recursing into nested arrays (`has([[1,2]], 2)` → `true`). |
| array | boolean, `null`, or other | `false`. |
| string | string | `container === item`. Not a substring test. |
| other | other | `typeOf(container) === typeOf(item) && container === item`. |

#### `TAFFY.hasAll(container, items)`

If `items` is an array: `true` iff `has(container, x)` for **every** element
(vacuously `true` for `[]`). Otherwise `has(container, items)`.

### 11.4 `TAFFY.extend(name, fn)`

Adds (or **replaces**, without warning, including built-ins) a method on every
query object of every database. The method receives its arguments **sorted**
(§2), and `this` is the query object. Inside, run the query and read the results
like this:

```js
TAFFY.extend("avg", function (col) {
  this.context({ results: this.getDBI().query(this.context()) });
  const rs = this.context().results;
  let total = 0;
  TAFFY.each(rs, r => { total += r[col]; });
  return total / rs.length;
});
db().avg("price");
```

`context()` returns the query's internal state:
`{q, filterRaw, index, order, limit, start, results, run, sort, settings}`.
`getDBI()` exposes the internal database interface: `query`, `insert`,
`update(id, changes, runEvent)`, `remove(id)`, `removeCommit(runEvent)`, `sort`,
`dm`. `getroot(context)` builds a query object from a context.

---

## 12. Execution model

### 12.1 Laziness

`db(...)`, `filter`, `order`, `limit`, and `start` only build a description.
The query runs when an executing method is called: `get`, `first`, `last`,
`count`, `select`, `distinct`, `sum`, `min`, `max`, `map`, `each`, `supplant`,
`stringify`, `update`, `remove`, `join`, or the `callback` timer.

### 12.2 Pipeline

```
candidates = indexes ? resolve(indexes) : all records (storage order)
results    = candidates where every predicate is true
results    = order(results)            if order spec present
results    = start/limit(results)      if either is set
```

### 12.3 Caching and staleness

- Each database has a change timestamp (`dm`, a `Date` with millisecond
  resolution). It is bumped on every insert, real update, removal, and `sort`,
  and bumping it clears the query cache.
- **Per-database cache.** It is keyed by the JSON of the query's raw filters,
  order, start, and limit (RegExps are stringified for the key). Queries with a
  function filter are not cached.
- **Per-query-object results.** Once executed, a query object keeps its results
  and re-runs only if `dm` is strictly newer than its run time. **Defect:** with
  millisecond resolution, a change made in the same millisecond as the previous
  run is missed, so the query object returns stale results. Verified: running
  `count()`, inserting a matching record, then running `count()` again on the
  same query object returns the old count.
- `update` and `remove` mark their query object for re-execution.
- See also the `filter()` cache-poisoning defect (§6.1).

**Port note:** do not cache by default. If you do, invalidate by a monotonic
version counter, not a timestamp.

### 12.4 References and mutation

- `get()`, `first()`, `last()`, `each`, `map`, and event handlers expose
  **stored objects**. Mutating them changes the database silently: no events,
  no cache invalidation, no change timestamp.
- `update` replaces stored objects, so previously obtained references go stale.
- `insert` stores the caller's object (§9.1).

**Port note:** return copies, or document that results are read-only.

### 12.5 Concurrency

JavaScript is single-threaded; TaffyDB has no locking. Events, `onDBChange`, and
`localStorage` sync run synchronously or on timers.

**Port note:** a Go server needs its own synchronization (e.g. `sync.RWMutex`
around storage; take the write lock for update, remove, insert, and sort).

### 12.6 Performance

All queries are linear scans except direct ID lookups. Joins are nested loops.
There are no secondary indexes.

---

## 13. Errors in the official documentation

The official documentation is taffydb.com, i.e. `docs/*.html` in the
repository.

| Documentation says | Actual |
|---|---|
| `db.order()` sorts the database | The method is `db.sort()`. |
| `forcePropertyCase` can be `"lower"` or `"higher"` | `"lower"` or `"upper"`. |
| `remove(true)` skips `onRemove` | `remove(false)` skips it. |
| `map(fn(record, recordnumber))` | `fn` receives only `record`. |
| `store()` returns true/false | Returns the database root; throws outside browsers. |
| `sum` example uses `db().last("balance")` | Typo for `db().sum("balance")`. |
| `select("charges","credits","balance")` → `[[-24,20,-4]]` (requested order) | Columns come back in sorted order: `balance, charges, credits`. |
| `update("column","value")` | Unreliable (§2). |
| `hasAll` description is identical to `has` | `hasAll` requires all operand elements; `has` with an array means any. |
| `isSameArray` / `isSameObject` operators | Do not work (§5.4). |
| `isUndefined` operator | Can never be true. |
| Arrays as operands give "IN" | Correct, but `!op` with an array is not NOT IN (§5.5). |
| README: `require('taffy')`, `nodeunit_suite.js`, `install_dev.sh` | The package is `taffydb`; the tests are `test/t.js`; there is no `install_dev.sh`. |
| Not documented at all | `contains`, `hasall`, the header-row insert form, nested property match (§5.6), `TAFFY.EXIT`, `mergeObj`, `typeOf`, `getObjectKeys`, `storageName`, the ID format, argument sorting. |

---

## 14. Porting notes for Go

### 14.1 Value model

Records arrive as JSON, so a natural representation is `map[string]any` with
values decoded by `encoding/json`: `float64`, `string`, `bool`, `nil`, `[]any`,
`map[string]any`. Consider `json.Decoder.UseNumber()` if integer precision above
2^53 matters.

| JS value | Go value | Note |
|---|---|---|
| missing / `undefined` | key absent | Every operator returns false (§5.3). Keep this; it is deliberate and observable. |
| `null` | `nil` | Distinct from absent. `{f: null}` matches only explicit nulls. |
| `RegExp` | — | Not representable in JSON. For a wire format, accept `{"regex": "pattern"}` with a string and compile it with `regexp` (RE2 syntax, which differs from JS: no backreferences or lookaround). Decide how flags such as `i` are passed (e.g. an inline `(?i)` prefix). |
| function filter | — | Not representable on the wire. Offer a Go-side `func(Record) bool` predicate in the API, but not over HTTP. |
| `Date` | — | Store as a string (RFC 3339) or a number. RFC 3339 strings in UTC compare correctly with `lt`/`gt` as strings. |

### 14.2 Operator semantics to pin down

JS operators coerce types; Go will not. Pick and document a rule for each.
Suggested defaults:

- `is` / `===`: equal only if the same JSON kind and the same value. Deep
  equality for arrays and objects (JS uses identity, which can never match a
  filter literal, so deep equality is strictly more useful).
- `==`: if you support it at all, implement JS loose equality for JSON kinds
  only: number↔string via numeric parse, bool→number, `null == null`.
- `lt`/`lte`/`gt`/`gte`: number vs number numerically, string vs string by
  bytes or code points; any other pairing is false (JS would coerce; see §7.2).
- String operators on non-string values: return false instead of failing the
  query.
- `like`/`left`/`right` on an array field: decide explicitly (JS gives membership
  and first-element for `like`/`left`, and throws for `right`). Suggested: false,
  and steer callers to `contains`.
- `has`/`hasAll`: implement the intended recursive containment, including the
  object/object case, which is broken in JS.
- `isSameArray`/`isSameObject`: implement deep equality (broken in JS).
- `!op`: NOT of the base test; with an array operand, NOT IN (broken in JS).
  Keep "absent field → false" even under negation for compatibility, or
  document that you changed it.
- Unknown operators: return an error (JS silently matches nothing).
- Operator names: case-sensitive, as in JS. Accept both `hasAll` and `hasall`
  only if you need compatibility; their JS meanings differ (§5.4).

### 14.3 Behaviors to replicate vs. fix

| Behavior | Replicate? |
|---|---|
| Filter grammar, AND/OR rules (§5.1–5.2) | Yes |
| Absent field → false for every operator | Yes (observable and reasonable) |
| Empty filter matches all; empty array matches none | Yes |
| One-level nested match via unknown operator names (§5.6) | Prefer an explicit path syntax; if you replicate it, document the `"is"`-substring exclusion. |
| Default `logical` (natural, case-insensitive) sort | Yes, with a total order (§7.3 port note) |
| Misspelled `asec`; unknown direction = no-op | Accept `asec` for compatibility, also accept `asc`; reject unknown directions. |
| 1-based `start` | Your choice; JS clients will send 1-based values. |
| `first()`/`last()` returning `false` | Return `(Record, bool)`. |
| `sum` string concatenation; `min`/`max` `null` reset | No |
| Argument sorting (§2) | No |
| Global ID counter; ID format | No; IDs are opaque. Keep the `___id` field name if JS clients read it. |
| `has`, `isSame*`, `!`-with-array, `filter()` cache poisoning, stale query objects, `onDBChange` on re-read | No (defects) |
| Join output shape (left fields, then right fields with a `right_` prefix on collision) | Yes, if you implement join. |

### 14.4 Suggested surface

A shape that maps directly onto this reference:

```go
type Record map[string]any

type DB interface {
    Insert(recs ...Record) ([]string, error)                 // returns IDs
    Merge(key string, recs ...Record) ([]string, error)
    Sort(spec string) error
    Query(filters ...Filter) *Query
    ByID(ids ...string) *Query
}

type Query struct{ /* filters, order, offset, limit */ }

func (q *Query) Filter(f ...Filter) *Query
func (q *Query) Order(spec string) *Query
func (q *Query) Offset(n int) *Query     // 0-based; map from JS start(n) as n-1
func (q *Query) Limit(n int) *Query
func (q *Query) Get() ([]Record, error)
func (q *Query) First() (Record, bool, error)
func (q *Query) Count() (int, error)
func (q *Query) Select(cols ...string) ([][]any, error)    // columns in requested order
func (q *Query) Distinct(cols ...string) ([][]any, error)
func (q *Query) Update(changes Record) (int, error)
func (q *Query) Remove() (int, error)

// Filter is the decoded JSON filter language from §5:
// map[string]any (AND), []any (OR), or a Go predicate func(Record) bool.
type Filter any
```

Compile filters once into a predicate tree (field → []test, with
AND/OR/NOT nodes) and validate operators at compile time, rather than
interpreting the raw `map[string]any` per record.

---

## 15. Appendix: conformance vectors

All results were produced by `taffy.js` 2.7.3 under Node.js 22. "JS result" is
the actual output; where it is a defect, "Intended" gives the behavior a port
should produce.

### Fixture A

```json
[
  {"id":1,"name":"Alice","state":"NY","age":30,"tags":["a","b"],"addr":{"city":"Albany","district":"D1"}},
  {"id":2,"name":"bob","state":"NV","age":25,"tags":["b","c"],"addr":{"city":"Reno"}},
  {"id":3,"name":"Carol","state":"MA","age":null},
  {"id":4,"name":"dave","state":"NY","age":41}
]
```

Results are listed as `name` values, in storage order.

| Filter | JS result | Intended (if different) |
|---|---|---|
| `{}` | Alice, bob, Carol, dave | |
| `{age: null}` | Carol | |
| `{age: "30"}` | — | |
| `{age: {"==": "30"}}` | Alice | |
| `{age: {"!=": "30"}}` | bob, Carol, dave | |
| `{age: {"!==": 30}}` | bob, Carol, dave | |
| `{age: [30, 25]}` | Alice, bob | |
| `{age: []}` | — | |
| `{age: {gte: 25, lt: 41}}` | Alice, bob | |
| `{age: {lt: [26, 31]}}` | Alice, bob, Carol | (Carol via `null < 26`; a port may exclude it) |
| `{age: {"<": 30}}` | bob, Carol | (same note) |
| `{age: {">=": 30}}` | Alice, dave | |
| `{age: {"!is": 30}}` | bob, Carol, dave | |
| `{age: {"!is": [30, 25]}}` | bob, Carol, dave | Carol, dave |
| `{age: {"!is": [25, 30]}}` | Alice, Carol, dave | Carol, dave |
| `{age: {isNull: true}}` | Carol | |
| `{age: {isNumber: false}}` | Carol | |
| `{age: {regex: /^3/}}` | Alice | |
| `{age: {like: "3"}}` | throws | — (string operators on non-strings are false) |
| `{age: {isnocase: 30}}` | throws (on Carol's `null`) | Alice |
| `{name: {like: "o"}}` | bob, Carol | |
| `{name: {likenocase: "A"}}` | Alice, Carol, dave | |
| `{name: {left: "A"}}` | Alice | |
| `{name: {leftnocase: "a"}}` | Alice | |
| `{name: {right: "ob"}}` | bob | |
| `{name: {rightnocase: "OL"}}` | Carol | |
| `{name: {right: "xxbob"}}` | — | |
| `{name: {left: ""}}` | Alice, bob, Carol, dave | |
| `{name: {isnocase: "ALICE"}}` | Alice | |
| `{name: {regex: /^[a-c]/i}}` | Alice, bob, Carol | |
| `{name: /^A/}` | Alice, bob, Carol, dave | (port: reject, or treat as regex → Alice) |
| `{name: {length: 3}}` | bob | |
| `{age: {IS: 30}}` / `{age: {foo: 30}}` | — | error: unknown operator |
| `{tags: {"!is": "x"}}` | Alice, bob | |
| `{tags: {isUndefined: true}}` | — | |
| `{tags: {has: "a"}}` | Alice | |
| `{tags: {has: ["a","c"]}}` | Alice, bob | |
| `{tags: {hasAll: ["a","b"]}}` | Alice | |
| `{tags: {hasAll: ["a","c"]}}` | — | |
| `{tags: {hasall: ["a","c"]}}` | Alice, bob | |
| `{tags: {contains: "c"}}` | bob | |
| `{tags: {isSameArray: ["a","b"]}}` | — | Alice |
| `{tags: {isSameArray: false}}` | Alice, bob | (port: invalid operand) |
| `{addr: {city: "Reno"}}` | bob | |
| `{addr: {district: "D1"}}` | — | Alice |
| `{addr: {has: "district"}}` | Alice | |
| `{addr: {has: {city: "Albany"}}}` | — | Alice |
| `{addr: {isSameObject: {city: "Reno"}}}` | — | bob |
| `[{state: "NV"}, {age: 41}]` | bob, dave | |
| `{state: "NY"}, {age: 41}` (two args) | dave | |
| `db({state: "NY"}).filter({age: {gt: 35}})` | dave | |
| `function () { return this.age > 26; }` | Alice, dave | |
| `"abc"` (non-ID string) | Alice, bob, Carol, dave | (port: reject) |
| `"street"` (ID-like string) | — | (port: reject) |

Other results on fixture A:

| Expression | JS result | Intended |
|---|---|---|
| `select("name","age")` | `[[30,"Alice"],[25,"bob"],[null,"Carol"],[41,"dave"]]` | `[["Alice",30],…]` |
| `distinct("state")` | `["NY","NV","MA"]` | |
| `distinct("state","name")` | `[["Alice","NY"],["bob","NV"],["Carol","MA"],["dave","NY"]]` | `[["NY","Alice"],…]` |
| `sum("age","id")` | `106` | |
| `sum("name")` | `"0AlicebobCaroldave"` | error or 0 |
| `min("age")` / `max("age")` | `41` / `41` | `25` / `41` (skip nulls) |
| `db({id: 99}).first()` | `false` | not found |
| `db({id: 99}).min("age")` | `null` | not found |

### Fixture B (ordering)

Values of `n`, in storage order:
`"file10","file2","File1","b","a",10,9,"-5","1.5"`

| Spec | JS result |
|---|---|
| `"n asec"` | `-5, 1.5, File1, a, b, file10, file2, 9, 10` |
| `"n desc"` | `file2, file10, b, a, File1, 10, 9, 1.5, -5` |
| `"n asc"`, `"n DESC"`, `"n  desc"` | unchanged storage order |
| `"n"` (logical) | engine-dependent on mixed data; `File1, file2, file10` are always in natural order relative to each other |

Single-kind logical vectors (deterministic):

| Input (ascending) | `logical` | `logicaldesc` |
|---|---|---|
| `"x2","x10","X3","x2a","x2.5","","y"` | `"","x2","x2a","x2.5","X3","x10","y"` | `"y","x10","X3","x2.5","x2a","x2",""` |
| `"file10","File1","file2"` | `File1, file2, file10` | `file10, file2, File1` |

### Fixture C (pagination), `i` = 1..6

See the table in §8.

### Fixture D (join)

```js
city  = [{name:'New York',state:'NY'},{name:'Las Vegas',state:'NV'},{name:'Boston',state:'MA'}]
state = [{name:'New York',abbreviation:'NY'},{name:'Nevada',abbreviation:'NV'},
         {name:'Massachusetts',abbreviation:'MA'}]
city().join(state, ['state','abbreviation'])
```

Result (system fields omitted):

```json
[
  {"name":"New York","state":"NY","right_name":"New York","abbreviation":"NY"},
  {"name":"Las Vegas","state":"NV","right_name":"Nevada","abbreviation":"NV"},
  {"name":"Boston","state":"MA","right_name":"Massachusetts","abbreviation":"MA"}
]
```

Joining with no conditions gives 9 rows (cartesian). `city({state:"NY"})` as the
left side gives 1 row.
