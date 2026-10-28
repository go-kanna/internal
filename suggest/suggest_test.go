package suggest_test

import (
	"testing"

	"github.com/go-kanna/internal/suggest"
)

func TestDistance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want int
	}{
		{a: "email", b: "email", want: 0},
		{a: "emial", b: "email", want: 1},
		{a: "emall", b: "email", want: 1},
		{a: "emai", b: "email", want: 1},
		{a: "emaill", b: "email", want: 1},
		{a: "", b: "email", want: 5},
		{a: "email", b: "", want: 5},
		{a: "ab", b: "ba", want: 1},
		{a: "required", b: "reqiured", want: 1},
		{a: "max", b: "min", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.a+"/"+tt.b, func(t *testing.T) {
			t.Parallel()

			if got := suggest.Distance(tt.a, tt.b); got != tt.want {
				t.Errorf("Distance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestClosest(t *testing.T) {
	t.Parallel()

	rules := []string{"required", "email", "url", "uuid", "max", "min"}

	tests := []struct {
		name       string
		candidates []string
		want       string
		wantOK     bool
	}{
		{name: "emial", candidates: rules, want: "email", wantOK: true},
		{name: "reqiured", candidates: rules, want: "required", wantOK: true},
		// "required" is two edits away; eight letters allow two.
		{name: "reqired_", candidates: rules, want: "required", wantOK: true},
		{name: "uri", candidates: rules, want: "url", wantOK: true},
		// "uuid" is two edits away, and three letters allow only one.
		{name: "uxd", candidates: rules, wantOK: false},
		// Three edits are too many however long the name is.
		{name: "requiredxy", candidates: rules, want: "required", wantOK: true},
		{name: "requiredxyz", candidates: rules, wantOK: false},
		{name: "xyz", candidates: rules, wantOK: false},
		{name: "phone", candidates: rules, wantOK: false},
		{name: "emial", candidates: nil, wantOK: false},
		// "max" and "min" are both one edit from "mix"; the first listed wins.
		{name: "mix", candidates: rules, want: "max", wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := suggest.Closest(tt.name, tt.candidates)
			if ok != tt.wantOK || ok && got != tt.want {
				t.Errorf("Closest(%q) = %q, %v, want %q, %v", tt.name, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
