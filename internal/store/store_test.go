package store

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/garrettladley/smerkle/internal/object"
)

func TestOpen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
	}{
		{
			name: "empty directory creates objects dir",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantErr: false,
		},
		{
			name: "existing directory with valid index file",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				// create objects directory
				if err := os.MkdirAll(filepath.Join(dir, objectsDir), 0o755); err != nil {
					t.Fatalf("failed to create objects dir: %v", err)
				}
				// create a valid index file
				now := time.Now().Truncate(time.Nanosecond)
				hash := object.HashBytes([]byte("test"))
				index := &object.Index{
					Entries: []object.IndexEntry{
						{Path: "test.txt", Size: 100, ModTime: now, Hash: hash},
					},
				}
				data, err := object.EncodeIndex(index)
				if err != nil {
					t.Fatalf("failed to encode index: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, indexFile), data, 0o644); err != nil {
					t.Fatalf("failed to write index: %v", err)
				}
				return dir
			},
			wantErr: false,
		},
		{
			name: "existing directory with corrupted index",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				// create objects directory
				if err := os.MkdirAll(filepath.Join(dir, objectsDir), 0o755); err != nil {
					t.Fatalf("failed to create objects dir: %v", err)
				}
				// write corrupted index file
				corrupted := []byte("CORRUPTED DATA")
				if err := os.WriteFile(filepath.Join(dir, indexFile), corrupted, 0o644); err != nil {
					t.Fatalf("failed to write corrupted index: %v", err)
				}
				return dir
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := tt.setup(t)

			store, err := Open(root)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Open() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// verify store was initialized
			if store == nil {
				t.Fatal("Open() returned nil store")
			}

			// verify objects directory exists
			objectsPath := filepath.Join(root, objectsDir)
			info, err := os.Stat(objectsPath)
			if err != nil {
				t.Fatalf("objects directory not created: %v", err)
			}
			if !info.IsDir() {
				t.Error("objects path is not a directory")
			}

			// verify index was loaded for the valid index case
			if tt.name == "existing directory with valid index file" {
				// check that the index file was loaded by verifying the index is not empty
				stats := store.Stats()
				if stats.IndexSize != 1 {
					t.Errorf("index size = %d, want 1 (index was not loaded)", stats.IndexSize)
				}
			}

			// clean up
			if err := store.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})
	}
}

func TestObjectPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hash     object.Hash
		wantPath string
	}{
		{
			name:     "standard hash",
			hash:     object.HashBytes([]byte("test content")),
			wantPath: "", // will be computed
		},
		{
			name:     "zero hash",
			hash:     object.ZeroHash,
			wantPath: "", // will be computed
		},
		{
			name:     "different content",
			hash:     object.HashBytes([]byte("different")),
			wantPath: "", // will be computed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			store, err := Open(dir)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer store.Close()

			path := store.objectPath(tt.hash)

			// verify format is objects/XX/YYYYYY... where XX is first 2 hex chars
			hex := tt.hash.String()
			expectedDir := hex[:2]
			expectedFile := hex[2:]
			expectedPath := filepath.Join(dir, objectsDir, expectedDir, expectedFile)

			if path != expectedPath {
				t.Errorf("objectPath() = %q, want %q", path, expectedPath)
			}

			// verify reproducibility - calling again should return same path
			path2 := store.objectPath(tt.hash)
			if path != path2 {
				t.Errorf("objectPath not reproducible: first = %q, second = %q", path, path2)
			}
		})
	}
}

func TestObjectStorage(t *testing.T) {
	t.Parallel()

	t.Run("PutObject creates file atomically", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		store, err := Open(dir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()

		hash := object.HashBytes([]byte("test"))
		data := []byte("test content")

		if err := store.PutObject(hash, data); err != nil {
			t.Fatalf("PutObject() error = %v", err)
		}

		// verify file exists at the correct path
		path := store.objectPath(hash)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("object file not created: %v", err)
		}
		if info.IsDir() {
			t.Error("object path is a directory, expected file")
		}

		// verify no .tmp file remains
		tmpPath := path + ".tmp"
		if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
			t.Error("temporary file was not cleaned up")
		}
	})

	t.Run("PutObject creates parent directories", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		store, err := Open(dir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()

		hash := object.HashBytes([]byte("test"))
		data := []byte("test content")

		// delete the shard directory if it exists to test creation
		hex := hash.String()
		shardDir := filepath.Join(dir, objectsDir, hex[:2])
		os.RemoveAll(shardDir)

		if err := store.PutObject(hash, data); err != nil {
			t.Fatalf("PutObject() error = %v", err)
		}

		// verify shard directory was created
		info, err := os.Stat(shardDir)
		if err != nil {
			t.Fatalf("shard directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Error("shard path is not a directory")
		}
	})

	t.Run("GetObject retrieves content correctly", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		store, err := Open(dir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()

		hash := object.HashBytes([]byte("test"))
		expected := []byte("test content")

		if err := store.PutObject(hash, expected); err != nil {
			t.Fatalf("PutObject() error = %v", err)
		}

		got, err := store.GetObject(hash)
		if err != nil {
			t.Fatalf("GetObject() error = %v", err)
		}

		if !bytes.Equal(got, expected) {
			t.Errorf("GetObject() = %q, want %q", got, expected)
		}
	})

	t.Run("GetObject returns error for missing objects", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		store, err := Open(dir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()

		// use a hash that doesn't exist
		hash := object.HashBytes([]byte("nonexistent"))

		_, err = store.GetObject(hash)
		if err == nil {
			t.Fatal("GetObject() expected error for missing object, got nil")
		}
		if !os.IsNotExist(err) {
			t.Errorf("GetObject() error = %v, want IsNotExist error", err)
		}
	})

	t.Run("HasObject returns true for existing, false for missing", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		store, err := Open(dir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()

		existingHash := object.HashBytes([]byte("exists"))
		missingHash := object.HashBytes([]byte("missing"))

		// put one object
		if err := store.PutObject(existingHash, []byte("content")); err != nil {
			t.Fatalf("PutObject() error = %v", err)
		}

		// test existing object
		if !store.HasObject(existingHash) {
			t.Error("HasObject() = false for existing object, want true")
		}

		// test missing object
		if store.HasObject(missingHash) {
			t.Error("HasObject() = true for missing object, want false")
		}
	})

	t.Run("round-trip: put then get returns same content", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			content []byte
		}{
			{
				name:    "empty content",
				content: []byte{},
			},
			{
				name:    "simple text",
				content: []byte("hello world"),
			},
			{
				name:    "binary content",
				content: []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd},
			},
			{
				name:    "large content",
				content: bytes.Repeat([]byte("x"), 1<<16),
			},
			{
				name:    "unicode content",
				content: []byte("こんにちは世界"),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				store, err := Open(dir)
				if err != nil {
					t.Fatalf("Open() error = %v", err)
				}
				defer store.Close()

				hash := object.HashBytes(tt.content)

				// put the content
				if err := store.PutObject(hash, tt.content); err != nil {
					t.Fatalf("PutObject() error = %v", err)
				}

				// get it back
				got, err := store.GetObject(hash)
				if err != nil {
					t.Fatalf("GetObject() error = %v", err)
				}

				// verify content matches
				if !bytes.Equal(got, tt.content) {
					t.Errorf("round-trip failed: got %d bytes, want %d bytes", len(got), len(tt.content))
				}
			})
		}
	})
}

func TestBlobStorage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content []byte
	}{
		{
			name:    "empty content",
			content: []byte{},
		},
		{
			name:    "simple text",
			content: []byte("hello world"),
		},
		{
			name:    "binary content",
			content: []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd},
		},
		{
			name:    "large content",
			content: bytes.Repeat([]byte("x"), 1<<16),
		},
		{
			name:    "unicode content",
			content: []byte("こんにちは世界 🌍"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer s.Close()

			blob := &object.Blob{Content: tt.content}

			// test: PutBlob stores content and returns hash
			hash1, err := s.PutBlob(blob)
			if err != nil {
				t.Fatalf("PutBlob() error = %v", err)
			}
			if hash1.IsZero() {
				t.Error("PutBlob() returned zero hash")
			}

			// verify the hash matches the blob's hash
			expectedHash := blob.Hash()
			if hash1 != expectedHash {
				t.Errorf("PutBlob() hash = %v, want %v", hash1, expectedHash)
			}

			// test: PutBlob deduplicates (returns same hash, doesn't write twice)
			hash2, err := s.PutBlob(blob)
			if err != nil {
				t.Fatalf("PutBlob() second call error = %v", err)
			}
			if hash2 != hash1 {
				t.Errorf("PutBlob() deduplication failed: got %v, want %v", hash2, hash1)
			}

			// verify object was not written twice (stats should show 1 object)
			stats := s.Stats()
			if stats.ObjectCount != 1 {
				t.Errorf("PutBlob() created %d objects, want 1 (deduplication failed)", stats.ObjectCount)
			}

			// test: GetBlob retrieves and decodes correctly
			retrieved, err := s.GetBlob(hash1)
			if err != nil {
				t.Fatalf("GetBlob() error = %v", err)
			}

			// test: Round-trip verification
			if !bytes.Equal(retrieved.Content, tt.content) {
				t.Errorf("GetBlob() content mismatch: got %v, want %v", retrieved.Content, tt.content)
			}
		})
	}

	// test: GetBlob returns error for missing blob
	t.Run("missing blob", func(t *testing.T) {
		t.Parallel()

		s, err := Open(t.TempDir())
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer s.Close()

		// create a hash that doesn't exist
		missingHash := object.HashBytes([]byte("nonexistent"))

		_, err = s.GetBlob(missingHash)
		if err == nil {
			t.Error("GetBlob() expected error for missing blob, got nil")
		}
	})
}

func TestTreeStorage(t *testing.T) {
	t.Parallel()

	hash1 := object.HashBytes([]byte("file1"))
	hash2 := object.HashBytes([]byte("file2"))
	hash3 := object.HashBytes([]byte("dir"))

	tests := []struct {
		name    string
		entries []object.Entry
	}{
		{
			name:    "empty tree",
			entries: []object.Entry{},
		},
		{
			name: "single file entry",
			entries: []object.Entry{
				{Name: "file.txt", Mode: object.ModeRegular, Size: 100, Hash: hash1},
			},
		},
		{
			name: "multiple entries with different modes",
			entries: []object.Entry{
				{Name: "readme.md", Mode: object.ModeRegular, Size: 1024, Hash: hash1},
				{Name: "script.sh", Mode: object.ModeExecutable, Size: 512, Hash: hash2},
				{Name: "subdir", Mode: object.ModeDirectory, Size: 0, Hash: hash3},
			},
		},
		{
			name: "unicode filenames",
			entries: []object.Entry{
				{Name: "文件.txt", Mode: object.ModeRegular, Size: 42, Hash: hash1},
				{Name: "données.json", Mode: object.ModeRegular, Size: 256, Hash: hash2},
			},
		},
		{
			name: "symlink entry",
			entries: []object.Entry{
				{Name: "link", Mode: object.ModeSymlink, Size: 10, Hash: hash1},
			},
		},
		{
			name: "zero hash",
			entries: []object.Entry{
				{Name: "empty", Mode: object.ModeRegular, Size: 0, Hash: object.ZeroHash},
			},
		},
		{
			name: "large tree with many entries",
			entries: []object.Entry{
				{Name: "file1.txt", Mode: object.ModeRegular, Size: 100, Hash: hash1},
				{Name: "file2.txt", Mode: object.ModeRegular, Size: 200, Hash: hash2},
				{Name: "file3.txt", Mode: object.ModeExecutable, Size: 300, Hash: hash3},
				{Name: "dir1", Mode: object.ModeDirectory, Size: 0, Hash: hash1},
				{Name: "dir2", Mode: object.ModeDirectory, Size: 0, Hash: hash2},
				{Name: "link1", Mode: object.ModeSymlink, Size: 50, Hash: hash3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer s.Close()

			tree := &object.Tree{Entries: tt.entries}

			// test: PutTree stores tree and returns hash
			hash1, err := s.PutTree(tree)
			if err != nil {
				t.Fatalf("PutTree() error = %v", err)
			}
			if hash1.IsZero() {
				t.Error("PutTree() returned zero hash")
			}

			// test: PutTree deduplicates
			hash2, err := s.PutTree(tree)
			if err != nil {
				t.Fatalf("PutTree() second call error = %v", err)
			}
			if hash2 != hash1 {
				t.Errorf("PutTree() deduplication failed: got %v, want %v", hash2, hash1)
			}

			// verify object was not written twice
			stats := s.Stats()
			if stats.ObjectCount != 1 {
				t.Errorf("PutTree() created %d objects, want 1 (deduplication failed)", stats.ObjectCount)
			}

			// test: GetTree retrieves and decodes correctly
			retrieved, err := s.GetTree(hash1)
			if err != nil {
				t.Fatalf("GetTree() error = %v", err)
			}

			// test: Round-trip with multiple tree entries
			if len(retrieved.Entries) != len(tt.entries) {
				t.Fatalf("GetTree() entry count = %d, want %d", len(retrieved.Entries), len(tt.entries))
			}

			// test: Verify tree entries are preserved correctly (name, mode, size, hash)
			for i, want := range tt.entries {
				got := retrieved.Entries[i]
				if got.Name != want.Name {
					t.Errorf("entry[%d].Name = %q, want %q", i, got.Name, want.Name)
				}
				if got.Mode != want.Mode {
					t.Errorf("entry[%d].Mode = %v, want %v", i, got.Mode, want.Mode)
				}
				if got.Size != want.Size {
					t.Errorf("entry[%d].Size = %d, want %d", i, got.Size, want.Size)
				}
				if got.Hash != want.Hash {
					t.Errorf("entry[%d].Hash = %v, want %v", i, got.Hash, want.Hash)
				}
			}
		})
	}
}

func TestConcurrency(t *testing.T) {
	t.Parallel()

	t.Run("concurrent UpdateCache calls", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		s, err := Open(tmpDir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer s.Close()

		const numGoroutines = 50
		const updatesPerGoroutine = 100

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		now := time.Now().Truncate(time.Nanosecond)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < updatesPerGoroutine; j++ {
					path := filepath.Join("file", string(rune('a'+id%26)), string(rune('0'+j%10)))
					hash := object.HashBytes([]byte{byte(id), byte(j)})
					s.UpdateCache(path, int64(j), now, hash)
				}
			}(i)
		}

		wg.Wait()

		// verify the index contains the expected number of unique paths
		s.indexMu.RLock()
		indexSize := len(s.index)
		s.indexMu.RUnlock()

		if indexSize == 0 {
			t.Error("index is empty after concurrent updates")
		}

		// verify dirty flag was set
		s.indexMu.RLock()
		dirty := s.dirty
		s.indexMu.RUnlock()

		if !dirty {
			t.Error("dirty flag not set after concurrent updates")
		}
	})

	t.Run("concurrent LookupCache calls", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		s, err := Open(tmpDir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer s.Close()

		// populate cache with test data
		now := time.Now().Truncate(time.Nanosecond)
		type cacheEntry struct {
			path string
			size int64
			hash object.Hash
		}
		entries := make([]cacheEntry, 100)
		for i := 0; i < 100; i++ {
			path := filepath.Join("dir", string(rune('a'+i%26)), string(rune('0'+i/26)))
			hash := object.HashBytes([]byte{byte(i)})
			size := int64(i)
			s.UpdateCache(path, size, now, hash)
			entries[i] = cacheEntry{path: path, size: size, hash: hash}
		}

		const numGoroutines = 50
		const lookupsPerGoroutine = 100

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < lookupsPerGoroutine; j++ {
					idx := (id + j) % 100
					entry := entries[idx]

					hash, ok := s.LookupCache(entry.path, entry.size, now)
					if !ok {
						t.Errorf("LookupCache() failed for existing path %q", entry.path)
						continue
					}
					if hash != entry.hash {
						t.Errorf("LookupCache() hash = %v, want %v", hash, entry.hash)
					}
				}
			}(i)
		}

		wg.Wait()
	})

	t.Run("mixed concurrent UpdateCache and LookupCache", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		s, err := Open(tmpDir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer s.Close()

		const numGoroutines = 30
		const operationsPerGoroutine = 100

		var wg sync.WaitGroup
		wg.Add(numGoroutines * 2) // half updaters, half readers

		now := time.Now().Truncate(time.Nanosecond)

		// updater goroutines
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < operationsPerGoroutine; j++ {
					path := filepath.Join("mixed", string(rune('a'+id%26)), "file.txt")
					hash := object.HashBytes([]byte{byte(id), byte(j)})
					s.UpdateCache(path, int64(j), now, hash)
				}
			}(i)
		}

		// reader goroutines
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < operationsPerGoroutine; j++ {
					path := filepath.Join("mixed", string(rune('a'+id%26)), "file.txt")
					// just verify no panic/race, result may vary
					s.LookupCache(path, int64(j), now)
				}
			}(i)
		}

		wg.Wait()

		// verify store is in consistent state
		s.indexMu.RLock()
		indexSize := len(s.index)
		s.indexMu.RUnlock()

		if indexSize == 0 {
			t.Error("index is empty after mixed concurrent operations")
		}
	})

	t.Run("concurrent PutBlob calls with deduplication", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		s, err := Open(tmpDir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer s.Close()

		const numGoroutines = 20
		const blobsPerGoroutine = 10

		// create identical blobs to test deduplication
		identicalContent := []byte("identical content for deduplication test")
		identicalBlob := &object.Blob{Content: identicalContent}
		expectedHash := identicalBlob.Hash()

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		hashes := make([]object.Hash, numGoroutines*blobsPerGoroutine)
		errors := make([]error, numGoroutines*blobsPerGoroutine)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < blobsPerGoroutine; j++ {
					idx := id*blobsPerGoroutine + j
					blob := &object.Blob{Content: identicalContent}
					hash, err := s.PutBlob(blob)
					hashes[idx] = hash
					errors[idx] = err
				}
			}(i)
		}

		wg.Wait()

		// verify all operations succeeded
		for i, err := range errors {
			if err != nil {
				t.Errorf("PutBlob[%d] error = %v", i, err)
			}
		}

		// verify all hashes are identical
		for i, hash := range hashes {
			if hash != expectedHash {
				t.Errorf("hash[%d] = %v, want %v", i, hash, expectedHash)
			}
		}

		// verify only one object file was created (deduplication worked)
		objectPath := s.objectPath(expectedHash)
		if _, err := os.Stat(objectPath); os.IsNotExist(err) {
			t.Error("expected object file does not exist")
		}

		// count total object files created
		objectCount := 0
		objectsRoot := filepath.Join(tmpDir, objectsDir)
		filepath.Walk(objectsRoot, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				objectCount++
			}
			return nil
		})

		if objectCount != 1 {
			t.Errorf("object count = %d, want 1 (deduplication should create only one file)", objectCount)
		}
	})

	t.Run("concurrent PutBlob with different content", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		s, err := Open(tmpDir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer s.Close()

		const numGoroutines = 20
		const blobsPerGoroutine = 5

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		type result struct {
			hash object.Hash
			err  error
		}
		results := make([]result, numGoroutines*blobsPerGoroutine)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < blobsPerGoroutine; j++ {
					idx := id*blobsPerGoroutine + j
					// create unique content for each blob
					content := []byte{byte(id), byte(j), byte(idx)}
					blob := &object.Blob{Content: content}
					hash, err := s.PutBlob(blob)
					results[idx] = result{hash: hash, err: err}
				}
			}(i)
		}

		wg.Wait()

		// verify all operations succeeded
		for i, r := range results {
			if r.err != nil {
				t.Errorf("PutBlob[%d] error = %v", i, r.err)
			}
			if r.hash.IsZero() {
				t.Errorf("PutBlob[%d] returned zero hash", i)
			}
		}

		// count unique hashes
		uniqueHashes := make(map[object.Hash]struct{})
		for _, r := range results {
			uniqueHashes[r.hash] = struct{}{}
		}

		expectedUnique := numGoroutines * blobsPerGoroutine
		if len(uniqueHashes) != expectedUnique {
			t.Errorf("unique hashes = %d, want %d", len(uniqueHashes), expectedUnique)
		}
	})

	t.Run("concurrent GetBlob while PutBlob writes", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		s, err := Open(tmpDir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer s.Close()

		// pre-populate with some blobs
		preloadedHashes := make([]object.Hash, 10)
		preloadedContent := make([][]byte, 10)
		for i := 0; i < 10; i++ {
			content := bytes.Repeat([]byte{byte(i)}, 100)
			preloadedContent[i] = content
			blob := &object.Blob{Content: content}
			hash, err := s.PutBlob(blob)
			if err != nil {
				t.Fatalf("preload PutBlob[%d] error = %v", i, err)
			}
			preloadedHashes[i] = hash
		}

		const numReaders = 15
		const numWriters = 15
		const operationsPerGoroutine = 50

		var wg sync.WaitGroup
		wg.Add(numReaders + numWriters)

		// reader goroutines
		for i := 0; i < numReaders; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < operationsPerGoroutine; j++ {
					idx := (id + j) % len(preloadedHashes)
					hash := preloadedHashes[idx]
					blob, err := s.GetBlob(hash)
					if err != nil {
						t.Errorf("GetBlob() error = %v", err)
						continue
					}
					if !bytes.Equal(blob.Content, preloadedContent[idx]) {
						t.Errorf("GetBlob() content mismatch")
					}
				}
			}(i)
		}

		// writer goroutines (writing new blobs)
		for i := 0; i < numWriters; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < operationsPerGoroutine; j++ {
					content := []byte{byte(id + 100), byte(j), byte(id ^ j)}
					blob := &object.Blob{Content: content}
					hash, err := s.PutBlob(blob)
					if err != nil {
						t.Errorf("PutBlob() error = %v", err)
						continue
					}
					// immediately try to read back what we wrote
					retrieved, err := s.GetBlob(hash)
					if err != nil {
						t.Errorf("GetBlob() after PutBlob error = %v", err)
						continue
					}
					if !bytes.Equal(retrieved.Content, content) {
						t.Errorf("GetBlob() after PutBlob content mismatch")
					}
				}
			}(i)
		}

		wg.Wait()
	})

	t.Run("concurrent cache operations with Flush", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		s, err := Open(tmpDir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer s.Close()

		const numUpdaters = 10
		const numFlushers = 5
		const numReaders = 10
		const operationsPerGoroutine = 20

		var wg sync.WaitGroup
		wg.Add(numUpdaters + numFlushers + numReaders)

		now := time.Now().Truncate(time.Nanosecond)

		// updater goroutines
		for i := 0; i < numUpdaters; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < operationsPerGoroutine; j++ {
					path := filepath.Join("flush", string(rune('a'+id)), "file.txt")
					hash := object.HashBytes([]byte{byte(id), byte(j)})
					s.UpdateCache(path, int64(j), now, hash)
				}
			}(i)
		}

		// flusher goroutines
		for i := 0; i < numFlushers; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < operationsPerGoroutine; j++ {
					if err := s.Flush(); err != nil {
						t.Errorf("Flush() error = %v", err)
					}
					// small sleep to allow updates to happen
					time.Sleep(time.Microsecond)
				}
			}(i)
		}

		// reader goroutines
		for i := 0; i < numReaders; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < operationsPerGoroutine; j++ {
					path := filepath.Join("flush", string(rune('a'+id)), "file.txt")
					s.LookupCache(path, int64(j), now)
				}
			}(i)
		}

		wg.Wait()

		// final flush to ensure all data is written
		if err := s.Flush(); err != nil {
			t.Errorf("final Flush() error = %v", err)
		}

		// verify index file was created
		indexPath := filepath.Join(tmpDir, indexFile)
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			t.Error("index file does not exist after Flush()")
		}
	})

	t.Run("concurrent Stats calls", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		s, err := Open(tmpDir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer s.Close()

		// populate with some data
		now := time.Now().Truncate(time.Nanosecond)
		for i := 0; i < 10; i++ {
			path := filepath.Join("stats", string(rune('a'+i)), "file.txt")
			hash := object.HashBytes([]byte{byte(i)})
			s.UpdateCache(path, int64(i), now, hash)

			blob := &object.Blob{Content: []byte{byte(i)}}
			s.PutBlob(blob)
		}

		const numGoroutines = 20
		const statsPerGoroutine = 50

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < statsPerGoroutine; j++ {
					stats := s.Stats()
					// just verify we got some stats back without panicking
					if stats.IndexSize < 0 {
						t.Error("Stats() returned negative IndexSize")
					}
					if stats.ObjectCount < 0 {
						t.Error("Stats() returned negative ObjectCount")
					}
				}
			}()
		}

		wg.Wait()
	})

	t.Run("concurrent operations across all methods", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		s, err := Open(tmpDir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer s.Close()

		const numGoroutines = 40
		const operationsPerGoroutine = 25

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		now := time.Now().Truncate(time.Nanosecond)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < operationsPerGoroutine; j++ {
					// perform a variety of operations
					switch (id + j) % 5 {
					case 0:
						// updateCache
						path := filepath.Join("all", string(rune('a'+id%26)), "file.txt")
						hash := object.HashBytes([]byte{byte(id), byte(j)})
						s.UpdateCache(path, int64(j), now, hash)
					case 1:
						// lookupCache
						path := filepath.Join("all", string(rune('a'+id%26)), "file.txt")
						s.LookupCache(path, int64(j), now)
					case 2:
						// putBlob
						content := []byte{byte(id), byte(j), byte(id ^ j)}
						blob := &object.Blob{Content: content}
						s.PutBlob(blob)
					case 3:
						// getBlob (try to get something that might exist)
						hash := object.HashBytes([]byte{byte(id), byte(j), byte(id ^ j)})
						s.GetBlob(hash) // ignore error, might not exist
					case 4:
						// stats
						s.Stats()
					}
				}
			}(i)
		}

		wg.Wait()

		// verify store is in consistent state
		stats := s.Stats()
		if stats.IndexSize < 0 {
			t.Error("Stats() returned negative IndexSize after mixed operations")
		}
		if stats.ObjectCount < 0 {
			t.Error("Stats() returned negative ObjectCount after mixed operations")
		}
	})
}
