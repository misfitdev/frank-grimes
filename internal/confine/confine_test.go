package confine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These are package tests rather than CLI tests because what is under test is
// whether a kernel facility actually denied something. That is not observable
// through the engine's output, which reports only that a role ran.

const (
	ledgerSecret = "SECRET-LEDGER-BYTES"
	handedText   = "handed to this role"
	targetText   = "reviewable source"
)

// target lays out a repository the way a run does: a target file, a .grimes
// directory holding one artifact this role was handed and one it was not, and
// an output directory for what it produces.
func target(t *testing.T) (root, handed, ledger, out string) {
	t.Helper()
	root = t.TempDir()
	grimes := filepath.Join(root, GrimesDir)
	out = filepath.Join(grimes, "run-1")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	handed = filepath.Join(grimes, "inventory.textproto")
	ledger = filepath.Join(grimes, "ledger.textproto")
	write(t, filepath.Join(root, "source.go"), targetText)
	write(t, handed, handedText)
	write(t, ledger, ledgerSecret)
	return root, handed, ledger, out
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func builtin(t *testing.T) Mechanism {
	t.Helper()
	m, err := Default()
	if err != nil {
		t.Skipf("no built-in mechanism on this platform: %v", err)
	}
	return m
}

// run spawns a shell under the mechanism and returns what it printed. The exit
// code is deliberately ignored: a denial surfaces differently depending on
// which call the shell was making when it was refused.
func run(t *testing.T, m Mechanism, p Policy, script string) string {
	t.Helper()
	argv, err := m.Wrap(p, []string{"/bin/sh", "-c", script})
	if err != nil {
		t.Fatalf("wrapping: %v", err)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = p.Root
	out, _ := cmd.Output()
	return string(out)
}

func TestAConfinedRoleReadsTheTargetAndWhatItWasHanded(t *testing.T) {
	m := builtin(t)
	root, handed, _, out := target(t)

	got := run(t, m, Policy{Root: root, ReadPaths: []string{handed}, WriteDirs: []string{out}},
		"cat "+filepath.Join(root, "source.go")+" "+handed+" 2>&1")

	for _, want := range []string{targetText, handedText} {
		if !strings.Contains(got, want) {
			t.Errorf("a confined role could not read %q; got %q", want, got)
		}
	}
}

func TestAConfinedRoleCannotReadTheLedger(t *testing.T) {
	m := builtin(t)
	root, handed, ledger, out := target(t)

	got := run(t, m, Policy{Root: root, ReadPaths: []string{handed}, WriteDirs: []string{out}},
		"cat "+ledger+" 2>/dev/null; exit 0")

	if strings.Contains(got, ledgerSecret) {
		t.Errorf("a confined role read the ledger it was not handed; got %q", got)
	}
}

func TestAConfinedRoleCannotRewriteTheTarget(t *testing.T) {
	m := builtin(t)
	root, handed, _, out := target(t)
	source := filepath.Join(root, "source.go")

	run(t, m, Policy{Root: root, ReadPaths: []string{handed}, WriteDirs: []string{out}},
		"echo tampered >"+source+" 2>/dev/null; exit 0")

	body, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != targetText {
		t.Errorf("a confined role rewrote the target: %q", body)
	}
}

func TestAConfinedRoleWritesAndReadsBackItsOwnDirectory(t *testing.T) {
	m := builtin(t)
	root, handed, _, out := target(t)
	// WriteAtomic renames a temporary file into place, so the role needs the
	// directory rather than one named file.
	script := "echo finding >" + filepath.Join(out, "report.tmp") +
		" && mv " + filepath.Join(out, "report.tmp") + " " + filepath.Join(out, "report") +
		" && cat " + filepath.Join(out, "report")

	got := run(t, m, Policy{Root: root, ReadPaths: []string{handed}, WriteDirs: []string{out}}, script+" 2>&1")

	if !strings.Contains(got, "finding") {
		t.Errorf("a confined role could not stage its own output; got %q", got)
	}
}

// A role is a coding agent the operator chose to run with their own
// credentials. Confining it to the review means leaving the rest of the
// machine alone, and a policy that broke HOME or TMPDIR would break every
// provider CLI without protecting anything this package claims to protect.
func TestConfinementLeavesTheRestOfTheMachineAlone(t *testing.T) {
	m := builtin(t)
	root, handed, _, out := target(t)
	elsewhere := filepath.Join(t.TempDir(), "state")

	got := run(t, m, Policy{Root: root, ReadPaths: []string{handed}, WriteDirs: []string{out}},
		"echo kept >"+elsewhere+" 2>&1; cat "+elsewhere+" 2>&1")

	if !strings.Contains(got, "kept") {
		t.Errorf("a confined role could not use its own state outside the target; got %q", got)
	}
}

func TestTheProbeAcceptsTheBuiltInMechanism(t *testing.T) {
	m := builtin(t)
	root, _, _, _ := target(t)

	if err := Verify(context.Background(), m, root); err != nil {
		t.Fatalf("the built-in mechanism failed its own probe: %v", err)
	}
}

// The fixing policy hands over the repository and keeps the review's own
// directory. Proving the first half matters as much as the second: a mechanism
// that denied the worktree would refuse every fix without saying so.
func TestTheFixingProbeAcceptsTheBuiltInMechanism(t *testing.T) {
	m := builtin(t)
	root, _, _, _ := target(t)

	if err := VerifyFixing(context.Background(), m, root); err != nil {
		t.Fatalf("the built-in mechanism failed the fixing probe: %v", err)
	}
}

// A fixing role may edit the repository and still may not touch the ledger.
func TestAFixingRoleWritesTheWorktreeAndNotTheLedger(t *testing.T) {
	m := builtin(t)
	root, handed, ledger, out := target(t)
	source := filepath.Join(root, "source.go")

	run(t, m, Policy{Root: root, ReadPaths: []string{handed}, WriteDirs: []string{out, root}},
		"echo fixed >"+source+" 2>/dev/null; echo tampered >"+ledger+" 2>/dev/null; exit 0")

	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(after)) != "fixed" {
		t.Errorf("a fixing role could not edit the target it was handed; got %q", after)
	}
	kept, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(kept)) != ledgerSecret {
		t.Errorf("a fixing role rewrote the ledger; got %q", kept)
	}
}

func TestTheProbeRefusesAWrapperThatConfinesNothing(t *testing.T) {
	root, _, _, _ := target(t)

	err := Verify(context.Background(), External{Command: []string{"/usr/bin/env"}}, root)

	if !errors.Is(err, ErrNotConfined) {
		t.Fatalf("err = %v, want ErrNotConfined", err)
	}
	for _, want := range []string{"read a file", "wrote inside the target"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
}

// The fixing probe grants the repository, so the only thing left for it to
// prove is the part that still holds. A mechanism that lets that through is
// one a fixing role could rewrite the ledger under.
func TestTheFixingProbeRefusesAWrapperThatConfinesNothing(t *testing.T) {
	root, _, _, _ := target(t)

	err := VerifyFixing(context.Background(), External{Command: []string{"/usr/bin/env"}}, root)

	if !errors.Is(err, ErrNotConfined) {
		t.Fatalf("err = %v, want ErrNotConfined", err)
	}
	if !strings.Contains(err.Error(), "review's own directory") {
		t.Errorf("the refusal does not name the write it caught: %v", err)
	}
}

func TestTheProbeRefusesAWrapperThatCannotRun(t *testing.T) {
	root, _, _, _ := target(t)

	err := Verify(context.Background(), External{Command: []string{"grimes-no-such-wrapper"}}, root)

	if err == nil || errors.Is(err, ErrNotConfined) {
		t.Fatalf("err = %v, want a startup failure distinct from ErrNotConfined", err)
	}
}

// Unsafe is a recorded choice, not a mechanism, so the probe has nothing to
// check and must not refuse the run it was asked to allow.
func TestTheProbeAcceptsAnExplicitlyUnconfinedRun(t *testing.T) {
	root, _, _, _ := target(t)

	if err := Verify(context.Background(), Unsafe{}, root); err != nil {
		t.Fatalf("Verify(Unsafe) = %v, want nil", err)
	}
}

// A wrapper that exits without reaching what it was handed leaves neither
// forbidden effect behind, which is indistinguishable from a wrapper that
// stopped both unless the canary is made to say it ran.
func TestTheProbeRefusesAWrapperThatNeverRanTheCanary(t *testing.T) {
	root, _, _, _ := target(t)
	wrapper := filepath.Join(t.TempDir(), "wrapper.sh")
	write(t, wrapper, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(wrapper, 0o755); err != nil {
		t.Fatal(err)
	}

	err := Verify(context.Background(), External{Command: []string{wrapper}}, root)

	if err == nil {
		t.Fatal("Verify accepted a wrapper that never ran the probe")
	}
	if !strings.Contains(err.Error(), "did not run the probe") {
		t.Errorf("the refusal does not say the probe never ran: %v", err)
	}
}

// The probe writes into the directory it is testing, so it names that file
// after its own staging directory. A fixed name would collide with whatever the
// repository already holds: the probe would read someone else's file as its own
// result, refuse the mechanism, and then delete the file.
func TestTheProbeLeavesTheReviewDirectoryAsItFoundIt(t *testing.T) {
	m := builtin(t)
	root, _, _, _ := target(t)
	bystander := filepath.Join(root, "grimes-probe-written")
	write(t, bystander, "a file this repository already had")

	if err := Verify(context.Background(), m, root); err != nil {
		t.Fatalf("the probe refused over a file it did not write: %v", err)
	}

	got, err := os.ReadFile(bystander)
	if err != nil {
		t.Fatalf("the probe removed a file it did not write: %v", err)
	}
	if string(got) != "a file this repository already had" {
		t.Errorf("the probe rewrote a file it did not write; got %q", got)
	}
}

// A relative path that happens to resolve would produce a rule the kernel
// never matches, so it is refused for being relative rather than for being
// unresolvable.
func TestAPolicyPathThatIsNotAbsoluteIsRefused(t *testing.T) {
	m := builtin(t)
	root, _, _, _ := target(t)
	t.Chdir(root)

	if _, err := m.Wrap(Policy{Root: "."}, []string{"/bin/true"}); err == nil {
		t.Fatal("a relative root was accepted")
	}
}

// A rule naming a path that does not exist is silently inert under every
// mechanism here, which would confine nothing while appearing to succeed.
func TestAPolicyPathThatDoesNotExistIsRefused(t *testing.T) {
	m := builtin(t)
	root, _, _, out := target(t)

	_, err := m.Wrap(Policy{
		Root:      root,
		ReadPaths: []string{filepath.Join(root, GrimesDir, "never-staged")},
		WriteDirs: []string{out},
	}, []string{"/bin/true"})

	if err == nil {
		t.Fatal("a readable path that was never staged was accepted")
	}
}
