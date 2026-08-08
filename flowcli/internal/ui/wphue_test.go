package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"flowcli/internal/styles"
)

// wpHue must be deterministic (same WP ID always maps to the same hue) and
// stay within the hue palette, so every view that colours a work package by
// wpHue renders it with the same, stable colour.
func TestWpHueDeterministic(t *testing.T) {
	for _, id := range []string{
		"WP-AUTH", "WP-BOOK", "WP-PAY", "WP-OBS", "WP-UI", "WP-LEGACY",
		"WP-BEER", "WP-DOCS", "T-2010", "T-1042",
	} {
		first := wpHue(id)
		for range 50 {
			require.Equal(t, first, wpHue(id), "wpHue(%q) must be stable", id)
		}
	}
}

// Two different work packages should generally map to different hues so they
// stay visually distinguishable, like the lane view's footnotes.
func TestWpHueVariesAcrossPackages(t *testing.T) {
	seen := map[int]bool{}
	for _, id := range []string{"WP-AUTH", "WP-BOOK", "WP-PAY", "WP-OBS", "WP-UI"} {
		seen[wpHue(id)] = true
	}
	require.GreaterOrEqual(t, len(seen), 2, "expected more than one hue among demo WPs")
}

func TestWpHueInRange(t *testing.T) {
	for _, id := range []string{"WP-AUTH", "WP-BOOK", "no-such-wp", ""} {
		h := wpHue(id)
		require.GreaterOrEqual(t, h, 0)
		require.Less(t, h, len(styles.Hues), "wpHue(%q) out of range: %d", id, h)
	}
}
