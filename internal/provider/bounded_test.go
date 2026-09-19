package provider

import (
	"strings"
	"testing"
)

// The tail is what is kept, so a case is named by which bytes survive rather
// than by how many. Every write is distinct, so retaining the wrong end reads
// as the wrong characters rather than as the right count.
func TestBoundedBufferKeepsTheTail(t *testing.T) {
	for _, c := range []struct {
		name   string
		limit  int
		writes []string
		want   string
	}{
		{"fits entirely", 10, []string{"abcd"}, "abcd"},
		{"exactly at the limit", 4, []string{"abcd"}, "abcd"},
		{"straddles the limit in one write", 4, []string{"abcdefg"}, "defg"},
		{"fills then overflows", 4, []string{"ab", "cdef"}, "cdef"},
		{"already full", 3, []string{"abc", "defg"}, "efg"},
		{"many small writes", 5, []string{"ab", "cd", "ef", "gh"}, "defgh"},
		{"far past the trim threshold", 2, []string{strings.Repeat("x", 50) + "yz"}, "yz"},
	} {
		b := &boundedBuffer{limit: c.limit}
		total := 0
		for _, w := range c.writes {
			n, err := b.Write([]byte(w))
			if err != nil {
				t.Fatalf("%s: Write: %v", c.name, err)
			}
			if n != len(w) {
				t.Errorf("%s: Write returned %d, want %d; a short write stops the pipe", c.name, n, len(w))
			}
			total += len(w)
		}
		if got := string(b.bytes()); got != c.want {
			t.Errorf("%s: retained %q, want %q", c.name, got, c.want)
		}
		if len(b.bytes())+b.dropped() != total {
			t.Errorf("%s: retained %d + dropped %d != %d written",
				c.name, len(b.bytes()), b.dropped(), total)
		}
	}
}
