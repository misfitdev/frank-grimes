package store

import (
	"context"

	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// ContentPath holds a target that arrived with no path of its own, beside the
// inventory and the ledger.
const ContentPath = ".grimes/target.bin"

// FileContentStore persists collected bytes at Path.
type FileContentStore struct {
	Path string
}

// NewFileContentStore roots the content under dir.
func NewFileContentStore(dir string) *FileContentStore {
	return &FileContentStore{Path: joinRoot(dir, ContentPath)}
}

// Save writes the content atomically and returns where a provider can read it.
func (c *FileContentStore) Save(ctx context.Context, content []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := contracts.WriteAtomic(c.Path, content); err != nil {
		return "", err
	}
	return c.Path, nil
}
