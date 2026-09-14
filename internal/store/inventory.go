package store

import (
	"context"
	"path/filepath"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// InventoryPath is what collection resolved, beside the ledger and the result.
const InventoryPath = ".grimes/inventory.pb"

// FileInventoryStore persists the target inventory at Path.
type FileInventoryStore struct {
	path string
}

// NewFileInventoryStore roots the inventory under dir.
func NewFileInventoryStore(dir string) *FileInventoryStore {
	return &FileInventoryStore{path: joinRoot(dir, InventoryPath)}
}

// Path is where a provider reads the units it must account for.
func (i *FileInventoryStore) Path() string {
	abs, err := filepath.Abs(i.path)
	if err != nil {
		return i.path
	}
	return abs
}

// Save writes the inventory atomically.
func (i *FileInventoryStore) Save(ctx context.Context, inventory *pb.TargetInventory) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	encoded, err := contracts.EncodeCanonical(inventory)
	if err != nil {
		return err
	}
	return contracts.WriteAtomic(i.path, encoded)
}
