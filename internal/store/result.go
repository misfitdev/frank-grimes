package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// ResultPath is the last derived result, beside the ledger and loop state.
const ResultPath = ".grimes/result.pb"

// FileResultStore persists the run's result at Path.
type FileResultStore struct {
	Path string
}

// NewFileResultStore roots the result under dir.
func NewFileResultStore(dir string) *FileResultStore {
	return &FileResultStore{Path: joinRoot(dir, ResultPath)}
}

// Load returns the stored result, or nil when no run has produced one.
func (r *FileResultStore) Load(ctx context.Context) (*pb.GrimesResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(r.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := &pb.GrimesResult{}
	if err := contracts.UnmarshalCanonical(data, result); err != nil {
		dest, qerr := contracts.Quarantine(r.Path, "invalid")
		if qerr != nil {
			return nil, fmt.Errorf("result invalid (%w) and could not be quarantined: %v", err, qerr)
		}
		return nil, fmt.Errorf("result invalid, quarantined to %s: %w", dest, err)
	}
	return result, nil
}

// Save writes the result atomically and returns its digest.
func (r *FileResultStore) Save(ctx context.Context, result *pb.GrimesResult) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	encoded, err := contracts.EncodeCanonical(result)
	if err != nil {
		return nil, err
	}
	if err := contracts.WriteAtomic(r.Path, encoded); err != nil {
		return nil, err
	}
	return contracts.Digest(result)
}

// Clear removes the stored result.
func (r *FileResultStore) Clear(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(r.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
