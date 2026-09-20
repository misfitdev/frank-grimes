package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/confine"
	"github.com/misfitdev/frank-grimes/internal/engine"
)

func script(t *testing.T, body string) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "provider.sh")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return []string{path}
}

func primaryReq() engine.Request {
	return engine.Request{
		Role:       engine.RolePrimary,
		Target:     &pb.Target{Root: "/repo", Scope: "src", FingerprintSha256: make([]byte, 32), Kind: pb.TargetKind_TARGET_KIND_CODE},
		Mode:       pb.Mode_MODE_REPORT,
		Iteration:  1,
		Categories: []pb.Category{pb.Category_CATEGORY_SEC, pb.Category_CATEGORY_COR},
		Research:   "offline",
	}
}

func TestExecReturnsStdout(t *testing.T) {
	e := &Exec{Confine: confine.Unsafe{}, Command: script(t, "echo hello\n")}
	out, err := e.Review(context.Background(), primaryReq())
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if strings.TrimSpace(string(out.Raw)) != "hello" {
		t.Errorf("got %q, want %q", out.Raw, "hello")
	}
}

func TestExecNonZeroExitFailsClosed(t *testing.T) {
	e := &Exec{Confine: confine.Unsafe{}, Command: script(t, "echo partial\nexit 7\n")}
	out, err := e.Review(context.Background(), primaryReq())
	if err == nil {
		t.Fatal("want an error for a non-zero provider exit")
	}
	if out != nil {
		t.Error("output returned alongside a provider failure")
	}
	if !errors.Is(err, engine.ErrProviderFailed) {
		t.Errorf("error = %v, want ErrProviderFailed", err)
	}
}

// A role that talks past the bound is still reviewing. The bytes over it are
// not the answer -- the report comes back as a sealed file -- so the pass
// survives and keeps its tail. Unit-level because the boundary is the retained
// slice, which the CLI only shows once truncated into a log.
func TestExecKeepsTheTailAndDoesNotFailOnVolume(t *testing.T) {
	e := &Exec{
		Confine:        confine.Unsafe{},
		Command:        script(t, "head -c 20000 /dev/zero | tr '\\0' 'a'\nprintf 'THE-END'\n"),
		MaxOutputBytes: 4096,
	}
	out, err := e.Review(context.Background(), primaryReq())
	if err != nil {
		t.Fatalf("a verbose role was failed for volume: %v", err)
	}
	if len(out.Raw) != 4096 {
		t.Errorf("retained %d bytes, want the bound of 4096", len(out.Raw))
	}
	// The end of the stream, not the start: the head is where a role was still
	// setting up.
	if !strings.HasSuffix(string(out.Raw), "THE-END") {
		t.Errorf("retained the wrong end; it ends %q", tailOf(string(out.Raw), 20))
	}
}

// A role that never stops is ended by the timeout, which is the only thing that
// can tell it apart from one still working.
func TestExecUnboundedOutputEndsAtTheTimeout(t *testing.T) {
	e := &Exec{
		Confine:        confine.Unsafe{},
		Command:        script(t, "yes 0123456789abcdef\n"),
		MaxOutputBytes: 4096,
		Timeout:        2 * time.Second,
	}
	start := time.Now()
	_, err := e.Review(context.Background(), primaryReq())
	if !errors.Is(err, engine.ErrProviderFailed) {
		t.Errorf("error = %v, want ErrProviderFailed", err)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Errorf("an unbounded provider ran %v past its timeout", elapsed)
	}
}

func tailOf(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func TestExecOutputExactlyAtBoundSucceeds(t *testing.T) {
	e := &Exec{
		Confine:        confine.Unsafe{},
		Command:        script(t, "printf '%0.sx' {1..64}\n"),
		MaxOutputBytes: 64,
	}
	out, err := e.Review(context.Background(), primaryReq())
	if err != nil {
		t.Fatalf("output exactly at the bound was rejected: %v", err)
	}
	if len(out.Raw) != 64 {
		t.Errorf("got %d bytes, want 64", len(out.Raw))
	}
}

func TestExecTimeout(t *testing.T) {
	e := &Exec{Confine: confine.Unsafe{}, Command: script(t, "sleep 60\n"), Timeout: 500 * time.Millisecond}
	start := time.Now()
	_, err := e.Review(context.Background(), primaryReq())
	if err == nil {
		t.Fatal("want an error when the provider outruns its timeout")
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Errorf("timeout took %v to fire", elapsed)
	}
}

// A shell provider that backgrounds work keeps the stdout pipe open; killing
// only the direct child would leave the read blocked past the deadline.
func TestExecTimeoutKillsProcessGroup(t *testing.T) {
	// The descendant touches a file before sleeping, so the test can prove it
	// really started rather than infer it from the parent's own output.
	marker := filepath.Join(t.TempDir(), "descendant-started")
	e := &Exec{
		Confine: confine.Unsafe{},
		Command: script(t, "( : >\""+marker+"\"; sleep 60 ) &\necho started\nwait\n"),
		Timeout: 500 * time.Millisecond,
	}

	done := make(chan error, 1)
	go func() {
		_, err := e.Review(context.Background(), primaryReq())
		done <- err
	}()

	// Bounded well under WaitDelay: without the group kill, Wait only returns
	// once that delay expires, which is the slow path this guards against.
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want an error when the provider outruns its timeout")
		}
		if !errors.Is(err, engine.ErrProviderFailed) {
			t.Errorf("error = %v, want ErrProviderFailed", err)
		}
		if !strings.Contains(err.Error(), "deadline exceeded") {
			t.Errorf("error = %v, want it to name the deadline", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Review blocked on a descendant holding the pipe open")
	}

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the descendant never started, so termination was not exercised: %v", err)
	}
}

func TestExecContextCancel(t *testing.T) {
	e := &Exec{Confine: confine.Unsafe{}, Command: script(t, "sleep 60\n")}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	if _, err := e.Review(ctx, primaryReq()); err == nil {
		t.Fatal("want an error when the run is cancelled")
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Errorf("cancellation took %v to take effect", elapsed)
	}
}

func TestExecMissingCommand(t *testing.T) {
	if _, err := (&Exec{Confine: confine.Unsafe{}}).Review(context.Background(), primaryReq()); err == nil {
		t.Fatal("want an error when no provider command is configured")
	}
}

func TestExecPrimaryRequestEnv(t *testing.T) {
	e := &Exec{Confine: confine.Unsafe{}, Command: script(t, "env\n")}
	out, err := e.Review(context.Background(), primaryReq())
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	env := string(out.Raw)
	for _, want := range []string{
		"GRIMES_ROLE=primary",
		"GRIMES_TARGET_ROOT=/repo",
		"GRIMES_TARGET_SCOPE=src",
		"GRIMES_MODE=report",
		"GRIMES_ITERATION=1",
		"GRIMES_CATEGORIES=SEC,COR",
		"GRIMES_RESEARCH=offline",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("environment missing %q", want)
		}
	}
}

// The adjudicator receives the target's identity and nothing about the review.
// The claimed verdict is the sharpest leak of the set: a reviewer shown the
// conclusion it was asked to reach independently is anchored before it starts.
func TestExecAdjudicatorIsNotToldTheVerdict(t *testing.T) {
	e := &Exec{Confine: confine.Unsafe{}, Command: script(t, "env\n")}
	req := primaryReq()
	req.Role = engine.RoleAdjudicator
	req.Claimed = &pb.Verdict{
		Decision:           pb.Decision_DECISION_PASS,
		ResidualRisk:       pb.ResidualRisk_RESIDUAL_RISK_LOW,
		ReviewConfidence:   pb.ReviewConfidence_REVIEW_CONFIDENCE_HIGH,
		ReviewCompleteness: pb.ReviewCompleteness_REVIEW_COMPLETENESS_SUFFICIENT,
	}
	out, err := e.Review(context.Background(), req)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	env := string(out.Raw)
	// It still has to know which role it is playing, and which target.
	for _, want := range []string{"GRIMES_ROLE=adjudicator", "GRIMES_TARGET_FINGERPRINT="} {
		if !strings.Contains(env, want) {
			t.Errorf("environment missing %q", want)
		}
	}
	for _, leak := range []string{
		"GRIMES_CLAIMED",
		"GRIMES_FINDING", "GRIMES_EVIDENCE", "GRIMES_LEDGER",
		"GRIMES_SEVERITY", "GRIMES_CATEGORIES", "GRIMES_ITERATION",
	} {
		if strings.Contains(env, leak) {
			t.Errorf("adjudicator environment leaks %q", leak)
		}
	}
}

// A role may write its own work directory, so it may plant a link there. The
// engine writes the log unconfined, so an open that followed one would write
// wherever the role pointed it.
func TestRecordDoesNotFollowAPlantedLink(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, ".grimes", "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(work, "primary.log")); err != nil {
		t.Fatal(err)
	}

	(&Exec{Dir: root}).record(primaryReq(), []string{"provider"}, []byte("x"), "", nil, 0, 0)

	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "untouched" {
		t.Errorf("the log followed a planted link and wrote %q", got)
	}
}
