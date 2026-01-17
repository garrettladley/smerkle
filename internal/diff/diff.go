package diff

import (
	"fmt"
	"path"

	"github.com/garrettladley/smerkle/internal/object"
	"github.com/garrettladley/smerkle/internal/store"
)

type ChangeType uint8

var _ fmt.Stringer = ChangeType(0)

const (
	ChangeAdded      ChangeType = iota // entry only in new tree
	ChangeDeleted                      // entry only in old tree
	ChangeModified                     // same name, different hash
	ChangeTypeChange                   // file <-> directory change
)

func (c ChangeType) String() string {
	switch c {
	case ChangeAdded:
		return "added"
	case ChangeDeleted:
		return "deleted"
	case ChangeModified:
		return "modified"
	case ChangeTypeChange:
		return "type_change"
	default:
		return "unknown"
	}
}

type Change struct {
	Type     ChangeType
	Path     string        // e.g., "internal/client/whoop/client.go"
	OldEntry *object.Entry // nil for added
	NewEntry *object.Entry // nil for deleted
}

type Result struct {
	Changes []Change
}

func (r *Result) HasChanges() bool {
	return len(r.Changes) > 0
}

func (r *Result) Added() []Change {
	return r.filterByType(ChangeAdded)
}

func (r *Result) Deleted() []Change {
	return r.filterByType(ChangeDeleted)
}

func (r *Result) Modified() []Change {
	return r.filterByType(ChangeModified)
}

func (r *Result) TypeChanges() []Change {
	return r.filterByType(ChangeTypeChange)
}

func (r *Result) filterByType(t ChangeType) []Change {
	var out []Change
	for _, c := range r.Changes {
		if c.Type == t {
			out = append(out, c)
		}
	}
	return out
}

type Options struct {
	Recursive bool // default: true
}

func DiffDefault(s *store.Store, oldHash, newHash object.Hash) (*Result, error) {
	return Diff(s, oldHash, newHash, Options{Recursive: true})
}

func Diff(s *store.Store, oldHash, newHash object.Hash, opts Options) (*Result, error) {
	result := &Result{}

	if err := diffTrees(s, oldHash, newHash, "", opts, result); err != nil {
		return nil, err
	}

	return result, nil
}

func diffTrees(s *store.Store, oldHash, newHash object.Hash, prefix string, opts Options, result *Result) error {
	if oldHash == newHash {
		return nil
	}

	oldTree, err := loadTree(s, oldHash)
	if err != nil {
		return err
	}

	newTree, err := loadTree(s, newHash)
	if err != nil {
		return err
	}

	oldIdx, newIdx := 0, 0

	for oldIdx < len(oldTree.Entries) || newIdx < len(newTree.Entries) {
		var oldEntry, newEntry *object.Entry

		if oldIdx < len(oldTree.Entries) {
			oldEntry = &oldTree.Entries[oldIdx]
		}
		if newIdx < len(newTree.Entries) {
			newEntry = &newTree.Entries[newIdx]
		}

		switch {
		case oldEntry == nil:
			fullPath := joinPath(prefix, newEntry.Name)
			result.Changes = append(result.Changes, Change{
				Type:     ChangeAdded,
				Path:     fullPath,
				NewEntry: newEntry,
			})
			if opts.Recursive && newEntry.Mode == object.ModeDirectory {
				if err := addAllEntries(s, newEntry.Hash, fullPath, ChangeAdded, result); err != nil {
					return err
				}
			}
			newIdx++

		case newEntry == nil:
			fullPath := joinPath(prefix, oldEntry.Name)
			result.Changes = append(result.Changes, Change{
				Type:     ChangeDeleted,
				Path:     fullPath,
				OldEntry: oldEntry,
			})
			if opts.Recursive && oldEntry.Mode == object.ModeDirectory {
				if err := addAllEntries(s, oldEntry.Hash, fullPath, ChangeDeleted, result); err != nil {
					return err
				}
			}
			oldIdx++

		case oldEntry.Name < newEntry.Name:
			fullPath := joinPath(prefix, oldEntry.Name)
			result.Changes = append(result.Changes, Change{
				Type:     ChangeDeleted,
				Path:     fullPath,
				OldEntry: oldEntry,
			})
			if opts.Recursive && oldEntry.Mode == object.ModeDirectory {
				if err := addAllEntries(s, oldEntry.Hash, fullPath, ChangeDeleted, result); err != nil {
					return err
				}
			}
			oldIdx++

		case oldEntry.Name > newEntry.Name:
			fullPath := joinPath(prefix, newEntry.Name)
			result.Changes = append(result.Changes, Change{
				Type:     ChangeAdded,
				Path:     fullPath,
				NewEntry: newEntry,
			})
			if opts.Recursive && newEntry.Mode == object.ModeDirectory {
				if err := addAllEntries(s, newEntry.Hash, fullPath, ChangeAdded, result); err != nil {
					return err
				}
			}
			newIdx++

		default:
			if err := diffEntry(s, oldEntry, newEntry, prefix, opts, result); err != nil {
				return err
			}
			oldIdx++
			newIdx++
		}
	}

	return nil
}

func diffEntry(s *store.Store, oldEntry, newEntry *object.Entry, prefix string, opts Options, result *Result) error {
	fullPath := joinPath(prefix, oldEntry.Name)
	oldIsDir := oldEntry.Mode == object.ModeDirectory
	newIsDir := newEntry.Mode == object.ModeDirectory

	if oldIsDir != newIsDir {
		return handleTypeChange(s, oldEntry, newEntry, fullPath, oldIsDir, newIsDir, opts, result)
	}

	if oldEntry.Hash == newEntry.Hash {
		return nil
	}

	if oldIsDir && opts.Recursive {
		return diffTrees(s, oldEntry.Hash, newEntry.Hash, fullPath, opts, result)
	}

	result.Changes = append(result.Changes, Change{
		Type:     ChangeModified,
		Path:     fullPath,
		OldEntry: oldEntry,
		NewEntry: newEntry,
	})
	return nil
}

func handleTypeChange(s *store.Store, oldEntry, newEntry *object.Entry, fullPath string, oldIsDir, newIsDir bool, opts Options, result *Result) error {
	result.Changes = append(result.Changes, Change{
		Type:     ChangeTypeChange,
		Path:     fullPath,
		OldEntry: oldEntry,
		NewEntry: newEntry,
	})

	if opts.Recursive && oldIsDir {
		if err := addAllEntries(s, oldEntry.Hash, fullPath, ChangeDeleted, result); err != nil {
			return err
		}
	}

	if opts.Recursive && newIsDir {
		if err := addAllEntries(s, newEntry.Hash, fullPath, ChangeAdded, result); err != nil {
			return err
		}
	}

	return nil
}

func loadTree(s *store.Store, hash object.Hash) (*object.Tree, error) {
	if hash.IsZero() {
		return &object.Tree{Entries: []object.Entry{}}, nil
	}
	tree, err := s.GetTree(hash)
	if err != nil {
		return nil, fmt.Errorf("get tree %s: %w", hash, err)
	}
	return tree, nil
}

func addAllEntries(s *store.Store, hash object.Hash, prefix string, changeType ChangeType, result *Result) error {
	tree, err := loadTree(s, hash)
	if err != nil {
		return err
	}

	for i := range tree.Entries {
		entry := &tree.Entries[i]
		fullPath := joinPath(prefix, entry.Name)

		change := Change{
			Path: fullPath,
			Type: changeType,
		}
		if changeType == ChangeAdded {
			change.NewEntry = entry
		} else {
			change.OldEntry = entry
		}
		result.Changes = append(result.Changes, change)

		if entry.Mode == object.ModeDirectory {
			if err := addAllEntries(s, entry.Hash, fullPath, changeType, result); err != nil {
				return err
			}
		}
	}

	return nil
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return path.Join(prefix, name)
}
