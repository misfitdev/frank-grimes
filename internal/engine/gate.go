package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/proc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DefaultGateTimeout bounds one gate run. A gate is the repository's own aggregate
// check, which is allowed to be slow; one that has not finished in this long is
// not going to answer this iteration.
const DefaultGateTimeout = 30 * time.Minute

// gateOutputLimit bounds what is retained from a gate. Only the digest reaches
// the record, but the digest has to be over something bounded.
const gateOutputLimit = 4 << 20

// ExecGate runs one command over a whole fix batch and records what it did.
//
// The gate is not a role and is not confined: it is the repository's own check,
// selected by a rule the record names, and it has to be able to build. What
// bounds it is the worktree it runs in, which is where the batch already is.
type ExecGate struct {
	// Command is run through a shell, because a repository's check is written
	// for one: "just check", "make test && make lint".
	Command  string
	Selected pb.GateSelection
	Timeout  time.Duration
}

// Run executes the gate in cwd and reports the outcome whichever way it exits.
//
// A gate that failed is evidence, not an error: the batch is unverified and the
// run says so. An error here is reserved for a gate that could not be run at
// all, which is a different fact and reaches the record as unavailable.
func (g ExecGate) Run(ctx context.Context, cwd string) (*pb.Verification, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if g.Command == "" {
		return unavailableGate(), nil
	}

	timeout := g.Timeout
	if timeout <= 0 {
		timeout = DefaultGateTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", g.Command)
	cmd.Dir = cwd
	// A gate is given a repository, not a conversation.
	cmd.Stdin = nil
	var out bytes.Buffer
	sink := &boundedWriter{to: &out, left: gateOutputLimit}
	cmd.Stdout = sink
	cmd.Stderr = sink
	proc.SetGroup(cmd)
	cmd.Cancel = func() error { return proc.KillGroup(cmd) }

	runErr := cmd.Run()
	digest := sha256.Sum256(out.Bytes())

	// A gate killed by its own deadline never finished, so it did not fail: it
	// was unavailable, and recording a verdict from it would read a timeout as
	// a judgement of the batch.
	if ctx.Err() != nil {
		return unavailableGate(), nil
	}
	// A shell that could not start the command is not the command failing.
	var notStarted *exec.Error
	if errors.As(runErr, &notStarted) {
		return unavailableGate(), nil
	}

	status := pb.VerificationStatus_VERIFICATION_STATUS_PASSED
	if runErr != nil {
		status = pb.VerificationStatus_VERIFICATION_STATUS_FAILED
	}
	v := &pb.Verification{
		Status:       status,
		Command:      g.Command,
		Cwd:          &pb.RepoPath{Value: "."},
		ExitCode:     int32(exitCode(runErr)),
		OutputSha256: digest[:],
		CompletedAt:  timestamppb.Now(),
		SelectedBy:   g.Selected,
	}
	return v, nil
}

func unavailableGate() *pb.Verification {
	return &pb.Verification{
		Status:     pb.VerificationStatus_VERIFICATION_STATUS_UNAVAILABLE,
		SelectedBy: pb.GateSelection_GATE_SELECTION_UNAVAILABLE,
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}

// SelectGate chooses the gate once, in the order the skill sets out, and says
// which rule chose it.
//
// Rule 3, a command documented in the repository, is resolved by whoever read
// that documentation and reaches this as a supplied command. Prose is not a
// command, and an engine guessing at one would be running something nobody
// chose, out of a file the target controls.
func SelectGate(supplied, root string) ExecGate {
	if strings.TrimSpace(supplied) != "" {
		return ExecGate{Command: supplied, Selected: pb.GateSelection_GATE_SELECTION_USER_SUPPLIED}
	}
	if cmd := repositoryCheck(root); cmd != "" {
		return ExecGate{Command: cmd, Selected: pb.GateSelection_GATE_SELECTION_REPOSITORY_CHECK}
	}
	return ExecGate{Selected: pb.GateSelection_GATE_SELECTION_UNAVAILABLE}
}

// runners are the task runners a checked-in aggregate check is looked for in,
// in the order they are preferred.
//
// Only the runner's own name is ever executed, never a line read out of the
// file: the file is inside the target, and the Untrusted Target Rule covers
// what a target says as much as what it does. What the file decides is whether
// a `check` target exists, not what runs.
var runners = []struct {
	file    string
	command string
	recipe  *regexp.Regexp
}{
	{"justfile", "just check", regexp.MustCompile(`(?m)^check[^:=]*:`)},
	{"Justfile", "just check", regexp.MustCompile(`(?m)^check[^:=]*:`)},
	{"Makefile", "make check", regexp.MustCompile(`(?m)^check[^:=]*:`)},
	{"makefile", "make check", regexp.MustCompile(`(?m)^check[^:=]*:`)},
}

func repositoryCheck(root string) string {
	for _, r := range runners {
		body, err := os.ReadFile(filepath.Join(root, r.file))
		if err != nil {
			continue
		}
		if !r.recipe.Match(body) {
			continue
		}
		if _, err := exec.LookPath(strings.Fields(r.command)[0]); err != nil {
			continue
		}
		return r.command
	}
	return ""
}

// boundedWriter keeps the first of what it is given and counts the rest away.
type boundedWriter struct {
	to   *bytes.Buffer
	left int
}

func (b *boundedWriter) Write(p []byte) (int, error) {
	if b.left <= 0 {
		return len(p), nil
	}
	if len(p) > b.left {
		b.to.Write(p[:b.left])
		b.left = 0
		return len(p), nil
	}
	b.to.Write(p)
	b.left -= len(p)
	return len(p), nil
}

var _ = fmt.Sprintf
