//go:build linux

package confine

import (
	"fmt"
	"os/exec"
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
	}
	// Mounts apply in order, so the repository is bound before the review's own
	// directory is replaced: a fixing role that was handed the whole repository
	// still finds nothing where the ledger is.
	if p.writableRoot() {
		out = append(out, "--bind", p.Root, p.Root)
	} else {
		out = append(out, "--ro-bind", p.Root, p.Root)
	}
	for _, w := range p.WriteDirs {
		if w == p.Root || p.inGrimes(w) {
			continue
		}
		out = append(out, "--bind", w, w)
	}
	out = append(out, "--tmpfs", p.grimesDir())
	for _, r := range p.ReadPaths {
		out = append(out, "--ro-bind", r, r)
	}
	// Last, over the tmpfs: what the role keeps inside the review's directory.
	for _, w := range p.WriteDirs {
		if p.inGrimes(w) {
			out = append(out, "--bind", w, w)
		}
	}
	// A provider command can begin with something that looks like a bwrap flag.
	return append(append(out, "--"), argv...), nil
}
