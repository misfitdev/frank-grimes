// Package confine bounds what a spawned role can do to the review's own
// artifacts.
//
// The policy protects the review, not the host. A role is a coding agent the
// operator chose to run with their own credentials; hiding their keys from it
// is neither achievable nor the point. What is achievable is keeping a context
// from editing the artifact it was asked to judge, rewriting a copy another
// role is about to read, or reading the record it is being graded against.
//
// Nothing here names a provider. The policy is derived from the run, so any
// command works under it unchanged.
package confine

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Policy is what one role may do inside the reviewed repository.
//
// Everything outside Root is left alone: a role keeps its own state under HOME
// and TMPDIR, and enumerating the paths some particular CLI needs is exactly
// the per-provider knowledge this package refuses to hold.
type Policy struct {
	// Root is the reviewed repository. Nothing under it is writable except
	// WriteDirs.
	Root string
	// ReadPaths are the files under Root/.grimes this role was handed. The rest
	// of that directory is unreadable: the ledger, the result, the loop state,
	// and the copies staged for other roles.
	ReadPaths []string
	// ReadDirs are directories under Root/.grimes this role was handed whole.
	// A role reads the target through its own files, so a copy of it is a tree
	// rather than a list: naming each file would mean knowing, before the role
	// runs, which ones it decides to open.
	ReadDirs []string
	// WriteDirs are the directories this role may write, empty when it should
	// write nothing. Directories rather than files because a record is written
	// through a temporary file and a rename, which a single-file mount refuses.
	//
	// A fixing role gets two: the one it records what it did in, and the
	// worktree holding the bytes it was authorized to change. Everything else
	// under Root stays read-only, including the target the worktree came from.
	WriteDirs []string
}

// GrimesDir is the review's own directory inside the target, named here
// because the policy is expressed in terms of it.
const GrimesDir = ".grimes"

// resolve returns the policy with every path made absolute and symlink-free.
//
// A mechanism describes a path to the kernel, which has already resolved the
// one the role actually opened. On macOS a temporary directory reaches the
// engine as /var/folders/... and the kernel as /private/var/folders/..., so a
// rule written from the unresolved path matches nothing and the run proceeds
// unconfined without any error to notice.
func (p Policy) resolve() (Policy, error) {
	root, err := resolvePath(p.Root, "root")
	if err != nil {
		return Policy{}, err
	}
	out := Policy{Root: root}
	for _, r := range p.ReadPaths {
		got, err := resolvePath(r, "readable path")
		if err != nil {
			return Policy{}, err
		}
		out.ReadPaths = append(out.ReadPaths, got)
	}
	for _, r := range p.ReadDirs {
		got, err := resolvePath(r, "readable directory")
		if err != nil {
			return Policy{}, err
		}
		out.ReadDirs = append(out.ReadDirs, got)
	}
	for _, w := range p.WriteDirs {
		got, err := resolvePath(w, "writable directory")
		if err != nil {
			return Policy{}, err
		}
		out.WriteDirs = append(out.WriteDirs, got)
	}
	return out, nil
}

func resolvePath(path, what string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("confine: %s %q is not absolute", what, path)
	}
	// A path that does not exist cannot be resolved, and a rule naming it would
	// be silently inert, so it is an error rather than a pass-through.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("confine: resolving %s %q: %w", what, path, err)
	}
	return resolved, nil
}

// grimesDir is Root's review directory, which every backend has to carve back
// out of whatever it granted.
func (p Policy) grimesDir() string { return filepath.Join(p.Root, GrimesDir) }

// inGrimes reports whether a granted directory sits inside the review's own,
// which is the only case a backend has to re-grant after denying it.
func (p Policy) inGrimes(dir string) bool {
	rel, err := filepath.Rel(p.grimesDir(), dir)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// writableRoot reports whether the whole repository was granted, which is what
// a fixing role gets and nothing else does.
func (p Policy) writableRoot() bool {
	for _, w := range p.WriteDirs {
		if w == p.Root {
			return true
		}
	}
	return false
}

// Mechanism turns the argv a role would have run into the argv that runs it
// under a policy.
type Mechanism interface {
	// Wrap returns the command to spawn instead of argv.
	Wrap(p Policy, argv []string) ([]string, error)
	// Name identifies the mechanism in a run record and in an error.
	Name() string
}

// Unsafe runs a role with no confinement at all. It exists so an operator can
// say so explicitly and have the run record carry it, not as a fallback the
// engine may select on its own.
type Unsafe struct{}

func (Unsafe) Wrap(_ Policy, argv []string) ([]string, error) { return argv, nil }
func (Unsafe) Name() string                                   { return "unsafe" }

// External is an operator-supplied wrapper: srt, firejail, a container runner.
//
// The engine cannot read such a wrapper's policy, so it does not try. It runs
// the probe against it instead, which asks the only question that matters:
// whether anything is actually blocked.
type External struct{ Command []string }

func (e External) Wrap(_ Policy, argv []string) ([]string, error) {
	if len(e.Command) == 0 {
		return nil, fmt.Errorf("confine: no wrapper command")
	}
	return append(append([]string{}, e.Command...), argv...), nil
}

func (e External) Name() string { return e.Command[0] }

// Default is the built-in mechanism for this platform, or an error where there
// is none. An absent mechanism is refused rather than downgraded: a run that
// quietly stopped confining would report the same result as one that did.
func Default() (Mechanism, error) { return platformDefault() }
