// Package provider runs a review through an external process.
//
// Everything the subprocess writes is untrusted. This package's only job is to
// obtain those bytes within a bound and hand them back; it never interprets
// them.
package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/engine"
)

// DefaultMaxOutputBytes bounds a single provider's stdout.
const DefaultMaxOutputBytes = 1 << 20

// DefaultTimeout bounds a single provider invocation.
const DefaultTimeout = 10 * time.Minute

// stderrLimit bounds retained provider diagnostics.
const stderrLimit = 8 << 10

// Exec invokes Command with the request supplied through the environment.
type Exec struct {
	Command        []string
	Dir            string
	Env            []string
	MaxOutputBytes int64
	Timeout        time.Duration
}

// ErrOutputTooLarge reports a provider that wrote past the byte bound.
var ErrOutputTooLarge = engine.ErrOutputTooLarge

// Review runs the subprocess and returns its stdout.
func (e *Exec) Review(ctx context.Context, req engine.Request) (*engine.ProviderOutput, error) {
	if len(e.Command) == 0 {
		return nil, errors.New("no provider command configured")
	}

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	limit := e.MaxOutputBytes
	if limit <= 0 {
		limit = DefaultMaxOutputBytes
	}

	cmd := exec.CommandContext(ctx, e.Command[0], e.Command[1:]...)
	cmd.Dir = e.Dir
	cmd.Env = append(baseEnv(e.Env), requestEnv(req)...)
	setProcessGroup(cmd)
	// CommandContext kills only the direct child; a shell provider leaves its
	// own children holding the pipe open and Wait blocks past the deadline.
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = 5 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// Diagnostics are for the error message only, so keep a bounded tail rather
	// than every byte a provider decides to emit.
	stderr := &boundedBuffer{limit: stderrLimit}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w: %v", engine.ErrProviderFailed, err)
	}

	// Read one byte past the limit so an exactly-at-limit read is not reported
	// as an overrun.
	out, readErr := io.ReadAll(io.LimitReader(stdout, limit+1))
	overrun := int64(len(out)) > limit
	if overrun {
		_ = killGroup(cmd)
	}
	_, _ = io.Copy(io.Discard, stdout)

	waitErr := cmd.Wait()

	if overrun {
		return nil, fmt.Errorf("%w: provider wrote more than %d bytes", ErrOutputTooLarge, limit)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("%w: %v", engine.ErrProviderFailed, ctxErr)
	}
	if readErr != nil {
		return nil, fmt.Errorf("%w: reading provider output: %v", engine.ErrProviderFailed, readErr)
	}
	if waitErr != nil {
		return nil, fmt.Errorf("%w: %v: %s", engine.ErrProviderFailed, waitErr, strings.TrimSpace(stderr.String()))
	}
	return &engine.ProviderOutput{Raw: out}, nil
}

// requestEnv renders a request as environment variables.
//
// An adjudicator is given the target's identity and the claimed tuple and
// nothing else: a reviewer that sees the findings is not an independent
// reviewer.
// boundedBuffer keeps the first limit bytes and counts the rest.
type boundedBuffer struct {
	limit   int
	buf     bytes.Buffer
	dropped int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room > 0 {
		if len(p) <= room {
			return b.buf.Write(p)
		}
		b.buf.Write(p[:room])
	}
	b.dropped += len(p) - max(0, b.limit-b.buf.Len())
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	if b.dropped > 0 {
		return fmt.Sprintf("%s ... (%d further bytes dropped)", b.buf.String(), b.dropped)
	}
	return b.buf.String()
}

func requestEnv(req engine.Request) []string {
	env := []string{
		"GRIMES_TARGET_ROOT=" + req.Target.GetRoot(),
		"GRIMES_TARGET_SCOPE=" + req.Target.GetScope(),
		"GRIMES_TARGET_FINGERPRINT=" + hex(req.Target.GetFingerprintSha256()),
	}
	if req.Role == engine.RoleAdjudicator {
		return append(env,
			"GRIMES_ROLE=adjudicator",
			"GRIMES_CLAIMED_DECISION="+short(req.Claimed.GetDecision().String(), "DECISION_"),
			"GRIMES_CLAIMED_RESIDUAL_RISK="+short(req.Claimed.GetResidualRisk().String(), "RESIDUAL_RISK_"),
			"GRIMES_CLAIMED_CONFIDENCE="+short(req.Claimed.GetReviewConfidence().String(), "REVIEW_CONFIDENCE_"),
			"GRIMES_CLAIMED_COMPLETENESS="+short(req.Claimed.GetReviewCompleteness().String(), "REVIEW_COMPLETENESS_"),
		)
	}
	return append(env,
		"GRIMES_ROLE=primary",
		"GRIMES_MODE="+short(req.Mode.String(), "MODE_"),
		"GRIMES_ITERATION="+strconv.FormatUint(uint64(req.Iteration), 10),
		"GRIMES_CATEGORIES="+categories(req.Categories),
		"GRIMES_RESEARCH="+req.Research,
	)
}

func categories(cats []pb.Category) string {
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		names = append(names, contracts.CategoryName(c))
	}
	return strings.Join(names, ",")
}

func short(s, prefix string) string {
	return strings.ToLower(strings.TrimPrefix(s, prefix))
}

func hex(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&0x0f])
	}
	return string(out)
}

// baseEnv is the environment the provider starts from. It deliberately does
// not inherit the parent's: a provider should receive only what it is given.
func baseEnv(extra []string) []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	return append(env, extra...)
}
