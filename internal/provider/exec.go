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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/confine"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/engine"
	"github.com/misfitdev/frank-grimes/internal/proc"
	"github.com/misfitdev/frank-grimes/internal/store"
)

// DefaultMaxOutputBytes bounds a single provider's stdout.
const DefaultMaxOutputBytes = 1 << 20

// DefaultTimeout bounds a single provider invocation.
const DefaultTimeout = 10 * time.Minute

// stderrLimit bounds retained provider diagnostics.
const stderrLimit = 8 << 10

// Exec invokes Command with the request supplied through the environment.
type Exec struct {
	Command []string
	Dir     string
	Env     []string
	// Confine bounds what the spawned role can reach. A nil mechanism is not a
	// default: the engine resolves one, or records that the operator asked for
	// none, before it builds this.
	Confine        confine.Mechanism
	MaxOutputBytes int64
	Timeout        time.Duration
}

// confined returns the argv to spawn in place of the provider's own.
//
// The policy is built from the request, never from the command: what a role may
// read is what the engine handed it, so a provider shipping next year needs no
// entry here and no flag of its own.
func (e *Exec) confined(req engine.Request) ([]string, error) {
	root, err := filepath.Abs(e.Dir)
	if err != nil {
		return nil, err
	}
	p := confine.Policy{Root: root}
	for _, path := range []string{req.ContentPath, req.InventoryPath, req.ClaimsPath} {
		if path != "" {
			p.ReadPaths = append(p.ReadPaths, path)
		}
	}
	// Every role records what it produced through the contract CLI, which writes
	// it here. Created by the engine because creating it is itself a write into
	// the review directory the role is not allowed to make.
	//
	work := filepath.Join(root, store.WorkDir)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return nil, err
	}
	p.WriteDirs = append(p.WriteDirs, work)
	// A fixing role is handed the worktree as well: the target it is reviewing
	// stays read-only, and the copy it was authorized to change does not.
	if req.WriteRoot != "" {
		p.WriteDirs = append(p.WriteDirs, req.WriteRoot)
	}
	return e.Confine.Wrap(p, e.Command)
}

// lookup reports where the role's command will be found, resolving it the way
// the spawned process will.
//
// A command naming a directory is resolved against the working directory the
// child is given, not the one the engine happens to be in, so checking it here
// against the engine's would reject a provider the child could have run.
func (e *Exec) lookup() error {
	name := e.Command[0]
	if strings.ContainsRune(name, filepath.Separator) && !filepath.IsAbs(name) {
		dir, err := filepath.Abs(e.Dir)
		if err != nil {
			return err
		}
		// Absolute rather than joined: joining against a relative directory can
		// drop the leading "./", which turns a path into a bare name and sends
		// the lookup to $PATH instead of the directory it names.
		name = filepath.Join(dir, name)
	}
	_, err := exec.LookPath(name)
	return err
}

var ErrOutputTooLarge = engine.ErrOutputTooLarge

// Review runs the subprocess and returns its stdout.
func (e *Exec) Review(ctx context.Context, req engine.Request) (*engine.ProviderOutput, error) {
	if len(e.Command) == 0 {
		return nil, errors.New("no provider command configured")
	}
	if e.Confine == nil {
		return nil, errors.New("no confinement mechanism configured")
	}
	// Resolved here rather than at spawn, because a wrapper makes the argv the
	// kernel sees the wrapper's own: a missing provider would surface as that
	// wrapper's failure, or as nothing at all.
	if err := e.lookup(); err != nil {
		return nil, fmt.Errorf("%w: %v", engine.ErrProviderFailed, err)
	}

	argv, err := e.confined(req)
	if err != nil {
		return nil, err
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

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = e.Dir
	cmd.Env = append(baseEnv(e.Env), requestEnv(req)...)
	// A role is given a request, not a conversation. Left inherited, a provider
	// that decided to prompt would hold the operator's terminal until the
	// timeout, and read whatever was typed at it in the meantime.
	cmd.Stdin = nil
	proc.SetGroup(cmd)
	// CommandContext kills only the direct child; a shell provider leaves its
	// own children holding the pipe open and Wait blocks past the deadline.
	cmd.Cancel = func() error { return proc.KillGroup(cmd) }
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
		_ = proc.KillGroup(cmd)
		// Closed rather than drained: a descendant that left the process group
		// survives the kill, and draining its output would wait on a writer
		// that has no reason to stop.
		_ = stdout.Close()
	}

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

// boundedBuffer keeps the first limit bytes and counts the rest.
type boundedBuffer struct {
	limit   int
	buf     bytes.Buffer
	dropped int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	// How much is kept has to be decided before the write, since writing moves
	// the remaining room and would make retained bytes look dropped.
	kept := min(len(p), max(0, b.limit-b.buf.Len()))
	if kept > 0 {
		b.buf.Write(p[:kept])
	}
	b.dropped += len(p) - kept
	// Report the full length: a short write from a Stderr sink stops the pipe.
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
		// Absolute, like the content path and for the same reason: a provider
		// runs in its own working directory, so a relative root names whatever
		// happens to sit beside it. A unit id is relative to this root, so a
		// provider that cannot resolve the root cannot read the unit it is
		// about to cite.
		"GRIMES_TARGET_ROOT=" + absolute(req.Target.GetRoot()),
		"GRIMES_TARGET_SCOPE=" + req.Target.GetScope(),
		"GRIMES_TARGET_FINGERPRINT=" + hex(req.Target.GetFingerprintSha256()),
		// The kind selects what each evidence tier requires of a finding, so a
		// provider that did not know it would cite a document as if it were code.
		"GRIMES_TARGET_KIND=" + strings.ToLower(short(req.Target.GetKind().String(), "TARGET_KIND_")),
		// Where the reviewed bytes are. Both roles get it: an adjudicator that
		// cannot see the artifact cannot form an opinion of its own, and zero
		// knowledge is about the first report, not the target.
		"GRIMES_TARGET_CONTENT=" + req.ContentPath,
	}
	// The claimed tuple is deliberately withheld. An independent review that is
	// shown the conclusion it is meant to reach is anchored by construction;
	// the engine resolves the two tuples afterwards, which needs nothing shown
	// to the adjudicator beforehand.
	if req.Role == engine.RoleAdjudicator {
		return append(env, "GRIMES_ROLE=adjudicator")
	}
	// A refuter gets the claims and the run it must answer under. It is given no
	// inventory and no categories: it is not reviewing the target, and a
	// coverage denominator would invite it to.
	if req.Role == engine.RoleRefuter {
		return append(env,
			"GRIMES_ROLE=refuter",
			"GRIMES_RUN_ID="+req.RunID,
			"GRIMES_CLAIMS="+req.ClaimsPath,
		)
	}
	return append(env,
		"GRIMES_ROLE=primary",
		"GRIMES_RUN_ID="+req.RunID,
		// What this review must account for. Coverage is measured against it,
		// so a provider that could not read it would be graded on a set it was
		// never shown.
		"GRIMES_TARGET_INVENTORY="+req.InventoryPath,
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

// absolute resolves a root the provider can use from its own directory. An
// empty root stays empty: a document or an idea has none, and inventing one
// would name a repository this review is not about.
func absolute(root string) string {
	if root == "" {
		return ""
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	return abs
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
