package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

// The gate's outcome is visible through the CLI and asserted there. What is not
// reachable from outside is what it does with a command that never returns, a
// command that floods its output, or a repository it has to choose a gate from
// without one being supplied.

func TestAGateThatPassesIsRecordedWithItsEvidence(t *testing.T) {
	g := ExecGate{Command: "echo ok", Selected: pb.GateSelection_GATE_SELECTION_USER_SUPPLIED}

	v, err := g.Run(context.Background(), t.TempDir())

	if err != nil {
		t.Fatal(err)
	}
	if v.GetStatus() != pb.VerificationStatus_VERIFICATION_STATUS_PASSED {
		t.Errorf("status is %v", v.GetStatus())
	}
	if v.GetCommand() != "echo ok" || v.GetExitCode() != 0 {
		t.Errorf("the record does not name what ran: %q exit %d", v.GetCommand(), v.GetExitCode())
	}
	if len(v.GetOutputSha256()) != 32 || v.GetCompletedAt() == nil {
		t.Error("a passed gate must carry an output digest and a timestamp")
	}
	if v.GetSelectedBy() != pb.GateSelection_GATE_SELECTION_USER_SUPPLIED {
		t.Errorf("selection is %v", v.GetSelectedBy())
	}
}

// A gate that failed is the batch's verdict, not the engine's error.
func TestAGateThatFailsIsAnOutcomeRatherThanAnError(t *testing.T) {
	g := ExecGate{Command: "exit 3", Selected: pb.GateSelection_GATE_SELECTION_REPOSITORY_CHECK}

	v, err := g.Run(context.Background(), t.TempDir())

	if err != nil {
		t.Fatalf("a failing gate returned an error: %v", err)
	}
	if v.GetStatus() != pb.VerificationStatus_VERIFICATION_STATUS_FAILED {
		t.Errorf("status is %v", v.GetStatus())
	}
	if v.GetExitCode() != 3 {
		t.Errorf("exit code is %d, want 3", v.GetExitCode())
	}
	if v.GetSelectedBy() != pb.GateSelection_GATE_SELECTION_REPOSITORY_CHECK {
		t.Errorf("a failing gate lost which rule selected it: %v", v.GetSelectedBy())
	}
}

// A timeout is not a judgement. Recording one as failed would read "nobody
// waited long enough" as "the batch is broken".
func TestAGateThatRunsOutOfTimeIsUnavailableRatherThanFailed(t *testing.T) {
	g := ExecGate{Command: "sleep 30", Timeout: 100 * time.Millisecond}

	start := time.Now()
	v, err := g.Run(context.Background(), t.TempDir())

	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("the gate ran for %s past its own deadline", took)
	}
	if v.GetStatus() != pb.VerificationStatus_VERIFICATION_STATUS_UNAVAILABLE {
		t.Errorf("status is %v, want unavailable", v.GetStatus())
	}
}

// A shell that outlives its parent holds the pipe open; the deadline has to
// reach the whole group.
func TestAGateDoesNotLeaveItsChildrenBehind(t *testing.T) {
	g := ExecGate{Command: "sleep 30 & sleep 30", Timeout: 100 * time.Millisecond}

	start := time.Now()
	if _, err := g.Run(context.Background(), t.TempDir()); err != nil {
		t.Fatal(err)
	}

	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("the gate waited %s on a child of the command it killed", took)
	}
}

// The digest is over what was kept, and what is kept is bounded: a gate that
// prints a gigabyte cannot be held in memory to be hashed whole.
func TestAFloodingGateIsDigestedOverWhatTheBoundKept(t *testing.T) {
	flood := gateOutputLimit * 4
	g := ExecGate{Command: fmt.Sprintf("yes hello | head -c %d", flood)}

	v, err := g.Run(context.Background(), t.TempDir())

	if err != nil {
		t.Fatal(err)
	}
	if v.GetStatus() != pb.VerificationStatus_VERIFICATION_STATUS_PASSED {
		t.Errorf("status is %v", v.GetStatus())
	}
	kept := bytes.Repeat([]byte("hello\n"), gateOutputLimit/6+1)[:gateOutputLimit]
	want := sha256.Sum256(kept)
	if !bytes.Equal(v.GetOutputSha256(), want[:]) {
		whole := sha256.Sum256(bytes.Repeat([]byte("hello\n"), flood/6+1)[:flood])
		if bytes.Equal(v.GetOutputSha256(), whole[:]) {
			t.Fatal("the digest is over the whole flood; the output was not bounded")
		}
		t.Fatal("the digest is over neither the bounded output nor the whole of it")
	}
}

func TestAGateThatCannotStartIsUnavailable(t *testing.T) {
	g := ExecGate{Command: "grimes-no-such-command"}

	v, err := g.Run(context.Background(), t.TempDir())

	if err != nil {
		t.Fatal(err)
	}
	// A shell reports a missing command as exit 127, which is a failure of the
	// command rather than of the shell; either reading is defensible, and what
	// matters is that nothing claims the batch was tested.
	if v.GetStatus() == pb.VerificationStatus_VERIFICATION_STATUS_PASSED {
		t.Error("a command that does not exist passed a gate")
	}
}

func TestASuppliedCommandOutranksTheRepositorysOwn(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "justfile"), "check:\n\techo hi\n")

	g := SelectGate("my-own-check", root, true)

	if g.Command != "my-own-check" {
		t.Errorf("selected %q", g.Command)
	}
	if g.Selected != pb.GateSelection_GATE_SELECTION_USER_SUPPLIED {
		t.Errorf("selection is %v", g.Selected)
	}
}

func TestACheckedInCheckIsFoundWhenNothingWasSupplied(t *testing.T) {
	if _, err := os.Stat("/usr/bin/make"); err != nil {
		t.Skip("make is not installed")
	}
	root := t.TempDir()
	write(t, filepath.Join(root, "Makefile"), "build:\n\ttrue\ncheck:\n\ttrue\n")

	g := SelectGate("", root, true)

	if g.Command != "make check" {
		t.Errorf("selected %q, want make check", g.Command)
	}
	if g.Selected != pb.GateSelection_GATE_SELECTION_REPOSITORY_CHECK {
		t.Errorf("selection is %v", g.Selected)
	}
}

// A runner file without a check recipe selects nothing. Running `make check`
// against a Makefile that has no such target would report the gate as failed
// and read a missing check as a broken batch.
func TestARunnerWithNoCheckRecipeSelectsNothing(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Makefile"), "build:\n\ttrue\n")

	g := SelectGate("", root, true)

	if g.Selected != pb.GateSelection_GATE_SELECTION_UNAVAILABLE {
		t.Errorf("selected %q by %v", g.Command, g.Selected)
	}
}

// The recipe is a command out of a file inside the target, which the skill
// says is inspected before it runs. Nothing here can inspect it, so the
// operator saying they have is what selects it.
func TestACheckedInCheckIsNotRunWithoutAuthorization(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Makefile"), "check:\n\ttrue\n")

	g := SelectGate("", root, false)

	if g.Selected != pb.GateSelection_GATE_SELECTION_UNAVAILABLE || g.Command != "" {
		t.Errorf("an unauthorized repository check was selected: %q by %v", g.Command, g.Selected)
	}
}

// check-ci is not check. Selecting it would run a target the file does not
// have, and a gate that cannot run reads as a batch that failed.
func TestARecipeThatMerelyStartsWithCheckIsNotIt(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Makefile"), "check-ci:\n\ttrue\ncheckout:\n\ttrue\n")

	g := SelectGate("", root, true)

	if g.Selected != pb.GateSelection_GATE_SELECTION_UNAVAILABLE {
		t.Errorf("selected %q by %v", g.Command, g.Selected)
	}
}

func TestARepositoryWithNoCheckLeavesTheGateUnavailable(t *testing.T) {
	g := SelectGate("", t.TempDir(), true)

	if g.Selected != pb.GateSelection_GATE_SELECTION_UNAVAILABLE || g.Command != "" {
		t.Errorf("selected %q by %v", g.Command, g.Selected)
	}
	v, err := g.Run(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if v.GetStatus() != pb.VerificationStatus_VERIFICATION_STATUS_UNAVAILABLE {
		t.Errorf("an unselected gate ran anyway: %v", v.GetStatus())
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
