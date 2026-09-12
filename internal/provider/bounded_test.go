package provider

import (
	"strings"
	"testing"
)

func TestBoundedBufferCountsOnlyDroppedBytes(t *testing.T) {
	for _, c := range []struct {
		name    string
		limit   int
		writes  []int
		keep    int
		dropped int
	}{
		{"straddles the limit in one write", 10, []int{15}, 10, 5},
		{"fits entirely", 10, []int{4}, 4, 0},
		{"exactly at the limit", 10, []int{10}, 10, 0},
		{"fills then overflows", 10, []int{6, 9}, 10, 5},
		{"already full", 10, []int{10, 7}, 10, 7},
		{"many small writes", 5, []int{2, 2, 2, 2}, 5, 3},
	} {
		b := &boundedBuffer{limit: c.limit}
		total := 0
		for _, n := range c.writes {
			w, err := b.Write([]byte(strings.Repeat("x", n)))
			if err != nil {
				t.Fatalf("%s: Write: %v", c.name, err)
			}
			if w != n {
				t.Errorf("%s: Write returned %d, want %d; a short write stops the pipe", c.name, w, n)
			}
			total += n
		}
		if b.buf.Len() != c.keep {
			t.Errorf("%s: retained %d bytes, want %d", c.name, b.buf.Len(), c.keep)
		}
		if b.dropped != c.dropped {
			t.Errorf("%s: reported %d dropped, want %d", c.name, b.dropped, c.dropped)
		}
		if b.buf.Len()+b.dropped != total {
			t.Errorf("%s: retained %d + dropped %d != %d written",
				c.name, b.buf.Len(), b.dropped, total)
		}
	}
}
