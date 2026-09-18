//go:build darwin

package confine

import (
	"fmt"
	"os/exec"
	"strings"
)

// seatbelt applies the policy through sandbox-exec.
//
// sandbox-exec has carried a deprecation notice since around 2016 with no
// removal, and it remains the only documented way to apply a policy to an
// arbitrary process on this platform: App Sandbox needs a signed bundle and an
// entitlement, and Endpoint Security authorizes rather than confines. The probe
// in probe.go is what turns that standing risk into a detectable one.
type seatbelt struct{}

func platformDefault() (Mechanism, error) {
	if _, err := exec.LookPath(sandboxExec); err != nil {
		return nil, fmt.Errorf("confine: %s not available: %w", sandboxExec, err)
	}
	return seatbelt{}, nil
}

const sandboxExec = "/usr/bin/sandbox-exec"

func (seatbelt) Name() string { return "sandbox-exec" }

func (seatbelt) Wrap(p Policy, argv []string) ([]string, error) {
	p, err := p.resolve()
	if err != nil {
		return nil, err
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("confine: nothing to run")
	}
	return append([]string{sandboxExec, "-p", profile(p), "--"}, argv...), nil
}

// profile denies by exception rather than by enumeration. A later rule wins in
// Seatbelt, so each deny is followed by the literals it carves back out.
//
// Reads outside the review's own directory stay allowed: the role has to read
// the target, and listing what else it may read would mean knowing which CLI it
// is.
func profile(p Policy) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n")

	fmt.Fprintf(&b, "(deny file-write* (subpath %s))\n", quote(p.Root))
	for _, w := range p.WriteDirs {
		fmt.Fprintf(&b, "(allow file-write* (subpath %s))\n", quote(w))
	}

	// After the grants, and again for reads below. A fixing role is handed the
	// whole repository, and the review's own directory is inside it: granted
	// first and taken back here, since the last rule to match is the one that
	// applies.
	fmt.Fprintf(&b, "(deny file-write* (subpath %s))\n", quote(p.grimesDir()))
	fmt.Fprintf(&b, "(deny file-read* (subpath %s))\n", quote(p.grimesDir()))
	for _, r := range p.ReadPaths {
		fmt.Fprintf(&b, "(allow file-read* (literal %s))\n", quote(r))
	}
	// A subpath rather than a literal: a literal names one file, and what is
	// granted here is a tree the role reads its own way through.
	for _, r := range p.ReadDirs {
		fmt.Fprintf(&b, "(allow file-read* (subpath %s))\n", quote(r))
	}
	// Only what the role is meant to keep inside that directory: its own work.
	// Everything else it may write is outside, where reads are allowed anyway.
	for _, w := range p.WriteDirs {
		if !p.inGrimes(w) {
			continue
		}
		fmt.Fprintf(&b, "(allow file-write* (subpath %s))\n", quote(w))
		fmt.Fprintf(&b, "(allow file-read* (subpath %s))\n", quote(w))
	}
	return b.String()
}

// quote renders a path as a Seatbelt string literal. The profile is passed as
// one argument rather than through a file, so an unescaped quote or backslash
// in a path would end the literal early and change which rule was written.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}
