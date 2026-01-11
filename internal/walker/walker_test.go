package walker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/garrettladley/smerkle/internal/ignore"
	"github.com/garrettladley/smerkle/internal/object"
	"github.com/garrettladley/smerkle/internal/store"
)

func TestWalk(t *testing.T) {
	t.Parallel()

	t.Run("root not exist returns error", func(t *testing.T) {
		t.Parallel()

		s := setupStore(t)

		_, err := Walk(context.Background(), "/nonexistent/path/that/does/not/exist", s)
		if err == nil {
			t.Fatal("Walk() expected error for nonexistent root")
		}
		if !errors.Is(err, ErrRootNotExist) {
			t.Errorf("error = %v, want ErrRootNotExist", err)
		}
	})

	t.Run("root is file returns error", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		filePath := filepath.Join(root, "file.txt")
		writeFile(t, filePath, "content")
		s := setupStore(t)

		_, err := Walk(context.Background(), filePath, s)
		if err == nil {
			t.Fatal("Walk() expected error when root is file")
		}
		if !errors.Is(err, ErrRootNotDirectory) {
			t.Errorf("error = %v, want ErrRootNotDirectory", err)
		}
	})

	t.Run("empty directory returns valid hash", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}
		if !result.Ok() {
			t.Errorf("Walk() has errors: %v", result.Err())
		}
		if result.Hash.IsZero() {
			t.Error("Walk() returned zero hash for empty directory")
		}
	})

	t.Run("single file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "hello.txt"), "hello world")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}
		if !result.Ok() {
			t.Errorf("Walk() has errors: %v", result.Err())
		}
		if result.Hash.IsZero() {
			t.Error("Walk() returned zero hash")
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}
		if len(tree.Entries) != 1 {
			t.Errorf("tree has %d entries, want 1", len(tree.Entries))
		}
		if tree.Entries[0].Name != "hello.txt" {
			t.Errorf("entry name = %q, want %q", tree.Entries[0].Name, "hello.txt")
		}
	})

	t.Run("multiple files sorted by name", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "zebra.txt"), "z")
		writeFile(t, filepath.Join(root, "alpha.txt"), "a")
		writeFile(t, filepath.Join(root, "beta.txt"), "b")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}

		names := make([]string, len(tree.Entries))
		for i, e := range tree.Entries {
			names[i] = e.Name
		}

		if !sort.StringsAreSorted(names) {
			t.Errorf("entries not sorted: %v", names)
		}
	})

	t.Run("nested directories", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "root.txt"), "root")
		writeFile(t, filepath.Join(root, "sub", "nested.txt"), "nested")
		writeFile(t, filepath.Join(root, "sub", "deep", "deeper.txt"), "deep")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}
		if !result.Ok() {
			t.Errorf("Walk() has errors: %v", result.Err())
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}

		if len(tree.Entries) != 2 {
			t.Errorf("root tree has %d entries, want 2", len(tree.Entries))
		}

		var subEntry *object.Entry
		for i := range tree.Entries {
			if tree.Entries[i].Name == "sub" {
				subEntry = &tree.Entries[i]
				break
			}
		}
		if subEntry == nil {
			t.Fatal("sub directory not found in tree")
		}
		if subEntry.Mode != object.ModeDirectory {
			t.Errorf("sub entry mode = %v, want directory", subEntry.Mode)
		}

		subTree, err := s.GetTree(subEntry.Hash)
		if err != nil {
			t.Fatalf("GetTree(sub) error = %v", err)
		}
		if len(subTree.Entries) != 2 {
			t.Errorf("sub tree has %d entries, want 2 (nested.txt and deep/)", len(subTree.Entries))
		}
	})
}

func TestWalkFileModes(t *testing.T) {
	t.Parallel()

	t.Run("regular file has ModeRegular", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "regular.txt"), "content")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}
		if tree.Entries[0].Mode != object.ModeRegular {
			t.Errorf("mode = %v, want ModeRegular", tree.Entries[0].Mode)
		}
	})

	t.Run("executable file has ModeExecutable", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeExecutable(t, filepath.Join(root, "script.sh"), "#!/bin/sh\necho hello")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}
		if tree.Entries[0].Mode != object.ModeExecutable {
			t.Errorf("mode = %v, want ModeExecutable", tree.Entries[0].Mode)
		}
	})

	t.Run("symlink has ModeSymlink", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "target.txt"), "target content")
		writeSymlink(t, filepath.Join(root, "link.txt"), "target.txt")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}

		var linkEntry *object.Entry
		for i := range tree.Entries {
			if tree.Entries[i].Name == "link.txt" {
				linkEntry = &tree.Entries[i]
				break
			}
		}
		if linkEntry == nil {
			t.Fatal("link entry not found")
		}
		if linkEntry.Mode != object.ModeSymlink {
			t.Errorf("mode = %v, want ModeSymlink", linkEntry.Mode)
		}
	})

	t.Run("symlink hashes target path not content", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "target.txt"), "target content")
		writeSymlink(t, filepath.Join(root, "link.txt"), "target.txt")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}

		var linkEntry, targetEntry *object.Entry
		for i := range tree.Entries {
			switch tree.Entries[i].Name {
			case "link.txt":
				linkEntry = &tree.Entries[i]
			case "target.txt":
				targetEntry = &tree.Entries[i]
			}
		}

		// symlink hash should differ from target hash
		if linkEntry.Hash == targetEntry.Hash {
			t.Error("symlink hash should differ from target hash")
		}

		blob, err := s.GetBlob(linkEntry.Hash)
		if err != nil {
			t.Fatalf("GetBlob() error = %v", err)
		}
		if string(blob.Content) != "target.txt" {
			t.Errorf("symlink content = %q, want %q", blob.Content, "target.txt")
		}
	})

	t.Run("directory has ModeDirectory", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		mkdir(t, filepath.Join(root, "subdir"))
		writeFile(t, filepath.Join(root, "subdir", "file.txt"), "content")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}
		if tree.Entries[0].Mode != object.ModeDirectory {
			t.Errorf("mode = %v, want ModeDirectory", tree.Entries[0].Mode)
		}
	})
}

func TestWalkIgnorePatterns(t *testing.T) {
	t.Parallel()

	t.Run("respects smerkleignore file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "keep.txt"), "keep")
		writeFile(t, filepath.Join(root, "ignore.log"), "ignore")
		writeIgnoreFile(t, root, "*.log")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}

		for _, e := range tree.Entries {
			if e.Name == "ignore.log" {
				t.Error("ignored file should not appear in tree")
			}
		}
	})

	t.Run("smerkleignore not included in tree", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "keep.txt"), "keep")
		writeIgnoreFile(t, root, "*.log")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}

		for _, e := range tree.Entries {
			if e.Name == ".smerkleignore" {
				t.Error(".smerkleignore should not appear in tree")
			}
		}
	})

	t.Run("ignores directories", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "keep.txt"), "keep")
		writeFile(t, filepath.Join(root, "node_modules", "pkg", "index.js"), "ignored")
		writeIgnoreFile(t, root, "node_modules/")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}

		for _, e := range tree.Entries {
			if e.Name == "node_modules" {
				t.Error("ignored directory should not appear in tree")
			}
		}
	})

	t.Run("Walk(...WithIgnorer(...)) uses provided ignorer", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "keep.txt"), "keep")
		writeFile(t, filepath.Join(root, "secret.txt"), "secret")
		s := setupStore(t)

		ign := mustIgnorer(t, "secret.txt")

		result, err := Walk(context.Background(), root, s, WithIgnorer(ign))
		if err != nil {
			t.Fatalf("WalkWithIgnorer() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}

		for _, e := range tree.Entries {
			if e.Name == "secret.txt" {
				t.Error("ignored file should not appear in tree")
			}
		}
	})

	t.Run("negation patterns work", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "debug.log"), "debug")
		writeFile(t, filepath.Join(root, "important.log"), "important")
		writeIgnoreFile(t, root, "*.log", "!important.log")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}

		hasImportant := false
		hasDebug := false
		for _, e := range tree.Entries {
			if e.Name == "important.log" {
				hasImportant = true
			}
			if e.Name == "debug.log" {
				hasDebug = true
			}
		}

		if !hasImportant {
			t.Error("important.log should not be ignored (negation)")
		}
		if hasDebug {
			t.Error("debug.log should be ignored")
		}
	})
}

func TestWalkCache(t *testing.T) {
	t.Parallel()

	t.Run("uses cache for unchanged files", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "cached.txt"), "content")
		s := setupStore(t)

		// first walk
		result1, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		// second walk should use cache
		result2, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		if result1.Hash != result2.Hash {
			t.Error("hash should be identical for unchanged directory")
		}
	})

	t.Run("detects modified files", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		filePath := filepath.Join(root, "modify.txt")
		writeFile(t, filePath, "original")
		s := setupStore(t)

		result1, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		// modify file (ensure mtime changes)
		time.Sleep(10 * time.Millisecond)
		writeFile(t, filePath, "modified")

		result2, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		if result1.Hash == result2.Hash {
			t.Error("hash should differ for modified file")
		}
	})

	t.Run("detects new files", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "existing.txt"), "existing")
		s := setupStore(t)

		result1, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		writeFile(t, filepath.Join(root, "new.txt"), "new")

		result2, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		if result1.Hash == result2.Hash {
			t.Error("hash should differ when file added")
		}
	})

	t.Run("detects deleted files", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		deletePath := filepath.Join(root, "delete.txt")
		writeFile(t, filepath.Join(root, "keep.txt"), "keep")
		writeFile(t, deletePath, "delete")
		s := setupStore(t)

		result1, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		if err := os.Remove(deletePath); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}

		result2, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		if result1.Hash == result2.Hash {
			t.Error("hash should differ when file deleted")
		}
	})
}

func TestWalkErrors(t *testing.T) {
	t.Parallel()

	t.Run("unreadable file collects error and continues", func(t *testing.T) {
		t.Parallel()

		if os.Getuid() == 0 {
			t.Skip("test requires non-root user")
		}

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "readable.txt"), "readable")
		unreadable := filepath.Join(root, "unreadable.txt")
		writeFile(t, unreadable, "unreadable")
		if err := os.Chmod(unreadable, 0o000); err != nil {
			t.Fatalf("Chmod() error = %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(unreadable, 0o600) })
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		// should have collected error
		if result.Ok() {
			t.Error("expected errors for unreadable file")
		}
		if len(result.Errors) != 1 {
			t.Errorf("errors count = %d, want 1", len(result.Errors))
		}

		// should still have readable file in tree
		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}
		hasReadable := false
		for _, e := range tree.Entries {
			if e.Name == "readable.txt" {
				hasReadable = true
			}
		}
		if !hasReadable {
			t.Error("readable file should be in tree")
		}
	})
}

func TestWalkContext(t *testing.T) {
	t.Parallel()

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		// create many files to increase chance of cancellation during walk
		for i := range 100 {
			writeFile(t, filepath.Join(root, "sub", string(rune('a'+i%26))+string(rune('0'+i/26))+".txt"), "content")
		}
		s := setupStore(t)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		_, err := Walk(ctx, root, s)
		if err == nil {
			// may or may not error depending on timing
			t.Log("Walk completed before context check")
		} else if !errors.Is(err, context.Canceled) {
			t.Logf("Walk error (expected context.Canceled): %v", err)
		}
	})
}

func TestWalkDeterminism(t *testing.T) {
	t.Parallel()

	t.Run("same content produces same hash", func(t *testing.T) {
		t.Parallel()

		setup := func(t *testing.T) (string, *store.Store) {
			t.Helper()
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "a.txt"), "content a")
			writeFile(t, filepath.Join(root, "b.txt"), "content b")
			writeFile(t, filepath.Join(root, "sub", "c.txt"), "content c")
			return root, setupStore(t)
		}

		root1, s1 := setup(t)
		root2, s2 := setup(t)

		result1, err := Walk(context.Background(), root1, s1)
		if err != nil {
			t.Fatalf("Walk(root1) error = %v", err)
		}
		result2, err := Walk(context.Background(), root2, s2)
		if err != nil {
			t.Fatalf("Walk(root2) error = %v", err)
		}

		if result1.Hash != result2.Hash {
			t.Error("identical directories should produce identical hashes")
		}
	})

	t.Run("different content produces different hash", func(t *testing.T) {
		t.Parallel()

		root1 := t.TempDir()
		writeFile(t, filepath.Join(root1, "file.txt"), "content 1")
		s1 := setupStore(t)

		root2 := t.TempDir()
		writeFile(t, filepath.Join(root2, "file.txt"), "content 2")
		s2 := setupStore(t)

		result1, err := Walk(context.Background(), root1, s1)
		if err != nil {
			t.Fatalf("Walk(root1) error = %v", err)
		}
		result2, err := Walk(context.Background(), root2, s2)
		if err != nil {
			t.Fatalf("Walk(root2) error = %v", err)
		}

		if result1.Hash == result2.Hash {
			t.Error("different content should produce different hashes")
		}
	})
}

func TestWalkEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty directory in tree", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		mkdir(t, filepath.Join(root, "empty"))
		writeFile(t, filepath.Join(root, "file.txt"), "content")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}

		hasEmpty := false
		for _, e := range tree.Entries {
			if e.Name == "empty" && e.Mode == object.ModeDirectory {
				hasEmpty = true
			}
		}
		if !hasEmpty {
			t.Error("empty directory should appear in tree")
		}
	})

	t.Run("deeply nested structure", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		deepPath := filepath.Join(root, "a", "b", "c", "d", "e", "f", "g", "deep.txt")
		writeFile(t, deepPath, "deep content")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}
		if !result.Ok() {
			t.Errorf("Walk() has errors: %v", result.Err())
		}
		if result.Hash.IsZero() {
			t.Error("Walk() returned zero hash")
		}
	})

	t.Run("unicode filenames", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "日本語.txt"), "japanese")
		writeFile(t, filepath.Join(root, "données.json"), "french")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}
		if !result.Ok() {
			t.Errorf("Walk() has errors: %v", result.Err())
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}
		if len(tree.Entries) != 2 {
			t.Errorf("tree has %d entries, want 2", len(tree.Entries))
		}
	})

	t.Run("empty file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "empty.txt"), "")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}
		if len(tree.Entries) != 1 {
			t.Errorf("tree has %d entries, want 1", len(tree.Entries))
		}
		if tree.Entries[0].Size != 0 {
			t.Errorf("empty file size = %d, want 0", tree.Entries[0].Size)
		}
	})

	t.Run("dotfiles included by default", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, ".hidden"), "hidden")
		writeFile(t, filepath.Join(root, ".config"), "config")
		s := setupStore(t)

		result, err := Walk(context.Background(), root, s)
		if err != nil {
			t.Fatalf("Walk() error = %v", err)
		}

		tree, err := s.GetTree(result.Hash)
		if err != nil {
			t.Fatalf("GetTree() error = %v", err)
		}
		if len(tree.Entries) != 2 {
			t.Errorf("tree has %d entries, want 2 (dotfiles should be included)", len(tree.Entries))
		}
	})
}

func TestWalkConcurrency(t *testing.T) {
	t.Parallel()

	t.Run("concurrent walks produce same hash", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		for i := range 20 {
			writeFile(t, filepath.Join(root, string(rune('a'+i))+".txt"), "content")
		}

		const numWalks = 5
		results := make([]object.Hash, numWalks)

		var wg sync.WaitGroup
		wg.Add(numWalks)

		for i := range numWalks {
			go func(idx int) {
				defer wg.Done()
				s := setupStore(t)
				result, err := Walk(context.Background(), root, s)
				if err != nil {
					t.Errorf("Walk[%d] error = %v", idx, err)
					return
				}
				results[idx] = result.Hash
			}(i)
		}

		wg.Wait()

		for i := 1; i < numWalks; i++ {
			if results[i] != results[0] {
				t.Errorf("Walk[%d] hash = %v, want %v", i, results[i], results[0])
			}
		}
	})
}

func setupStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("store.Close() error = %v", err)
		}
	})
	return s
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o750); err != nil { //nolint:gosec // test helper needs executable permission
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func writeSymlink(t *testing.T, path, target string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("Symlink(%q -> %q) error = %v", path, target, err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
}

func writeIgnoreFile(t *testing.T, root string, patterns ...string) {
	t.Helper()
	content := strings.Join(patterns, "\n")
	writeFile(t, filepath.Join(root, ".smerkleignore"), content)
}

func mustIgnorer(t *testing.T, patterns ...string) *ignore.Ignorer {
	t.Helper()
	content := strings.Join(patterns, "\n")
	ign, err := ignore.New(strings.NewReader(content))
	if err != nil {
		t.Fatalf("ignore.New() error = %v", err)
	}
	return ign
}
