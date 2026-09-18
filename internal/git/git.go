// Package git runs the git commands a fix supervisor needs and nothing else.
//
// It is the only place in the repository that shells out to git. History is
// read only to decide which files a target covers: a review is still about the
// bytes in front of it, and a range selects the files, never an older version
// of them.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// ErrForeignWorktree reports a worktree that belongs to some other repository
// than the one about to write to it.
var ErrForeignWorktree = errors.New("the worktree belongs to another repository")

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
func (r *Repo) Clean(ctx context.Context, ignore ...string) error {
	args := []string{"status", "--porcelain", "--"}
	for _, path := range ignore {
		args = append(args, ":(exclude)"+path)
	}
	out, err := r.run(ctx, args...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("%w:\n%s\n\n%s", ErrNotClean, strings.TrimSpace(out), howToClean())
	}
	return nil
}

// howToClean names the ways out of a dirty tree, including the one an operator
// is most likely to reach for and least likely to be told about.
//
// A fix run reviews the commit its worktree was made from, so uncommitted work
// means the bytes in front of the operator are not the bytes under review. That
// is worth refusing, but the refusal is only useful if it says what to do:
// tooling that writes into the repository is not the operator's work and not
// something the run should be asked to judge.
func howToClean() string {
	var b strings.Builder
	b.WriteString("Commit or stash the work, or, for files that belong to tooling rather " +
		"than to the review, add them to .gitignore or .git/info/exclude.")
	// Only where someone tried it. Said always, it is noise; said here, it is
	// the answer to why an exclusion that was set appeared to be ignored.
	if configuredInEnvironment() {
		b.WriteString("\n\nGit configuration in the environment (GIT_CONFIG_*) is not used: " +
			"it can decide which edits git admits to, and a run that let it would " +
			"report a batch by what it was told to see. An exclude file named that " +
			"way has no effect here; .git/info/exclude does.")
	}
	return b.String()
}

func configuredInEnvironment() bool {
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "GIT_CONFIG_") {
			return true
		}
	}
	return false
}

// Worktree is an isolated checkout a fix batch is applied in.
type Worktree struct {
	Dir    string
	Branch string

	repo *Repo
	// common is the repository this worktree was resolved to belong to, kept
	// from the moment it was opened. A fixing role may write this directory,
	// including the .git file that says where its repository is, so what the
	// engine commits to is the repository it checked, not whatever the path
	// says by the time it commits.
	common string
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
	return r.openWorktree(ctx, abs)
}

// OpenWorktree returns a worktree that is already there, from a run that made
// it earlier.
//
// Its branch is read rather than assumed: a later run has its own identity, and
// a name derived from that would not be the name of the branch the last one
// left checked out here.
func OpenWorktree(ctx context.Context, r *Repo, dir string) (*Worktree, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return r.openWorktree(ctx, abs)
}

// openWorktree asks git what is at dir and refuses anything that is not this
// repository's worktree.
func (r *Repo) openWorktree(ctx context.Context, dir string) (*Worktree, error) {
	// EvalSymlinks first: a path that points somewhere else would otherwise be
	// checked here and used there.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotRepository, dir)
	}
	common, err := run(ctx, resolved, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotRepository, resolved)
	}
	common = strings.TrimSpace(common)
	mine, err := r.run(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	if !sameDir(common, strings.TrimSpace(mine)) {
		return nil, fmt.Errorf("%w: %s belongs to %s", ErrForeignWorktree, resolved, common)
	}
	branch, err := run(ctx, resolved, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, err
	}
	return &Worktree{
		Dir:    resolved,
		Branch: strings.TrimSpace(branch),
		repo:   r,
		common: common,
	}, nil
}

// sameDir compares two directory paths through their links, so a worktree
// reached by one name is not read as a different repository from the same one
// reached by another.
func sameDir(a, b string) bool {
	if a == b {
		return true
	}
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false
	}
	return ra == rb
}

// check confirms the worktree still belongs to the repository it did when it
// was opened. Called before the operations that write.
func (w *Worktree) check(ctx context.Context) error {
	common, err := run(ctx, w.Dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNotRepository, w.Dir)
	}
	if !sameDir(strings.TrimSpace(common), w.common) {
		return fmt.Errorf("%w: %s now points at %s", ErrForeignWorktree, w.Dir, strings.TrimSpace(common))
	}
	return nil
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
	// What this returns decides which findings the batch is credited with and
	// whether it stayed in scope, so it is asked of the repository this
	// worktree belonged to when it was opened.
	if err := w.check(ctx); err != nil {
		return nil, err
	}
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
	// The batch has been running in here since this worktree was opened, and a
	// commit is the one thing that leaves it.
	if err := w.check(ctx); err != nil {
		return "", err
	}
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
	cmd.Env = append(withoutRepoSelection(cmd.Environ()), "GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0")

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

// repoSelection names the variables that tell git which repository to act on,
// whatever directory it was started in.
//
// Anything running inside a git hook has these exported, and grimes is run from
// one. Left inherited, the directory this package is so careful about would not
// be the repository the command reached.
//
// The GIT_CONFIG_ family goes with them, by prefix. git exports it whenever the
// operator passed -c to an outer command, and it decides which edits status
// admits to: core.fileMode=false hides a mode change outright. That list is
// what the batch is credited with and what its scope is judged on.
var repoSelection = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_COMMON_DIR",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_NAMESPACE",
	"GIT_CEILING_DIRECTORIES",
	"GIT_DISCOVERY_ACROSS_FILESYSTEM",
}

func withoutRepoSelection(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if slices.Contains(repoSelection, name) || strings.HasPrefix(name, "GIT_CONFIG_") {
			continue
		}
		out = append(out, kv)
	}
	return out
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
