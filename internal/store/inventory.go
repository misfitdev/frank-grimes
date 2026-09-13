package store

import (
	"context"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// InventoryPath is what collection resolved, beside the ledger and the result.
const InventoryPath = ".grimes/inventory.pb"

// FileInventoryStore persists the target inventory at Path.
type FileInventoryStore struct {
	Path string
}

// NewFileInventoryStore roots the inventory under dir.
func NewFileInventoryStore(dir string) *FileInventoryStore {
	return &FileInventoryStore{Path: joinRoot(dir, InventoryPath)}
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
	return contracts.WriteAtomic(i.Path, encoded)
}
