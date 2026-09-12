package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// StatePath is the loop state location, beside the ledger.
//
// This is not .grimes-state.json: that file belongs to hooks/stop.sh, which
// still owns its own loop.
const StatePath = ".grimes/state.pb"

// FileStateStore persists LoopState at Path.
type FileStateStore struct {
	Path string
}

// NewFileStateStore roots loop state under dir.
func NewFileStateStore(dir string) *FileStateStore {
	return &FileStateStore{Path: joinRoot(dir, StatePath)}
}

// Load returns the stored state, or nil when no run is in progress.
func (s *FileStateStore) Load(ctx context.Context) (*pb.LoopState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	state := &pb.LoopState{}
	if err := contracts.UnmarshalCanonical(data, state); err != nil {
		dest, qerr := contracts.Quarantine(s.Path, "invalid")
		if qerr != nil {
			return nil, fmt.Errorf("loop state invalid (%w) and could not be quarantined: %v", err, qerr)
		}
		return nil, fmt.Errorf("loop state invalid, quarantined to %s: %w", dest, err)
	}
	return state, nil
}

// Save writes loop state atomically.
func (s *FileStateStore) Save(ctx context.Context, state *pb.LoopState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	encoded, err := contracts.EncodeCanonical(state)
	if err != nil {
		return err
	}
	return contracts.WriteAtomic(s.Path, encoded)
}

// Clear removes loop state. A run that never started is already clear.
func (s *FileStateStore) Clear(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func joinRoot(dir, rel string) string {
	if dir == "" {
		return rel
	}
	return filepath.Join(dir, rel)
}
