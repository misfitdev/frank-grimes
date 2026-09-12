// Package envelope carries the GrimesResult result block through provider
// transports that only pass text.
package envelope

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	Begin = "GRIMES_RESULT_PROTOBUF_V2_BEGIN"
	End   = "GRIMES_RESULT_PROTOBUF_V2_END"
)

// Contains reports whether s carries anything that looks like an envelope.
func Contains(s string) bool {
	return strings.Contains(s, Begin)
}

// Wrap renders canonical result bytes as a text-safe block.
func Wrap(b []byte) string {
	return fmt.Sprintf("%s\n%s\n%s\n", Begin, base64.StdEncoding.EncodeToString(b), End)
}

// Extract takes the last complete envelope. An assistant message may contain
// earlier partial or quoted blocks, and may also trail a truncated BEGIN after
// a complete block, so the scan walks back over BEGIN markers until one of them
// has a matching END rather than trusting the final marker.
func Extract(s string) ([]byte, error) {
	search := s
	seen := false
	for {
		begin := strings.LastIndex(search, Begin)
		if begin < 0 {
			if seen {
				return nil, fmt.Errorf("envelope opened but never closed")
			}
			return nil, fmt.Errorf("no %s marker found", Begin)
		}
		seen = true
		rest := search[begin+len(Begin):]
		if end := strings.Index(rest, End); end >= 0 {
			payload := strings.Join(strings.Fields(rest[:end]), "")
			return base64.StdEncoding.DecodeString(payload)
		}
		search = search[:begin]
	}
}
