// Package store persists the ledger and loop state as canonical protobuf.
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

// FileLedger reads and writes the ledger at Path.
type FileLedger struct {
	Path string
}

// NewFileLedger roots a ledger at the contract's fixed location under dir.
func NewFileLedger(dir string) *FileLedger {
	return &FileLedger{Path: joinRoot(dir, contracts.LedgerPath)}
}

// Load returns the stored ledger, or an empty one when no file exists.
//
// An unreadable ledger is quarantined rather than deleted: it is the only
// record of how the run broke.
func (l *FileLedger) Load(ctx context.Context) (*pb.Ledger, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(l.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return &pb.Ledger{SchemaMajor: contracts.SchemaMajor}, nil
	}
	if err != nil {
		return nil, err
	}
	ledger := &pb.Ledger{}
	if err := contracts.UnmarshalCanonical(data, ledger); err != nil {
		dest, qerr := contracts.Quarantine(l.Path, "invalid")
		if qerr != nil {
			return nil, fmt.Errorf("ledger invalid (%w) and could not be quarantined: %v", err, qerr)
		}
		return nil, fmt.Errorf("ledger invalid, quarantined to %s: %w", dest, err)
	}
	return ledger, nil
}

// Save writes the ledger atomically and returns the digest of what it wrote.
func (l *FileLedger) Save(ctx context.Context, ledger *pb.Ledger) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	encoded, err := contracts.EncodeCanonical(ledger)
	if err != nil {
		return nil, err
	}
	if err := contracts.WriteAtomic(l.Path, encoded); err != nil {
		return nil, err
	}
	return contracts.Digest(ledger)
}
