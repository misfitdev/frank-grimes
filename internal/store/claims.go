package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// ClaimsDir holds what refutation passes were asked to attack, beside the
// ledger. One file per run: two runs over a directory would otherwise overwrite
// each other's task between the save and the refuter's read, and a refuter
// would be handed another run's claims and control.
const ClaimsDir = ".grimes"

// FileClaimStore persists a refutation task under dir.
type FileClaimStore struct {
	dir string
}

// NewFileClaimStore roots the tasks under dir.
func NewFileClaimStore(dir string) *FileClaimStore {
	return &FileClaimStore{dir: joinRoot(dir, ClaimsDir)}
}

// Save writes the task atomically and returns where a refuter can read it.
// Absolute, since the refuter runs in its own working directory.
func (c *FileClaimStore) Save(ctx context.Context, task *pb.RefutationTask) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	encoded, err := contracts.EncodeCanonical(task)
	if err != nil {
		return "", err
	}
	path := filepath.Join(c.dir, "claims-"+RunSlug(task.GetRunId())+".pb")
	if err := contracts.WriteAtomic(path, encoded); err != nil {
		return "", err
	}
	// The refuter runs with its own working directory, so a relative path here
	// would resolve against the wrong root and read as a missing file.
	return filepath.Abs(path)
}

// Discard removes a task the pass is done with. A pass that left its file
// behind would accumulate one per run.
func (c *FileClaimStore) Discard(_ context.Context, path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RunSlug names a run in a path. A run identity is not constrained to the
// characters a filename admits, so it is digested rather than sanitized: two
// distinct runs that sanitize alike would collide on the file this is naming.
func RunSlug(runID string) string {
	sum := sha256.Sum256([]byte(runID))
	return hex.EncodeToString(sum[:8])
}

// WorkDir, relative to the review directory, is the only place a spawned role
// may write. It holds what a role is building — its report, its refutation —
// and nothing the engine relies on: the ledger, the result, the loop state and
// the staged copies all sit outside it and stay unreadable.
//
// Static rather than derived from the run, because a report can be started
// before the run that carries it exists, and both halves have to name the same
// file without an agreement passed between them.
const WorkDir = ".grimes/work"
