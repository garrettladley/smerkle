package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/garrettladley/smerkle/internal/object"
)

const (
	objectsDir = "objects"
	indexFile  = "index"
)

type Store struct {
	root string

	index   map[string]object.IndexEntry // path -> entry
	indexMu sync.RWMutex

	dirty bool // does the index need to be written?
}

func Open(root string) (*Store, error) {
	s := &Store{
		root:  root,
		index: make(map[string]object.IndexEntry),
	}

	if err := os.MkdirAll(filepath.Join(root, objectsDir), 0o750); err != nil {
		return nil, fmt.Errorf("create objects directory: %w", err)
	}

	if err := s.loadIndex(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return s, nil
}

func (s *Store) loadIndex() error {
	data, err := os.ReadFile(filepath.Join(s.root, indexFile))
	if err != nil {
		return err //nolint:wrapcheck // caller checks os.IsNotExist
	}

	idx, err := object.DecodeIndex(data)
	if err != nil {
		return fmt.Errorf("decode index: %w", err)
	}

	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	for _, e := range idx.Entries {
		s.index[e.Path] = e
	}

	return nil
}

func (s *Store) Flush() error {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	if !s.dirty {
		return nil
	}

	entries := make([]object.IndexEntry, 0, len(s.index))
	for _, e := range s.index {
		entries = append(entries, e)
	}

	data, err := object.EncodeIndex(&object.Index{Entries: entries})
	if err != nil {
		return fmt.Errorf("encode index: %w", err)
	}

	if err := os.WriteFile(filepath.Join(s.root, indexFile), data, 0o600); err != nil {
		return fmt.Errorf("write index file: %w", err)
	}

	s.dirty = false
	return nil
}

func (s *Store) Close() error {
	return s.Flush()
}

func (s *Store) LookupCache(path string, size int64, modTime time.Time) (object.Hash, bool) {
	s.indexMu.RLock()
	defer s.indexMu.RUnlock()

	e, ok := s.index[path]
	if !ok {
		return object.ZeroHash, false
	}

	if e.Matches(path, size, modTime) {
		return e.Hash, true
	}

	return object.ZeroHash, false
}

func (s *Store) UpdateCache(path string, size int64, modTime time.Time, hash object.Hash) {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	s.index[path] = object.IndexEntry{
		Path:    path,
		Size:    size,
		ModTime: modTime,
		Hash:    hash,
	}
	s.dirty = true
}

func (s *Store) objectPath(h object.Hash) string {
	hex := h.String()
	// uses git-style sharding: first 2 hex chars as directory.
	return filepath.Join(s.root, objectsDir, hex[:2], hex[2:])
}

func (s *Store) HasObject(h object.Hash) bool {
	_, err := os.Stat(s.objectPath(h))
	return err == nil
}

func (s *Store) PutObject(h object.Hash, data []byte) error {
	path := s.objectPath(h)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create object directory: %w", err)
	}

	// write atomically via unique temp file to avoid races
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmp := f.Name()

	_, writeErr := f.Write(data)
	closeErr := f.Close()

	if writeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write object data: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close temp file: %w", closeErr)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func (s *Store) GetObject(h object.Hash) ([]byte, error) {
	return os.ReadFile(s.objectPath(h)) //nolint:wrapcheck // callers use os.IsNotExist
}

func (s *Store) PutBlob(b *object.Blob) (object.Hash, error) {
	h := b.Hash()

	if s.HasObject(h) {
		return h, nil
	}

	data, err := object.EncodeBlob(b)
	if err != nil {
		return object.ZeroHash, fmt.Errorf("encode blob: %w", err)
	}

	if err := s.PutObject(h, data); err != nil {
		return object.ZeroHash, err
	}

	return h, nil
}

func (s *Store) GetBlob(h object.Hash) (*object.Blob, error) {
	data, err := s.GetObject(h)
	if err != nil {
		return nil, err
	}

	blob, err := object.DecodeBlob(data)
	if err != nil {
		return nil, fmt.Errorf("decode blob: %w", err)
	}
	return blob, nil
}

func (s *Store) PutTree(t *object.Tree) (object.Hash, error) {
	data, err := object.EncodeTree(t)
	if err != nil {
		return object.ZeroHash, fmt.Errorf("encode tree: %w", err)
	}

	h := object.HashBytes(data)

	if s.HasObject(h) {
		return h, nil
	}

	if err := s.PutObject(h, data); err != nil {
		return object.ZeroHash, err
	}

	return h, nil
}

func (s *Store) GetTree(h object.Hash) (*object.Tree, error) {
	data, err := s.GetObject(h)
	if err != nil {
		return nil, err
	}

	tree, err := object.DecodeTree(data)
	if err != nil {
		return nil, fmt.Errorf("decode tree: %w", err)
	}
	return tree, nil
}

type Stats struct {
	ObjectCount int
	IndexSize   int
}

func (s *Store) Stats() Stats {
	s.indexMu.RLock()
	indexSize := len(s.index)
	s.indexMu.RUnlock()

	objectCount := 0
	objectsRoot := filepath.Join(s.root, objectsDir)
	_ = filepath.Walk(objectsRoot, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			objectCount++
		}
		return nil
	})

	return Stats{
		ObjectCount: objectCount,
		IndexSize:   indexSize,
	}
}
