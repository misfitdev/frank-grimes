package engine

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// reviewed holds the bytes a fix run was asked about, for the roles that come
// after the one allowed to change them.
//
// In fix mode the primary reviews and repairs in a single pass, so by the time
// an adjudicator or a refuter runs, the target on disk is the batch. An
// adjudicator would be forming its own opinion of repaired code, and a refuter
// would be attacking a claim whose anchor had just been rewritten: the one
// finding no defect and the other finding nothing to break, neither of them
// telling the engine anything about the review it is adjudicating.
//
// A whole tree rather than the reviewed units alone. Evidence of the first tier
// is a command that ran, and a command needs the files around the one it is
// about: a copy holding only the units under review would refuse every
// reproduction a refuter could offer, which is the evidence that role exists to
// produce.
type reviewed struct {
	dir string
}

// stageReviewed copies the target as it stands before the batch touches it.
//
// Taken at collection time, which is the only moment the worktree holds what
// was reviewed: a later iteration's target is the previous one's output, and
// in a run that commits nothing that output is uncommitted, so there is no
// commit to check out instead.
func stageReviewed(root, dir string) (*reviewed, error) {
	// Whatever an earlier iteration left. A stale copy is worse than none,
	// since nothing about it says which iteration it belongs to.
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := copyTree(root, dir); err != nil {
		return nil, fmt.Errorf("staging the reviewed bytes: %w", err)
	}
	return &reviewed{dir: dir}, nil
}

// discard removes the copy. A run's reviewed bytes are its own.
func (r *reviewed) discard() {
	if r == nil {
		return
	}
	_ = os.RemoveAll(r.dir)
}

// copyTree copies src to dst, skipping the review's own directory and the
// repository's.
//
// .git is skipped because the copy is not a worktree: a .git file naming the
// repository would let a role reach history through a directory the policy
// grants it whole, and nothing here needs it. .grimes is skipped because it is
// the review's own state, and the copy lives inside it.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if base := filepath.Base(rel); base == ".git" || base == contracts.GrimesDir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type()&os.ModeSymlink != 0:
			// Copied as a link rather than followed: following one that points
			// outside would put bytes nobody reviewed into the copy, under a
			// name that says they were.
			got, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(got, target)
		case !d.Type().IsRegular():
			// A socket or a device is not reviewable and not copyable.
			return nil
		}
		return copyFile(path, target, d)
	})
}

func copyFile(src, dst string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	// The mode comes across because a reproduction may turn on it: a script
	// that is executable where it was reviewed has to be executable here.
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ReviewedDir is where a fix run keeps the bytes it was asked about, inside the
// review's own directory rather than the worktree: a copy under the worktree
// would be read as something the fixer produced.
const ReviewedDir = contracts.GrimesDir + "/reviewed"
