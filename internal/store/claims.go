package store

import (
	"context"
	"path/filepath"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// ClaimsPath is what a refutation pass was asked to attack, beside the ledger.
const ClaimsPath = ".grimes/claims.pb"

// FileClaimStore persists the refutation task at path.
type FileClaimStore struct {
	path string
}

// NewFileClaimStore roots the task under dir.
func NewFileClaimStore(dir string) *FileClaimStore {
	return &FileClaimStore{path: joinRoot(dir, ClaimsPath)}
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
	if err := contracts.WriteAtomic(c.path, encoded); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(c.path)
	if err != nil {
		return c.path, nil
	}
	return abs, nil
}
