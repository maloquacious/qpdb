# CLAUDE.md

Guidance for Claude Code when working in this repository.

## Project

`qpdb` (`github.com/maloquacious/qpdb`) is a Go package that applies a TaffyDB-style query surface to a client's collection of records.
It is not a TaffyDB clone and has no parser, wire format, or HTTP layer.
The specification is [`docs/design.md`](docs/design.md).
[`docs/taffydb/api.md`](docs/taffydb/api.md) documents what `taffy.js` 2.7.3 actually does; it is background only, and `docs/design.md` wins where they disagree.

The project is in alpha.

## Priorities

Fix bugs before adding features.
Before offering to implement a change or work on documentation, check for open bugs (`gh issue list --label bug --state open`).
If any are open, do not make the offer; propose working on the bugs instead.

## Build and test

```sh
go build ./...
go vet ./...
go test ./...
```

All three must pass before committing.

## Versioning

- Semantic versioning.
  The current version lives in `version.go` as a `semver.Version` literal (`Major`, `Minor`, `Patch`, `PreRelease`).
- **Every commit that changes code must bump the version in `version.go`.**
  Documentation-only commits are encouraged to bump it but are not required to.
- Choose the bump by the change: patch for fixes, minor for new features or any breaking change while below v1.0.0.
  Keep `PreRelease: "alpha"` until told otherwise.
- Do not create git tags.
  Versions will be tagged on GitHub once the project leaves alpha.

## Git workflow

- Commit directly to `main`.
  No feature branches or pull requests; history is the recovery mechanism.
  This may change after v1.0.0.
- Claude is authorized to commit and push each task as it is completed, provided all tests pass and the version has been bumped.
- Claude may create GitHub labels as needed.

## CI

There is no GitHub CI, and none should be added until the project is about to release v1.0.0.
Run the checks above locally instead.
