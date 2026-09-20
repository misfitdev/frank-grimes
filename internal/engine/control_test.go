package engine

import (
	"strings"
	"testing"
)

// The contract caps the reason, and a witness is a line of a target read with
// no promise about its encoding. %q turns one invalid byte into four
// characters, so a bound applied before quoting is not a bound.
func TestQuotedWitnessFitsTheReasonCap(t *testing.T) {
	for _, c := range []struct {
		name    string
		witness string
	}{
		{"short and printable", "return nil"},
		{"long and printable", strings.Repeat("x", 4000)},
		{"invalid utf-8", strings.Repeat("\xff", 4000)},
		{"escapes", strings.Repeat("\t\n\\\"", 1000)},
		{"empty", ""},
	} {
		got := quotedWitness(c.witness)
		// The template around it is ~126 characters and the contract caps the
		// whole reason at 400.
		if len(got) > 220 {
			t.Errorf("%s: quoted to %d characters, which does not leave room for the reason", c.name, len(got))
		}
	}
}
