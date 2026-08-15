package store

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// splitParagraphs must never emit an empty or untrimmed paragraph, whatever the
// input body looks like. (rapid: pgregory.net/rapid — the canonical Go
// property-testing library; the request's "pgavlin/rapid" is this import path.)
func TestSplitParagraphsInvariants(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Draw(t, "body")
		for _, p := range splitParagraphs(s) {
			if p == "" {
				t.Fatalf("empty paragraph from %q", s)
			}
			if p != strings.TrimSpace(p) {
				t.Fatalf("untrimmed paragraph %q from %q", p, s)
			}
		}
	})
}

// Round-trip: clean paragraphs (non-empty, single-line, trimmed) joined with a
// blank line split back to themselves.
func TestSplitParagraphsRoundTrip(t *testing.T) {
	clean := rapid.StringMatching(`[a-zA-Z0-9 ]*[a-zA-Z0-9][a-zA-Z0-9 ]*`).
		Filter(func(s string) bool { return strings.TrimSpace(s) == s })
	rapid.Check(t, func(t *rapid.T) {
		paras := rapid.SliceOfN(clean, 0, 6).Draw(t, "paras")
		got := splitParagraphs(strings.Join(paras, "\n\n"))
		if len(got) != len(paras) {
			t.Fatalf("len %d != %d for %#v", len(got), len(paras), paras)
		}
		for i := range paras {
			if got[i] != paras[i] {
				t.Fatalf("para %d: %q != %q", i, got[i], paras[i])
			}
		}
	})
}

// Native Go fuzzing: the same no-empty/no-untrimmed invariant, no panics.
func FuzzSplitParagraphs(f *testing.F) {
	f.Add("one\n\ntwo")
	f.Add("")
	f.Add("  \n\n  \n\n x ")
	f.Fuzz(func(t *testing.T, s string) {
		for _, p := range splitParagraphs(s) {
			if p == "" || p != strings.TrimSpace(p) {
				t.Fatalf("bad paragraph %q from %q", p, s)
			}
		}
	})
}
