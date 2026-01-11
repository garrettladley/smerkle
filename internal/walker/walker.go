package walker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/garrettladley/smerkle/internal/ignore"
	"github.com/garrettladley/smerkle/internal/object"
	"github.com/garrettladley/smerkle/internal/result"
	"github.com/garrettladley/smerkle/internal/store"
	"github.com/garrettladley/smerkle/internal/xerrors"
)

var (
	ErrRootNotDirectory = errors.New("walker: root is not a directory")
	ErrRootNotExist     = errors.New("walker: root does not exist")
)

const smerkleignoreFile = ".smerkleignore"

type Walker struct {
	root    string
	store   *store.Store
	ignorer *ignore.Ignorer
	ec      *xerrors.ErrorCollector
}

type Option func(*Walker)

func WithIgnorer(ign *ignore.Ignorer) Option {
	return func(w *Walker) {
		w.ignorer = ign
	}
}

// walk recursively traverses root, building a Merkle tree.
// loads .smerkleignore from root if present.
func Walk(ctx context.Context, root string, s *store.Store, opts ...Option) (*result.Result, error) {
	w := &Walker{
		root:  root,
		store: s,
	}
	for _, opt := range opts {
		opt(w)
	}

	info, err := os.Stat(w.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrRootNotExist
		}
		return nil, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, ErrRootNotDirectory
	}

	if w.ignorer == nil {
		var ign *ignore.Ignorer
		ignorePath := filepath.Join(root, smerkleignoreFile)
		if _, err := os.Stat(ignorePath); err == nil {
			ign, err = ignore.NewFromFile(ignorePath)
			if err != nil {
				return nil, fmt.Errorf("load ignore file: %w", err)
			}
		}
		w.ignorer = ign
	}

	w.ec = xerrors.NewErrorCollector()

	return w.walk(ctx)
}

func (w *Walker) walk(ctx context.Context) (*result.Result, error) {
	hash, err := w.walkDir(ctx, w.root, "")
	if err != nil {
		return nil, err
	}

	return &result.Result{
		Hash:   hash,
		Errors: w.ec.Errors(),
	}, nil
}

// walkDir walks a single directory recursively and returns its tree hash.
func (w *Walker) walkDir(ctx context.Context, absDir, relDir string) (object.Hash, error) {
	if err := ctx.Err(); err != nil {
		return object.ZeroHash, fmt.Errorf("context: %w", err)
	}

	dirEntries, err := os.ReadDir(absDir)
	if err != nil {
		return object.ZeroHash, fmt.Errorf("read dir: %w", err)
	}

	var entries []object.Entry
	for _, de := range dirEntries {
		name := de.Name()

		// skip .smerkleignore file itself
		if name == smerkleignoreFile {
			continue
		}

		relPath := name
		if relDir != "" {
			relPath = filepath.Join(relDir, name)
		}
		absPath := filepath.Join(absDir, name)

		entry, err := w.processEntry(ctx, absPath, relPath, name)
		if err != nil {
			return object.ZeroHash, err
		}
		if entry != nil {
			entries = append(entries, *entry)
		}
	}

	// sort entries by name for determinism
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	tree := &object.Tree{Entries: entries}
	hash, err := w.store.PutTree(tree)
	if err != nil {
		return object.ZeroHash, fmt.Errorf("put tree: %w", err)
	}
	return hash, nil
}

// processEntry processes a single directory entry and returns the corresponding tree entry.
// returns nil entry if the entry should be skipped (ignored or error collected).
func (w *Walker) processEntry(ctx context.Context, absPath, relPath, name string) (*object.Entry, error) {
	info, err := os.Lstat(absPath)
	if err != nil {
		w.ec.Add(relPath, err)
		return nil, nil
	}

	isDir := info.IsDir()

	if w.ignorer != nil && w.ignorer.Match(relPath, isDir) {
		return nil, nil
	}

	if isDir {
		return w.processDirEntry(ctx, absPath, relPath, name, info)
	}
	return w.processFileEntry(ctx, absPath, relPath, info)
}

// processDirEntry processes a directory entry.
func (w *Walker) processDirEntry(ctx context.Context, absPath, relPath, name string, info os.FileInfo) (*object.Entry, error) {
	hash, err := w.walkDir(ctx, absPath, relPath)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		w.ec.Add(relPath, err)
		return nil, nil
	}
	return &object.Entry{
		Name:    name,
		Mode:    object.ModeDirectory,
		Size:    0,
		ModTime: info.ModTime(),
		Hash:    hash,
	}, nil
}

// processFileEntry processes a file or symlink entry.
func (w *Walker) processFileEntry(ctx context.Context, absPath, relPath string, info os.FileInfo) (*object.Entry, error) {
	entry, err := w.hashFile(ctx, absPath, relPath, info)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		w.ec.Add(relPath, err)
		return nil, nil
	}
	return &entry, nil
}

// hashFile hashes a single file and returns its entry.
func (w *Walker) hashFile(ctx context.Context, absPath, relPath string, info os.FileInfo) (object.Entry, error) {
	if err := ctx.Err(); err != nil {
		return object.Entry{}, fmt.Errorf("context: %w", err)
	}

	mode := modeFromFileInfo(info)
	name := filepath.Base(relPath)

	// try cache for non-symlinks
	if mode != object.ModeSymlink {
		if hash, ok := w.store.LookupCache(relPath, info.Size(), info.ModTime()); ok {
			return object.Entry{
				Name:    name,
				Mode:    mode,
				Size:    info.Size(),
				ModTime: info.ModTime(),
				Hash:    hash,
			}, nil
		}
	}

	content, err := readContent(absPath, mode)
	if err != nil {
		return object.Entry{}, err
	}

	blob := &object.Blob{Content: content}
	hash, err := w.store.PutBlob(blob)
	if err != nil {
		return object.Entry{}, fmt.Errorf("put blob: %w", err)
	}

	// update cache for non-symlinks
	if mode != object.ModeSymlink {
		w.store.UpdateCache(relPath, info.Size(), info.ModTime(), hash)
	}

	return object.Entry{
		Name:    name,
		Mode:    mode,
		Size:    info.Size(),
		ModTime: info.ModTime(),
		Hash:    hash,
	}, nil
}

// readContent reads the content of a file or symlink target.
func readContent(absPath string, mode object.Mode) ([]byte, error) {
	if mode == object.ModeSymlink {
		target, err := os.Readlink(absPath)
		if err != nil {
			return nil, fmt.Errorf("readlink: %w", err)
		}
		return []byte(target), nil
	}

	content, err := os.ReadFile(absPath) //nolint:gosec // absPath is constructed from trusted directory traversal
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return content, nil
}

// modeFromFileInfo determines the object.Mode from os.FileInfo.
func modeFromFileInfo(info os.FileInfo) object.Mode {
	mode := info.Mode()

	if mode&os.ModeSymlink != 0 {
		return object.ModeSymlink
	}
	if mode.IsDir() {
		return object.ModeDirectory
	}
	if mode&0o111 != 0 {
		return object.ModeExecutable
	}
	return object.ModeRegular
}
