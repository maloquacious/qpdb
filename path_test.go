// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package qpdb

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func TestPathIsID(t *testing.T) {
	tests := []struct {
		name string
		p    Path
		want bool
	}{
		{"IDPath", IDPath, true},
		{"copy of IDPath", slices.Clone(IDPath), true},
		{"literal ___id field", Path{"___id"}, false},
		{"nested ___id field", Path{"x", "___id"}, false},
		{"IDPath extended", append(slices.Clone(IDPath), "x"), false},
		{"nil", nil, false},
		{"empty", Path{}, false},
		{"empty segment", Path{""}, false},
		{"U+FFFD key", Path{"�___id"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.IsID(); got != tt.want {
				t.Errorf("%q.IsID() = %v, want %v", []string(tt.p), got, tt.want)
			}
		})
	}
}

func TestPathValidation(t *testing.T) {
	tests := []struct {
		name    string
		p       Path
		wantErr error
	}{
		{"nil", nil, ErrEmptyPath},
		{"empty", Path{}, ErrEmptyPath},
		{"one segment", Path{"name"}, nil},
		{"two segments", Path{"addr", "city"}, nil},
		{"empty segment", Path{""}, nil},
		{"empty segments", Path{"", ""}, nil},
		{"IDPath", IDPath, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FilterErr(Eq(tt.p, 1))
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Errorf("FilterErr(Eq(%q, 1)) = %v, want %v", []string(tt.p), err, tt.wantErr)
			}
		})
	}
}

func TestPathSegmentsNotSplit(t *testing.T) {
	f := Eq(Path{"a.b"}, 1).(CompareNode)
	if want := (Path{"a.b"}); !slices.Equal(f.Path, want) {
		t.Errorf("Path = %q, want %q", []string(f.Path), []string(want))
	}
}

func TestPathCopied(t *testing.T) {
	p := Path{"addr", "city"}
	f := Eq(p, "NY").(CompareNode)
	p[1] = "zip"
	if f.Path[1] != "city" {
		t.Errorf("filter path changed with caller's slice: %q", []string(f.Path))
	}
	id := Eq(IDPath, "x").(CompareNode)
	if !id.Path.IsID() {
		t.Errorf("stored copy of IDPath is not an ID path")
	}
}

// TestIDPathJSONRoundTrip pins the documented limit: IDPath does not survive
// encoding/json.
func TestIDPathJSONRoundTrip(t *testing.T) {
	b, err := json.Marshal(IDPath)
	if err != nil {
		t.Fatal(err)
	}
	var p Path
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if p.IsID() {
		t.Errorf("IDPath survived a JSON round trip as %q; doc comment is wrong", []string(p))
	}
}
