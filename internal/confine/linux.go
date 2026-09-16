//go:build linux

package confine

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// bubblewrap applies the policy through bwrap.
//
// The whole filesystem is bound through so the role keeps its own state and
// tooling, then the repository is rebound read-only and the review's directory
// is replaced by a tmpfs. What the role was handed is bound back in
// individually, so the ledger and the other roles' copies are not merely
// unreadable but absent.
type bubblewrap struct{}

func platformDefault() (Mechanism, error) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		return nil, fmt.Errorf("confine: bwrap not available, install bubblewrap: %w", err)
	}
	return bubblewrap{}, nil
}

func (bubblewrap) Name() string { return "bwrap" }

func (bubblewrap) Wrap(p Policy, argv []string) ([]string, error) {
	p, err := p.resolve()
	if err != nil {
		return nil, err
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("confine: nothing to run")
	}

	out := []string{
		"bwrap",
		// Without a user namespace bwrap needs to be setuid, which is not how
		// it is packaged on most distributions. Try rather than require: where
		// it is setuid this is unnecessary and harmless.
		"--unshare-user-try",
		"--dev-bind", "/", "/",
		"--ro-bind", p.Root, p.Root,
		"--tmpfs", filepath.Join(p.Root, GrimesDir),
	}
	for _, r := range p.ReadPaths {
		out = append(out, "--ro-bind", r, r)
	}
	if p.WriteDir != "" {
		out = append(out, "--bind", p.WriteDir, p.WriteDir)
	}
	// A provider command can begin with something that looks like a bwrap flag.
	return append(append(out, "--"), argv...), nil
}
