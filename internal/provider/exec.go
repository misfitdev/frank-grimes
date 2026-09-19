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
	"syscall"
	"time"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/confine"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/engine"
	"github.com/misfitdev/frank-grimes/internal/proc"
	"github.com/misfitdev/frank-grimes/internal/store"
)

// DefaultMaxOutputBytes bounds how much of a role's stdout is retained.
//
// Sized for an agent CLI's transcript rather than for a report: the report
// arrives as a sealed file, and what comes back on stdout is whatever the role
// said on its way there. Exceeding this drops the oldest bytes; it does not
// stop the role.
const DefaultMaxOutputBytes = 16 << 20

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
	if req.Root != "" {
		p.ReadDirs = append(p.ReadDirs, req.Root)
	}
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

// collectSealed takes the report this pass sealed, if it sealed one.
//
// Read and removed here rather than left for the engine, because the same defer
// that clears an abandoned pass would clear this too. Nothing is interpreted:
// the bytes go back untouched, and the engine decides whether they answer its
// request.
func (e *Exec) collectSealed(req engine.Request) []byte {
	root, err := filepath.Abs(e.Dir)
	if err != nil {
		return nil
	}
	pass := contracts.PassToken(req.RunID, roleName(req.Role), req.Iteration)
	sealed := readSealed(filepath.Join(root, contracts.WorkEnvelopePath(pass)))
	return sealed
}

// readSealed returns what is at path, and removes it either way.
//
// A role may write this directory, so it may put something other than a file at
// the path the engine collects from. Opening a FIFO blocks until a writer
// arrives, and this read happens after Wait: neither the command timeout nor
// WaitDelay reaches it, so a role that left one behind would hold the engine
// open for as long as it liked. O_NONBLOCK is what makes the open return, and
// anything that then refuses to read like a file reads as no report at all.
func readSealed(path string) []byte {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(path)
	}()
	body, err := io.ReadAll(io.LimitReader(f, DefaultMaxOutputBytes))
	if err != nil {
		return nil
	}
	return body
}

// discardAbandoned removes whatever this pass was accumulating, if it is still
// there when the pass ends.
//
// A sealed report clears itself, so anything left is a pass that did not get
// that far. Found by looking rather than by name: a role may accumulate into a
// file of its own choosing, and the one place it can write them is this
// directory, so every marker in it is asked who it belongs to.
//
// Only this pass's own leftovers are touched. A marker naming another pass is
// that pass's business, and a report opened outside any pass has no marker at
// all — the documented sequence, where a review builds its report before the
// run that carries it.
func (e *Exec) discardAbandoned(req engine.Request) {
	root, err := filepath.Abs(e.Dir)
	if err != nil {
		return
	}
	markers, err := filepath.Glob(filepath.Join(root, store.WorkDir, "*"+passSuffix))
	if err != nil {
		return
	}
	mine := contracts.PassToken(req.RunID, roleName(req.Role), req.Iteration)
	// A pass that sealed and then died -- a non-zero exit, a timeout -- never
	// reached collection, so its envelope is still here. The pass token is the
	// same for a repeated invocation of this run, role and iteration, and a
	// retry would collect that report before producing one of its own.
	_ = os.Remove(filepath.Join(root, contracts.WorkEnvelopePath(mine)))
	for _, marker := range markers {
		held, err := os.ReadFile(marker)
		if err != nil || strings.TrimSpace(string(held)) != mine {
			continue
		}
		_ = os.Remove(strings.TrimSuffix(marker, passSuffix))
		_ = os.Remove(marker)
	}
}

// passSuffix names a report's marker from the report. The contract CLI writes
// it; this is the only other place that has to recognise one.
const passSuffix = ".pass"

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

	// A pass that does not come back leaves whatever it was accumulating, and
	// the file outlives the process. Cleared here rather than before the next
	// spawn, because only this pass knows the leftovers are its own: a report
	// built before the run began belongs to whoever built it.
	defer e.discardAbandoned(req)

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
	// Absolute, so a role that seals from a directory of its own choosing still
	// delivers where the engine collects. Taken from the Exec rather than the
	// request, since this is where the child is about to be run.
	cmd.Env = append(cmd.Env, "GRIMES_WORK_DIR="+absolute(filepath.Join(e.Dir, store.WorkDir)))
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

	// Drained rather than bounded by a kill. A role that talks past the limit
	// is still reviewing, and the bytes over it are not the answer: the report
	// comes back as a sealed file. Killing here would end a review over its
	// narration. A role that never stops is ended by the timeout instead.
	kept := &boundedBuffer{limit: int(limit)}
	_, readErr := io.Copy(kept, stdout)
	out := kept.bytes()

	waitErr := cmd.Wait()

	// Before any of the returns below, so that the passes worth diagnosing --
	// a timeout, a non-zero exit -- are the ones that leave a record rather
	// than the ones that do not.
	e.record(req, argv, out, stderr.String(), waitErr, kept.dropped()+stderr.dropped())

	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("%w: %v", engine.ErrProviderFailed, ctxErr)
	}
	if readErr != nil {
		return nil, fmt.Errorf("%w: reading provider output: %v", engine.ErrProviderFailed, readErr)
	}
	if waitErr != nil {
		return nil, fmt.Errorf("%w: %v: %s", engine.ErrProviderFailed, waitErr, strings.TrimSpace(stderr.String()))
	}
	return &engine.ProviderOutput{
		Raw:         out,
		Diagnostics: strings.TrimSpace(stderr.String()),
		Sealed:      e.collectSealed(req),
	}, nil
}

// RoleLog names where a role's pass is recorded, under the work directory the
// role itself may write to.
func RoleLog(role engine.Role) string {
	return store.WorkDir + "/" + roleName(role) + ".log"
}

// record appends one pass to the role's log.
//
// Both streams are already bounded by the caller, so this writes what the
// engine kept rather than everything the role emitted. Appended rather than
// truncated: a role runs once per iteration, and a pass is read against the
// ones before it.
//
// Best effort. A run that produced a verdict is not failed for want of a note
// about how it got there.
func (e *Exec) record(req engine.Request, argv []string, stdout []byte, stderr string, waitErr error, dropped int) {
	var b strings.Builder
	fmt.Fprintf(&b, "=== %s iteration %d: %s\n", roleName(req.Role), req.Iteration, strings.Join(argv, " "))
	if waitErr != nil {
		fmt.Fprintf(&b, "exit: %v\n", waitErr)
	} else {
		b.WriteString("exit: 0\n")
	}
	if dropped > 0 {
		fmt.Fprintf(&b, "dropped: %d bytes over the retention bound, oldest first\n", dropped)
	}
	fmt.Fprintf(&b, "--- stdout (%d bytes) ---\n", len(stdout))
	b.Write(stdout)
	if len(stdout) > 0 && !strings.HasSuffix(string(stdout), "\n") {
		b.WriteString("\n")
	}
	b.WriteString("--- stderr ---\n")
	b.WriteString(stderr)
	if stderr != "" && !strings.HasSuffix(stderr, "\n") {
		b.WriteString("\n")
	}

	path := filepath.Join(e.Dir, RoleLog(req.Role))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(b.String())
}

// boundedBuffer keeps the last limit bytes written and counts the rest.
//
// The tail rather than the head: the head is where a role was still setting
// up, and the tail is where it was when whatever happened to it happened.
type boundedBuffer struct {
	limit int
	buf   bytes.Buffer
	cut   int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.buf.Write(p)
	// Trimmed at twice the bound rather than on every write, which would copy
	// the whole retained tail each time. What is held is bounded either way.
	if b.buf.Len() > 2*b.limit {
		over := b.buf.Len() - b.limit
		b.buf.Next(over)
		b.cut += over
	}
	// Report the full length: a short write from a Stderr sink stops the pipe.
	return len(p), nil
}

// bytes returns the retained tail.
func (b *boundedBuffer) bytes() []byte {
	body := b.buf.Bytes()
	if len(body) > b.limit {
		return body[len(body)-b.limit:]
	}
	return body
}

// dropped counts what was written and not retained, including the part not yet
// trimmed away.
func (b *boundedBuffer) dropped() int {
	if over := b.buf.Len() - b.limit; over > 0 {
		return b.cut + over
	}
	return b.cut
}

func (b *boundedBuffer) String() string {
	if n := b.dropped(); n > 0 {
		return fmt.Sprintf("(%d earlier bytes dropped) ... %s", n, b.bytes())
	}
	return string(b.bytes())
}

// roleName is how a role is written into the environment and into the token
// that names its pass.
func roleName(r engine.Role) string {
	switch r {
	case engine.RoleAdjudicator:
		return "adjudicator"
	case engine.RoleRefuter:
		return "refuter"
	default:
		return "primary"
	}
}

// targetRoot is the directory this role resolves unit ids against.
//
// The target's own, except for a role handed a copy of it: in fix mode the
// roles after the fixing one read the bytes as they were reviewed, and a unit
// id pointed at the worktree instead would open the batch.
func targetRoot(req engine.Request) string {
	if req.Root != "" {
		return req.Root
	}
	return req.Target.GetRoot()
}

func requestEnv(req engine.Request) []string {
	env := []string{
		// Absolute, like the content path and for the same reason: a provider
		// runs in its own working directory, so a relative root names whatever
		// happens to sit beside it. A unit id is relative to this root, so a
		// provider that cannot resolve the root cannot read the unit it is
		// about to cite.
		"GRIMES_TARGET_ROOT=" + absolute(targetRoot(req)),
		"GRIMES_TARGET_SCOPE=" + req.Target.GetScope(),
		"GRIMES_TARGET_FINGERPRINT=" + hex(req.Target.GetFingerprintSha256()),
		// The kind selects what each evidence tier requires of a finding, so a
		// provider that did not know it would cite a document as if it were code.
		"GRIMES_TARGET_KIND=" + strings.ToLower(short(req.Target.GetKind().String(), "TARGET_KIND_")),
		// Where the reviewed bytes are. Both roles get it: an adjudicator that
		// cannot see the artifact cannot form an opinion of its own, and zero
		// knowledge is about the first report, not the target.
		"GRIMES_TARGET_CONTENT=" + req.ContentPath,
		// Which pass this is. The contract CLI stamps it on the report it is
		// accumulating, so a pass that died before sealing cannot have its
		// candidates delivered by the next one.
		"GRIMES_PASS=" + contracts.PassToken(req.RunID, roleName(req.Role), req.Iteration),
	}
	// The claimed tuple is deliberately withheld. An independent review that is
	// shown the conclusion it is meant to reach is anchored by construction;
	// the engine resolves the two tuples afterwards, which needs nothing shown
	// to the adjudicator beforehand.
	if req.Role == engine.RoleAdjudicator {
		// The run it answers under, so the engine can refuse a verdict reached
		// for some other one. Not knowledge of the first review: it is the
		// identity of the request, which every other role is given.
		return append(env,
			"GRIMES_ROLE=adjudicator",
			"GRIMES_RUN_ID="+req.RunID,
		)
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
