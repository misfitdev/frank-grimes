package contracts

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"buf.build/go/protovalidate"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// LedgerPath is the authoritative ledger location. The JSON projection beside
// it is for reading only and is never accepted as control input.
const LedgerPath = ".grimes/ledger.pb"

var validator protovalidate.Validator

func init() {
	v, err := protovalidate.New()
	if err != nil {
		panic(fmt.Sprintf("protovalidate init: %v", err))
	}
	validator = v
}

// Validate applies every constraint declared in the .proto.
func Validate(m proto.Message) error {
	return validator.Validate(m)
}

// deterministic marshalling: map entries ordered, no unknown field passthrough.
func marshalCanonical(m proto.Message) ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(m)
}

// UnmarshalCanonical decodes bytes and refuses anything that is not already the
// canonical encoding of what it decodes to.
//
// The round-trip equality check is the point: it rejects duplicate singular
// fields, non-minimal varints, alternate field ordering, and trailing garbage,
// none of which change the decoded value but all of which let two different
// byte strings claim to be the same result. A hook that trusts a result block
// out of an assistant message needs that guarantee.
func UnmarshalCanonical(b []byte, m proto.Message) error {
	opts := proto.UnmarshalOptions{DiscardUnknown: false}
	if err := opts.Unmarshal(b, m); err != nil {
		return fmt.Errorf("malformed wire data: %w", err)
	}
	if n := len(m.ProtoReflect().GetUnknown()); n > 0 {
		return fmt.Errorf("message carries %d bytes of unknown fields; schema drift or tampering", n)
	}
	if err := hasNoUnknownFields(m.ProtoReflect()); err != nil {
		return err
	}
	if err := Validate(m); err != nil {
		return err
	}
	round, err := marshalCanonical(m)
	if err != nil {
		return fmt.Errorf("re-marshal: %w", err)
	}
	if !bytes.Equal(b, round) {
		return fmt.Errorf("non-canonical encoding: input is %d bytes, canonical form is %d", len(b), len(round))
	}
	return nil
}

// Nested messages carry their own unknown-field sets, so the top-level check is
// not sufficient on its own.
func hasNoUnknownFields(m protoreflect.Message) error {
	var err error
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsMap():
			if fd.MapValue().Kind() == protoreflect.MessageKind {
				v.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
					err = hasNoUnknownFields(mv.Message())
					return err == nil
				})
			}
		case fd.IsList() && fd.Kind() == protoreflect.MessageKind:
			l := v.List()
			for i := 0; i < l.Len(); i++ {
				if err = hasNoUnknownFields(l.Get(i).Message()); err != nil {
					return false
				}
			}
		case fd.Kind() == protoreflect.MessageKind:
			err = hasNoUnknownFields(v.Message())
		}
		return err == nil
	})
	if err != nil {
		return err
	}
	if n := len(m.GetUnknown()); n > 0 {
		return fmt.Errorf("nested message %s carries unknown fields", m.Descriptor().FullName())
	}
	return nil
}

// EncodeCanonical validates then produces the deterministic encoding.
func EncodeCanonical(m proto.Message) ([]byte, error) {
	if err := Validate(m); err != nil {
		return nil, err
	}
	return marshalCanonical(m)
}

// Digest is the SHA-256 of the canonical encoding.
func Digest(m proto.Message) ([]byte, error) {
	b, err := EncodeCanonical(m)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	return sum[:], nil
}

// ParseTextProto reads a TextProto fixture.
func ParseTextProto(b []byte, m proto.Message) error {
	if err := prototext.Unmarshal(b, m); err != nil {
		return fmt.Errorf("textproto: %w", err)
	}
	return Validate(m)
}

// WriteAtomic writes through a temporary file in the same directory followed by
// a rename, so a crash mid-write cannot leave a half-written ledger that the
// next run would quarantine.
//
// The directory is synced after the rename as well as the file before it. A
// rename is a directory modification, so without that a caller can be told its
// write succeeded and find no file after power loss.
func WriteAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	// Directories MkdirAll has to create are new names in their own parents, and
	// a name is only durable once the parent that holds it is synced. Syncing
	// the file's directory alone would leave the record reachable through a path
	// that did not survive.
	created, err := makeDirs(dir)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if err := syncDirectory(dir); err != nil {
		return err
	}
	// Parents outward, so a directory is synced only after the one it holds.
	for _, d := range created {
		if err := syncDirectory(filepath.Dir(d)); err != nil {
			return err
		}
	}
	return nil
}

// makeDirs creates dir and returns the directories it had to create, innermost
// first. An existing dir returns nothing to sync.
func makeDirs(dir string) ([]string, error) {
	var created []string
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		created = append(created, d)
		if parent := filepath.Dir(d); parent == d {
			break
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return created, nil
}

// syncDirectory is a variable so a test can observe that it was called. An
// fsync has no effect anything short of power loss can see, so without this the
// call could be dropped and every assertion would still pass.
var syncDirectory = syncDir

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	// Some filesystems refuse fsync on a directory. The rename already happened,
	// so a refusal is not a failed write; a real I/O error still is.
	if err := d.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) &&
		!errors.Is(err, syscall.ENOTSUP) {
		d.Close()
		return err
	}
	return d.Close()
}

// Quarantine moves an unusable file aside rather than deleting it. A corrupt
// ledger is the only record of how the run broke.
func Quarantine(path, reason string) (string, error) {
	dir := filepath.Join(filepath.Dir(filepath.Dir(path)), ".grimes", "quarantine")
	if base := filepath.Dir(path); filepath.Base(base) == ".grimes" {
		dir = filepath.Join(base, "quarantine")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, fmt.Sprintf("%s-%s-%d-%s",
		filepath.Base(path), time.Now().UTC().Format("20060102T150405Z"), os.Getpid(), reason))
	return dest, os.Rename(path, dest)
}

// SchemaMajor is the contract major every persisted message declares. It is a
// const rule in the .proto, so a mismatch is a rejection rather than a
// migration.
const SchemaMajor uint32 = 2
