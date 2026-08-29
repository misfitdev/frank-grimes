// Package contracts implements the identity, codec, and lifecycle rules that
// proto/frank_grimes/v2/contracts.proto declares.
package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

// NUL separates fingerprint components so that no component's content can
// impersonate a boundary between two others.
const fieldSep = "\x00"

// NormalizeEvidence renders evidence text comparable across runs: UTF-8, LF
// line endings, no trailing whitespace, no leading or trailing blank lines.
// Line numbers are deliberately excluded by the caller, so that moving code
// without changing it keeps the same identity.
func NormalizeEvidence(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}

	start, end := 0, len(lines)
	for start < end && lines[start] == "" {
		start++
	}
	for end > start && lines[end-1] == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

// NormalizePath renders a repository-relative path comparable: forward slashes,
// no leading "./", no trailing slash.
func NormalizePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// Fingerprint is the content address of a finding: the same defect in the same
// place with the same evidence yields the same bytes on every run and every
// machine. Line numbers are not an input, so a finding survives the code above
// it moving.
func Fingerprint(category pb.Category, repoPath, evidence string) []byte {
	h := sha256.New()
	h.Write([]byte(CategoryName(category)))
	h.Write([]byte(fieldSep))
	h.Write([]byte(NormalizePath(repoPath)))
	h.Write([]byte(fieldSep))
	h.Write([]byte(NormalizeEvidence(evidence)))
	return h.Sum(nil)
}

// FindingID renders a fingerprint as the stable, human-referenceable ID.
// Width is 12 hex by default; a caller that has observed a collision between
// two distinct fingerprints extends both to 16.
func FindingID(category pb.Category, fingerprint []byte, wide bool) string {
	width := 12
	if wide {
		width = 16
	}
	return fmt.Sprintf("FG-%s-%s", CategoryName(category), hex.EncodeToString(fingerprint)[:width])
}

// CategoryName maps the enum to the three-letter code used in IDs and reports.
func CategoryName(c pb.Category) string {
	name := c.String() // CATEGORY_SEC
	return strings.TrimPrefix(name, "CATEGORY_")
}

// ParseCategory accepts the three-letter code used in reports.
func ParseCategory(code string) (pb.Category, error) {
	v, ok := pb.Category_value["CATEGORY_"+strings.ToUpper(code)]
	if !ok || v == 0 {
		return pb.Category_CATEGORY_UNSPECIFIED, fmt.Errorf("unknown category %q", code)
	}
	return pb.Category(v), nil
}
