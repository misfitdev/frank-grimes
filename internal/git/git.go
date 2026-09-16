// Package git runs the git commands a fix supervisor needs and nothing else.
//
// It is the only place in the repository that shells out to git. Nothing here
// reads history: a review is about the bytes in front of it, and a supervisor
// only has to put its edits somewhere isolated and, if they earn it, commit
// them once.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// commandTimeout bounds one git invocation. Every command here is local and
// touches no network; one that has not finished in this long is stuck.
const commandTimeout = 2 * time.Minute

// outputLimit bounds what is retained from a git command, in both directions.
// A status over a large tree is the realistic upper bound.
const outputLimit = 1 << 20

// ErrNotClean reports a working tree with changes in it.
var ErrNotClean = errors.New("the working tree has uncommitted changes")

// ErrNotRepository reports a directory git does not consider a work tree.
var ErrNotRepository = errors.New("not a git repository")

// Repo is a working tree git commands run against.
type Repo struct {
	Dir string
}

// Open confirms dir is a work tree before anything is asked of it, so a caller
// that was pointed at the wrong place fails at the start rather than half way
// through a batch.
func Open(ctx context.Context, dir string) (*Repo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	r := &Repo{Dir: abs}
	out, err := r.run(ctx, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotRepository, abs)
	}
	if strings.TrimSpace(out) != "true" {
		return nil, fmt.Errorf("%w: %s", ErrNotRepository, abs)
	}
	return r, nil
}

// Head is the commit the working tree is on.
func (r *Repo) Head(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Clean reports whether the working tree has nothing uncommitted, tracked or
// not.
//
// Untracked files count. A fix batch is judged by what changed between HEAD and
// the end of the run, and a file that was already lying there unversioned would
// be read as something the fixer produced.
func (r *Repo) Clean(ctx context.Context) error {
	out, err := r.run(ctx, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("%w:\n%s", ErrNotClean, strings.TrimSpace(out))
	}
	return nil
}

// Worktree is an isolated checkout a fix batch is applied in.
type Worktree struct {
	Dir    string
	Branch string

	repo *Repo
}

// AddWorktree checks commit out into dir on a new branch.
//
// The branch is created rather than reused so the supervisor's commits cannot
// land on anything the operator is working on, and dir is outside the
// repository so the working tree it came from is never written to.
func (r *Repo) AddWorktree(ctx context.Context, dir, branch, commit string) (*Worktree, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if _, err := r.run(ctx, "worktree", "add", "--quiet", "-b", branch, abs, commit); err != nil {
		return nil, fmt.Errorf("creating the worktree: %w", err)
	}
	return &Worktree{Dir: abs, Branch: branch, repo: r}, nil
}

// Remove deletes the worktree and the branch it was on.
//
// Only called for a batch nothing is keeping: an unverified edit is kept and
// reported, because a batch nobody could test is still work someone can read.
func (w *Worktree) Remove(ctx context.Context) error {
	if _, err := w.repo.run(ctx, "worktree", "remove", "--force", w.Dir); err != nil {
		return err
	}
	_, err := w.repo.run(ctx, "branch", "-D", w.Branch)
	return err
}

// Changed lists the repository-relative paths the worktree differs from its
// starting commit in, tracked or not, in the order git reports them.
func (w *Worktree) Changed(ctx context.Context) ([]string, error) {
	out, err := w.git(ctx, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	return parseStatus(out), nil
}

// Commit records everything in the worktree under message and returns the
// commit it made.
//
// Author and committer are left to the repository's own configuration: the
// supervisor is running as the operator, and inventing an identity here would
// put a name in history that nobody can be asked about.
func (w *Worktree) Commit(ctx context.Context, message string) (string, error) {
	if _, err := w.git(ctx, "add", "--all"); err != nil {
		return "", fmt.Errorf("staging the batch: %w", err)
	}
	if _, err := w.git(ctx, "commit", "--quiet", "--message", message); err != nil {
		return "", fmt.Errorf("committing the batch: %w", err)
	}
	out, err := w.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// parseStatus reads porcelain v1 -z records.
//
// Each record is XY, a space, then the path, and a rename carries a second
// NUL-separated path which is the name the file had before. Both names are
// reported: a fix that renamed a file out of scope changed two paths, and
// scope is judged on all of them.
func parseStatus(out string) []string {
	var paths []string
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		record := fields[i]
		if len(record) < 4 {
			continue
		}
		code, path := record[:2], record[3:]
		if path != "" {
			paths = append(paths, path)
		}
		// R and C spend the next record on the source name.
		if code[0] == 'R' || code[0] == 'C' {
			if i+1 < len(fields) && fields[i+1] != "" {
				paths = append(paths, fields[i+1])
			}
			i++
		}
	}
	return paths
}

func (r *Repo) run(ctx context.Context, args ...string) (string, error) {
	return run(ctx, r.Dir, args...)
}

func (w *Worktree) git(ctx context.Context, args ...string) (string, error) {
	return run(ctx, w.Dir, args...)
}

// run invokes git in dir.
//
// Unconfined, unlike a role: this is the engine's own command with the engine's
// own arguments, not a context being graded. What it must not do is inherit a
// terminal, since a git that decided to prompt would hold one until the
// timeout.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = nil
	// A pager would never return, and a prompt cannot be answered by anyone.
	cmd.Env = append(cmd.Environ(), "GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limited{w: &stdout, left: outputLimit}
	cmd.Stderr = &limited{w: &stderr, left: outputLimit}

	if err := cmd.Run(); err != nil {
		said := strings.TrimSpace(stderr.String())
		if said == "" {
			said = strings.TrimSpace(stdout.String())
		}
		if said != "" {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, said)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return stdout.String(), nil
}

// limited writes until it has taken its bound and then discards, so a command
// that produces more than expected cannot exhaust memory.
type limited struct {
	w    *bytes.Buffer
	left int
}

func (l *limited) Write(p []byte) (int, error) {
	if l.left <= 0 {
		return len(p), nil
	}
	if len(p) > l.left {
		l.w.Write(p[:l.left])
		l.left = 0
		return len(p), nil
	}
	l.w.Write(p)
	l.left -= len(p)
	return len(p), nil
}
