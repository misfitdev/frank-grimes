// Package envelope carries the GrimesResult result block through provider
// transports that only pass text.
package envelope

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Result markers wrap what the engine emits; report markers wrap what a
// provider hands back. Two names for two directions, so a payload sent the
// wrong way fails on the marker rather than deep inside validation.
const (
	Begin = "GRIMES_RESULT_PROTOBUF_V2_BEGIN"
	End   = "GRIMES_RESULT_PROTOBUF_V2_END"

	ReportBegin = "GRIMES_REPORT_PROTOBUF_V2_BEGIN"
	ReportEnd   = "GRIMES_REPORT_PROTOBUF_V2_END"
)

// Contains reports whether s carries anything that looks like a result envelope.
func Contains(s string) bool { return strings.Contains(s, Begin) }

// ContainsReport reports whether s carries anything that looks like a report
// envelope.
func ContainsReport(s string) bool { return strings.Contains(s, ReportBegin) }

// Wrap renders canonical result bytes as a text-safe block.
func Wrap(b []byte) string { return wrap(Begin, End, b) }

// WrapReport renders canonical report bytes as a text-safe block.
func WrapReport(b []byte) string { return wrap(ReportBegin, ReportEnd, b) }

func wrap(begin, end string, b []byte) string {
	return fmt.Sprintf("%s\n%s\n%s\n", begin, base64.StdEncoding.EncodeToString(b), end)
}

// Extract takes the last complete envelope. An assistant message may contain
// earlier partial or quoted blocks, and may also trail a truncated BEGIN after
// a complete block, so the scan walks back over BEGIN markers until one of them
// has a matching END rather than trusting the final marker.
func Extract(s string) ([]byte, error) { return extract(Begin, End, s) }

// ExtractReport takes the last complete report envelope.
func ExtractReport(s string) ([]byte, error) { return extract(ReportBegin, ReportEnd, s) }

func extract(begin, end, s string) ([]byte, error) {
	search := s
	seen := false
	for {
		at := strings.LastIndex(search, begin)
		if at < 0 {
			if seen {
				return nil, fmt.Errorf("envelope opened but never closed")
			}
			return nil, fmt.Errorf("no %s marker found", begin)
		}
		seen = true
		rest := search[at+len(begin):]
		if close := strings.Index(rest, end); close >= 0 {
			payload := strings.Join(strings.Fields(rest[:close]), "")
			return base64.StdEncoding.DecodeString(payload)
		}
		search = search[:at]
	}
}
