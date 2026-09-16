package store

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/engine"
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
	// Absolute: the provider resolves this from the review directory, which is
	// not necessarily the one this path was built against.
	return filepath.Abs(c.Path)
}

// Stage writes a copy for one role beside the collected content and returns
// where that role can read it.
func (c *FileContentStore) Stage(ctx context.Context, role engine.Role, content []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path := strings.TrimSuffix(c.Path, ".bin") + "-" + role.String() + ".bin"
	if err := contracts.WriteAtomic(path, content); err != nil {
		return "", err
	}
	return filepath.Abs(path)
}
