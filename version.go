// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"github.com/maloquacious/semver"
)

var (
	version = semver.Version{
		Major:      0,
		Minor:      1,
		Patch:      0,
		PreRelease: "alpha",
		Build:      semver.Commit(),
	}
)

// Version returns the current version of this package.
func Version() semver.Version {
	return version
}
