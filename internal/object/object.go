package object

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

type Hash [32]byte

var _ fmt.Stringer = Hash{}

var ZeroHash Hash

func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

func (h Hash) Bytes() []byte {
	return h[:]
}

func (h Hash) IsZero() bool {
	return h == ZeroHash
}

func HashBytes(data []byte) Hash {
	return sha256.Sum256(data)
}

type Mode uint8

const (
	ModeRegular    Mode = 0
	ModeExecutable Mode = 1
	ModeDirectory  Mode = 2
	ModeSymlink    Mode = 3
)

func (m Mode) String() string {
	switch m {
	case ModeRegular:
		return "regular"
	case ModeExecutable:
		return "executable"
	case ModeDirectory:
		return "directory"
	case ModeSymlink:
		return "symlink"
	default:
		return "unknown"
	}
}

func (m Mode) IsFile() bool {
	return m == ModeRegular || m == ModeExecutable
}

type Entry struct {
	Name    string
	Mode    Mode
	Size    int64
	ModTime time.Time
	Hash    Hash
}

type Blob struct {
	Content []byte
}

func (b *Blob) Hash() Hash {
	return HashBytes(b.Content)
}

type Tree struct {
	Entries []Entry
}

type IndexEntry struct {
	Path    string
	Size    int64
	ModTime time.Time
	Hash    Hash
}

func (e *IndexEntry) Matches(path string, size int64, modTime time.Time) bool {
	return e.Path == path && e.Size == size && e.ModTime.Equal(modTime)
}
