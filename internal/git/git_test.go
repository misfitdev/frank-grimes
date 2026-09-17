package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Git is not reachable through the CLI yet, and a worktree's lifecycle is not
// something a fake can stand in for: these run the real git against a real
// repository in a temporary directory.

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := run(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

// repo returns a repository with one commit in it.
func repo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "--quiet", "--initial-branch=main")
	git(t, dir, "config", "user.email", "test@example.invalid")
	git(t, dir, "config", "user.name", "Test")
	write(t, filepath.Join(dir, "app.sh"), "rm -rf ./build/*\n")
	git(t, dir, "add", "app.sh")
	git(t, dir, "commit", "--quiet", "--message", "first")

	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRefusesADirectoryThatIsNotARepository(t *testing.T) {
	_, err := Open(context.Background(), t.TempDir())

	if !errors.Is(err, ErrNotRepository) {
		t.Fatalf("want ErrNotRepository, got %v", err)
	}
}

func TestACleanTreeIsCleanAndADirtyOneIsNot(t *testing.T) {
	r := repo(t)
	if err := r.Clean(context.Background()); err != nil {
		t.Fatalf("a freshly committed tree is not clean: %v", err)
	}

	write(t, filepath.Join(r.Dir, "app.sh"), "rm -rf /\n")

	err := r.Clean(context.Background())
	if !errors.Is(err, ErrNotClean) {
		t.Fatalf("want ErrNotClean, got %v", err)
	}
	if !strings.Contains(err.Error(), "app.sh") {
		t.Errorf("the refusal does not say what is uncommitted: %v", err)
	}
}

// An untracked file is the case that would otherwise be read as the fixer's
// work at the end of the batch.
func TestAnUntrackedFileLeavesTheTreeDirty(t *testing.T) {
	r := repo(t)
	write(t, filepath.Join(r.Dir, "notes.txt"), "mine\n")

	if err := r.Clean(context.Background()); !errors.Is(err, ErrNotClean) {
		t.Fatalf("want ErrNotClean, got %v", err)
	}
}

func TestAWorktreeIsIsolatedFromTheTreeItCameFrom(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	head, err := r.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}

	w, err := r.AddWorktree(ctx, filepath.Join(t.TempDir(), "fix"), "grimes/fix-1", head)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(w.Dir, "app.sh"), "rm -rf ./build\n")

	if err := r.Clean(ctx); err != nil {
		t.Errorf("editing the worktree dirtied the tree it came from: %v", err)
	}
	original, err := os.ReadFile(filepath.Join(r.Dir, "app.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if string(original) != "rm -rf ./build/*\n" {
		t.Errorf("the original file changed; got %q", original)
	}
}

func TestChangedNamesEveryPathTheBatchTouched(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	head, _ := r.Head(ctx)
	w, err := r.AddWorktree(ctx, filepath.Join(t.TempDir(), "fix"), "grimes/fix-2", head)
	if err != nil {
		t.Fatal(err)
	}

	write(t, filepath.Join(w.Dir, "app.sh"), "rm -rf ./build\n")
	write(t, filepath.Join(w.Dir, "new.sh"), "echo hello\n")

	changed, err := w.Changed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(changed, " ")
	for _, want := range []string{"app.sh", "new.sh"} {
		if !strings.Contains(got, want) {
			t.Errorf("Changed does not report %q; got %q", want, got)
		}
	}
}

// A rename is two paths. Reporting only the destination would let a fix move a
// file out of the reviewed scope and be judged on where it landed.
func TestChangedReportsBothEndsOfARename(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	head, _ := r.Head(ctx)
	w, err := r.AddWorktree(ctx, filepath.Join(t.TempDir(), "fix"), "grimes/fix-3", head)
	if err != nil {
		t.Fatal(err)
	}

	git(t, w.Dir, "mv", "app.sh", "elsewhere.sh")

	changed, err := w.Changed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(changed, " ")
	for _, want := range []string{"app.sh", "elsewhere.sh"} {
		if !strings.Contains(got, want) {
			t.Errorf("Changed does not report %q; got %q", want, got)
		}
	}
}

func TestCommitRecordsTheBatchAndLeavesTheOriginalBranchAlone(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	head, _ := r.Head(ctx)
	w, err := r.AddWorktree(ctx, filepath.Join(t.TempDir(), "fix"), "grimes/fix-4", head)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(w.Dir, "app.sh"), "rm -rf ./build\n")

	sha, err := w.Commit(ctx, "fix: one deletion path\n\nCloses FG-SEC-0001")
	if err != nil {
		t.Fatal(err)
	}

	if len(sha) != 40 {
		t.Errorf("commit returned %q, want a full object name", sha)
	}
	if sha == head {
		t.Error("the commit is the one the worktree started from")
	}
	if now, _ := r.Head(ctx); now != head {
		t.Errorf("the original branch moved: %s -> %s", head, now)
	}
	if left, err := w.Changed(ctx); err != nil || len(left) != 0 {
		t.Errorf("the worktree still has %v uncommitted (%v)", left, err)
	}
}

// A later run opens what an earlier one left, and the branch it reports has to
// be the one that is actually checked out there: the run that made it had a
// different identity, and a name derived from this run's would name nothing.
func TestOpeningAWorktreeReadsTheBranchItIsOn(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	head, _ := r.Head(ctx)
	dir := filepath.Join(t.TempDir(), "fix")
	if _, err := r.AddWorktree(ctx, dir, "grimes/fix-earlier", head); err != nil {
		t.Fatal(err)
	}

	again, err := OpenWorktree(ctx, r, dir)
	if err != nil {
		t.Fatal(err)
	}

	if again.Branch != "grimes/fix-earlier" {
		t.Errorf("branch = %q, want the one the worktree is on", again.Branch)
	}
}

// The fixing role may write this directory, including the .git file that says
// which repository it belongs to.
func TestAWorktreeBelongingToAnotherRepositoryIsRefused(t *testing.T) {
	ctx := context.Background()
	mine, theirs := repo(t), repo(t)
	head, _ := theirs.Head(ctx)
	dir := filepath.Join(t.TempDir(), "fix")
	if _, err := theirs.AddWorktree(ctx, dir, "grimes/fix-theirs", head); err != nil {
		t.Fatal(err)
	}

	_, err := OpenWorktree(ctx, mine, dir)

	if !errors.Is(err, ErrForeignWorktree) {
		t.Fatalf("err = %v, want ErrForeignWorktree", err)
	}
}

// Repointed after it was opened, which is the only window a role has.
func TestACommitIntoARepointedWorktreeIsRefused(t *testing.T) {
	ctx := context.Background()
	mine, theirs := repo(t), repo(t)
	head, _ := mine.Head(ctx)
	dir := filepath.Join(t.TempDir(), "fix")
	w, err := mine.AddWorktree(ctx, dir, "grimes/fix-mine", head)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(w.Dir, "app.sh"), "rm -rf ./build\n")
	// What a role with write access to its own worktree can do to it.
	write(t, filepath.Join(w.Dir, ".git"), "gitdir: "+filepath.Join(theirs.Dir, ".git")+"\n")

	_, err = w.Commit(ctx, "not in this repository")

	if !errors.Is(err, ErrForeignWorktree) {
		t.Fatalf("err = %v, want ErrForeignWorktree", err)
	}
}

// What Changed reports decides which findings the batch is credited with and
// whether it stayed in scope, so it is asked of the repository this worktree
// belonged to when it was opened.
func TestChangedFromARepointedWorktreeIsRefused(t *testing.T) {
	ctx := context.Background()
	mine, theirs := repo(t), repo(t)
	head, _ := mine.Head(ctx)
	w, err := mine.AddWorktree(ctx, filepath.Join(t.TempDir(), "fix"), "grimes/fix-6", head)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(w.Dir, ".git"), "gitdir: "+filepath.Join(theirs.Dir, ".git")+"\n")

	_, err = w.Changed(ctx)

	if !errors.Is(err, ErrForeignWorktree) {
		t.Fatalf("err = %v, want ErrForeignWorktree", err)
	}
}

// A git hook exports these, and grimes is run from one. Inherited, they would
// decide which repository every command here reached, whatever directory it was
// given.
func TestTheEnvironmentCannotRedirectWhichRepositoryIsUsed(t *testing.T) {
	ctx := context.Background()
	mine, theirs := repo(t), repo(t)
	// A second commit, so the two repositories do not hash alike: identical
	// trees committed in the same second are the same object name.
	write(t, filepath.Join(theirs.Dir, "theirs.txt"), "not mine\n")
	git(t, theirs.Dir, "add", "-A")
	git(t, theirs.Dir, "commit", "--quiet", "--message", "second")
	mineHead, err := mine.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Uncommitted work in the other repository, which is what a redirected
	// clean check would report.
	write(t, filepath.Join(theirs.Dir, "app.sh"), "someone else's uncommitted work\n")

	t.Setenv("GIT_DIR", filepath.Join(theirs.Dir, ".git"))
	t.Setenv("GIT_WORK_TREE", theirs.Dir)

	if err := mine.Clean(ctx); err != nil {
		t.Errorf("a clean repository reported as dirty through the environment: %v", err)
	}
	head, err := mine.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if head != mineHead {
		t.Errorf("HEAD came from the repository the environment named: %s", head)
	}
}

// Configuration carried in the environment is configuration for whatever git
// runs next. git exports it to subprocesses whenever the operator passed -c to
// an outer command, and what it can change here is which edits status admits
// to: this list is what the batch is credited with and what its scope is judged
// on, so a change it does not mention is a change nothing accounted for.
func TestTheEnvironmentCannotDecideWhichEditsAreSeen(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	head, _ := r.Head(ctx)
	w, err := r.AddWorktree(ctx, filepath.Join(t.TempDir(), "fix"), "grimes/fix-7", head)
	if err != nil {
		t.Fatal(err)
	}
	// A mode change and nothing else, which core.fileMode decides the
	// visibility of.
	if err := os.Chmod(filepath.Join(w.Dir, "app.sh"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.fileMode")
	t.Setenv("GIT_CONFIG_VALUE_0", "false")

	changed, err := w.Changed(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(changed, " "); !strings.Contains(got, "app.sh") {
		t.Errorf("an edit the batch made went unreported; got %q", got)
	}
}

func TestRemoveTakesTheWorktreeAndItsBranch(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	head, _ := r.Head(ctx)
	dir := filepath.Join(t.TempDir(), "fix")
	w, err := r.AddWorktree(ctx, dir, "grimes/fix-5", head)
	if err != nil {
		t.Fatal(err)
	}

	if err := w.Remove(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("the worktree directory is still there")
	}
	if out := git(t, r.Dir, "branch", "--list", "grimes/fix-5"); strings.TrimSpace(out) != "" {
		t.Errorf("the branch is still there: %q", out)
	}
}
