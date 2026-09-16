package confine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrNotConfined reports a mechanism that let the probe through.
var ErrNotConfined = errors.New("confinement blocked nothing")

// probeTimeout bounds the canary. It runs one shell builtin against two paths;
// anything slower than this is a mechanism that is not going to work.
const probeTimeout = 30 * time.Second

// Verify runs a canary under the mechanism and reports whether it was actually
// confined.
//
// A policy that was applied and a policy that was silently ignored produce the
// same successful run, so the engine does not take either on faith. This is the
// same rule the refutation pass is held to: a check that cannot be shown to
// fail distinguishes nothing. It is also what catches the day a platform
// changes under the built-in backend, which is a likelier end for sandbox-exec
// than the removal its deprecation notice has been threatening since 2016.
//
// The canary is graded on what it did, not on what it returned. A wrapper is
// free to exit zero while blocking everything, or to eat the exit code of what
// it ran.
func Verify(ctx context.Context, m Mechanism, root string) error {
	if _, ok := m.(Unsafe); ok {
		return nil
	}

	dir, err := os.MkdirTemp(filepath.Join(root, GrimesDir), "probe-")
	if err != nil {
		return fmt.Errorf("confine: staging the probe: %w", err)
	}
	defer os.RemoveAll(dir)

	// Named so that neither outcome can be produced by accident: an empty file
	// and a missing file are both ordinary, a file holding this is not.
	const secret = "grimes-probe-must-not-be-readable"
	readable := filepath.Join(dir, "denied")
	if err := os.WriteFile(readable, []byte(secret), 0o600); err != nil {
		return fmt.Errorf("confine: staging the probe: %w", err)
	}
	written := filepath.Join(root, "grimes-probe-written")

	// The probe is confined by a policy that hands it nothing: no readable
	// path, no writable directory. Every mechanism that works at all must stop
	// both of these.
	argv, err := m.Wrap(Policy{Root: root}, []string{
		"/bin/sh", "-c",
		fmt.Sprintf("cat %s 2>/dev/null; : >%s 2>/dev/null; exit 0", shellQuote(readable), shellQuote(written)),
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = root
	out, runErr := cmd.Output()
	// A mechanism that could not start is not one that confined anything, and
	// grading it on the probe's effects would report it as a success.
	var notStarted *exec.Error
	if errors.As(runErr, &notStarted) {
		return fmt.Errorf("confine: %s could not run: %w", m.Name(), notStarted)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("confine: %s did not finish: %w", m.Name(), ctxErr)
	}

	var leaked []string
	if strings.Contains(string(out), secret) {
		leaked = append(leaked, "read a file it was not handed")
	}
	if _, err := os.Stat(written); err == nil {
		_ = os.Remove(written)
		leaked = append(leaked, "wrote inside the target")
	}
	if len(leaked) > 0 {
		return fmt.Errorf("%w: under %s a probe %s", ErrNotConfined, m.Name(), strings.Join(leaked, " and "))
	}
	return nil
}

// shellQuote renders a path for the single sh -c the probe runs.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
