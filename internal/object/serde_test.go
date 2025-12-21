package object

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"
	"time"
)

func TestEncodeDecodeBlob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		blob    *Blob
		wantErr bool
	}{
		{
			name:    "empty content",
			blob:    &Blob{Content: []byte{}},
			wantErr: false,
		},
		{
			name:    "simple text",
			blob:    &Blob{Content: []byte("hello world")},
			wantErr: false,
		},
		{
			name:    "binary content",
			blob:    &Blob{Content: []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd}},
			wantErr: false,
		},
		{
			name:    "large content",
			blob:    &Blob{Content: bytes.Repeat([]byte("x"), 1<<16)},
			wantErr: false,
		},
		{
			name:    "unicode content",
			blob:    &Blob{Content: []byte("こんにちは世界 🌍")},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := EncodeBlob(tt.blob)
			if (err != nil) != tt.wantErr {
				t.Fatalf("EncodeBlob() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			decoded, err := DecodeBlob(encoded)
			if err != nil {
				t.Fatalf("DecodeBlob() error = %v", err)
			}

			if !bytes.Equal(decoded.Content, tt.blob.Content) {
				t.Errorf("content mismatch: got %v, want %v", decoded.Content, tt.blob.Content)
			}
		})
	}
}

func TestDecodeBlobErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: "read header",
		},
		{
			name:    "wrong magic",
			data:    []byte("XXXX\x00\x01"),
			wantErr: "invalid magic",
		},
		{
			name:    "unsupported version",
			data:    append([]byte("MRKB"), 0x00, 0x99),
			wantErr: "unsupported version",
		},
		{
			name:    "truncated length",
			data:    []byte("MRKB\x00\x01\x00\x00"),
			wantErr: "read content length",
		},
		{
			name:    "truncated content",
			data:    []byte("MRKB\x00\x01\x00\x00\x00\x00\x00\x00\x00\x10"),
			wantErr: "read content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeBlob(tt.data)
			if err == nil {
				t.Fatal("DecodeBlob() expected error, got nil")
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErr)) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEncodeDecodeTree(t *testing.T) {
	t.Parallel()

	hash1 := HashBytes([]byte("file1"))
	hash2 := HashBytes([]byte("file2"))
	hash3 := HashBytes([]byte("dir"))

	tests := []struct {
		name    string
		tree    *Tree
		wantErr bool
	}{
		{
			name:    "empty tree",
			tree:    &Tree{Entries: []Entry{}},
			wantErr: false,
		},
		{
			name: "single file entry",
			tree: &Tree{Entries: []Entry{
				{Name: "file.txt", Mode: ModeRegular, Size: 100, Hash: hash1},
			}},
			wantErr: false,
		},
		{
			name: "multiple entries with different modes",
			tree: &Tree{Entries: []Entry{
				{Name: "readme.md", Mode: ModeRegular, Size: 1024, Hash: hash1},
				{Name: "script.sh", Mode: ModeExecutable, Size: 512, Hash: hash2},
				{Name: "subdir", Mode: ModeDirectory, Size: 0, Hash: hash3},
			}},
			wantErr: false,
		},
		{
			name: "unicode filenames",
			tree: &Tree{Entries: []Entry{
				{Name: "文件.txt", Mode: ModeRegular, Size: 42, Hash: hash1},
				{Name: "données.json", Mode: ModeRegular, Size: 256, Hash: hash2},
			}},
			wantErr: false,
		},
		{
			name: "symlink entry",
			tree: &Tree{Entries: []Entry{
				{Name: "link", Mode: ModeSymlink, Size: 10, Hash: hash1},
			}},
			wantErr: false,
		},
		{
			name: "zero hash",
			tree: &Tree{Entries: []Entry{
				{Name: "empty", Mode: ModeRegular, Size: 0, Hash: ZeroHash},
			}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := EncodeTree(tt.tree)
			if (err != nil) != tt.wantErr {
				t.Fatalf("EncodeTree() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			decoded, err := DecodeTree(encoded)
			if err != nil {
				t.Fatalf("DecodeTree() error = %v", err)
			}

			if len(decoded.Entries) != len(tt.tree.Entries) {
				t.Fatalf("entry count mismatch: got %d, want %d", len(decoded.Entries), len(tt.tree.Entries))
			}

			for i, want := range tt.tree.Entries {
				got := decoded.Entries[i]
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

func TestDecodeTreeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: "read header",
		},
		{
			name:    "wrong magic",
			data:    []byte("MRKB\x00\x01"),
			wantErr: "invalid magic",
		},
		{
			name:    "truncated entry count",
			data:    []byte("MRKT\x00\x01\x00"),
			wantErr: "read entry count",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeTree(tt.data)
			if err == nil {
				t.Fatal("DecodeTree() expected error, got nil")
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErr)) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEncodeDecodeIndex(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Nanosecond)
	hash1 := HashBytes([]byte("content1"))
	hash2 := HashBytes([]byte("content2"))

	tests := []struct {
		name    string
		index   *Index
		wantErr bool
	}{
		{
			name:    "empty index",
			index:   &Index{Entries: []IndexEntry{}},
			wantErr: false,
		},
		{
			name: "single entry",
			index: &Index{Entries: []IndexEntry{
				{Path: "file.txt", Size: 100, ModTime: now, Hash: hash1},
			}},
			wantErr: false,
		},
		{
			name: "multiple entries",
			index: &Index{Entries: []IndexEntry{
				{Path: "src/main.go", Size: 1024, ModTime: now, Hash: hash1},
				{Path: "src/util/helper.go", Size: 512, ModTime: now.Add(-time.Hour), Hash: hash2},
			}},
			wantErr: false,
		},
		{
			name: "paths with special characters",
			index: &Index{Entries: []IndexEntry{
				{Path: "path/with spaces/file.txt", Size: 10, ModTime: now, Hash: hash1},
				{Path: "path/日本語/ファイル.txt", Size: 20, ModTime: now, Hash: hash2},
			}},
			wantErr: false,
		},
		{
			name: "zero time",
			index: &Index{Entries: []IndexEntry{
				{Path: "old.txt", Size: 5, ModTime: time.Time{}, Hash: hash1},
			}},
			wantErr: false,
		},
		{
			name: "negative size",
			index: &Index{Entries: []IndexEntry{
				{Path: "weird.txt", Size: -1, ModTime: now, Hash: hash1},
			}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := EncodeIndex(tt.index)
			if (err != nil) != tt.wantErr {
				t.Fatalf("EncodeIndex() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			decoded, err := DecodeIndex(encoded)
			if err != nil {
				t.Fatalf("DecodeIndex() error = %v", err)
			}

			if len(decoded.Entries) != len(tt.index.Entries) {
				t.Fatalf("entry count mismatch: got %d, want %d", len(decoded.Entries), len(tt.index.Entries))
			}

			for i, want := range tt.index.Entries {
				got := decoded.Entries[i]
				if got.Path != want.Path {
					t.Errorf("entry[%d].Path = %q, want %q", i, got.Path, want.Path)
				}
				if got.Size != want.Size {
					t.Errorf("entry[%d].Size = %d, want %d", i, got.Size, want.Size)
				}
				if !got.ModTime.Equal(want.ModTime) {
					t.Errorf("entry[%d].ModTime = %v, want %v", i, got.ModTime, want.ModTime)
				}
				if got.Hash != want.Hash {
					t.Errorf("entry[%d].Hash = %v, want %v", i, got.Hash, want.Hash)
				}
			}
		})
	}
}

func TestDecodeIndexErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: "read header",
		},
		{
			name:    "wrong magic",
			data:    []byte("MRKB\x00\x01"),
			wantErr: "invalid magic",
		},
		{
			name:    "truncated entry count",
			data:    []byte("MRKI\x00\x01\x00"),
			wantErr: "read entry count",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeIndex(tt.data)
			if err == nil {
				t.Fatal("DecodeIndex() expected error, got nil")
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErr)) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestHeaderRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		magic string
	}{
		{name: "blob magic", magic: MagicBlob},
		{name: "tree magic", magic: MagicTree},
		{name: "index magic", magic: MagicIndex},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			if err := WriteHeader(&buf, tt.magic); err != nil {
				t.Fatalf("WriteHeader() error = %v", err)
			}

			version, err := ReadHeader(&buf, tt.magic)
			if err != nil {
				t.Fatalf("ReadHeader() error = %v", err)
			}

			if version != CurrentVersion {
				t.Errorf("version = %d, want %d", version, CurrentVersion)
			}
		})
	}
}

func TestBlobEncodedFormat(t *testing.T) {
	t.Parallel()

	blob := &Blob{Content: []byte("test")}
	encoded, err := EncodeBlob(blob)
	if err != nil {
		t.Fatalf("EncodeBlob() error = %v", err)
	}

	// Verify header structure
	if string(encoded[:4]) != MagicBlob {
		t.Errorf("magic = %q, want %q", encoded[:4], MagicBlob)
	}

	version := binary.BigEndian.Uint16(encoded[4:6])
	if version != CurrentVersion {
		t.Errorf("version = %d, want %d", version, CurrentVersion)
	}

	length := binary.BigEndian.Uint64(encoded[6:14])
	if length != 4 {
		t.Errorf("content length = %d, want 4", length)
	}

	if !bytes.Equal(encoded[14:], []byte("test")) {
		t.Errorf("content = %q, want %q", encoded[14:], "test")
	}
}

func TestTreeEncodedFormat(t *testing.T) {
	t.Parallel()

	hash := HashBytes([]byte("content"))
	tree := &Tree{Entries: []Entry{
		{Name: "a.txt", Mode: ModeRegular, Size: 42, Hash: hash},
	}}

	encoded, err := EncodeTree(tree)
	if err != nil {
		t.Fatalf("EncodeTree() error = %v", err)
	}

	if string(encoded[:4]) != MagicTree {
		t.Errorf("magic = %q, want %q", encoded[:4], MagicTree)
	}

	version := binary.BigEndian.Uint16(encoded[4:6])
	if version != CurrentVersion {
		t.Errorf("version = %d, want %d", version, CurrentVersion)
	}

	entryCount := binary.BigEndian.Uint32(encoded[6:10])
	if entryCount != 1 {
		t.Errorf("entry count = %d, want 1", entryCount)
	}
}

func TestIndexEncodedFormat(t *testing.T) {
	t.Parallel()

	now := time.Now()
	hash := HashBytes([]byte("content"))
	index := &Index{Entries: []IndexEntry{
		{Path: "file.txt", Size: 100, ModTime: now, Hash: hash},
	}}

	encoded, err := EncodeIndex(index)
	if err != nil {
		t.Fatalf("EncodeIndex() error = %v", err)
	}

	if string(encoded[:4]) != MagicIndex {
		t.Errorf("magic = %q, want %q", encoded[:4], MagicIndex)
	}

	version := binary.BigEndian.Uint16(encoded[4:6])
	if version != CurrentVersion {
		t.Errorf("version = %d, want %d", version, CurrentVersion)
	}

	entryCount := binary.BigEndian.Uint32(encoded[6:10])
	if entryCount != 1 {
		t.Errorf("entry count = %d, want 1", entryCount)
	}
}

func TestEncodeDeterministic(t *testing.T) {
	t.Parallel()

	t.Run("blob", func(t *testing.T) {
		t.Parallel()
		blob := &Blob{Content: []byte("deterministic")}

		enc1, _ := EncodeBlob(blob)
		enc2, _ := EncodeBlob(blob)

		if !bytes.Equal(enc1, enc2) {
			t.Error("EncodeBlob is not deterministic")
		}
	})

	t.Run("tree", func(t *testing.T) {
		t.Parallel()
		hash := HashBytes([]byte("x"))
		tree := &Tree{Entries: []Entry{
			{Name: "a", Mode: ModeRegular, Size: 1, Hash: hash},
			{Name: "b", Mode: ModeExecutable, Size: 2, Hash: hash},
		}}

		enc1, _ := EncodeTree(tree)
		enc2, _ := EncodeTree(tree)

		if !bytes.Equal(enc1, enc2) {
			t.Error("EncodeTree is not deterministic")
		}
	})

	t.Run("index", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		hash := HashBytes([]byte("x"))
		index := &Index{Entries: []IndexEntry{
			{Path: "a", Size: 1, ModTime: now, Hash: hash},
		}}

		enc1, _ := EncodeIndex(index)
		enc2, _ := EncodeIndex(index)

		if !bytes.Equal(enc1, enc2) {
			t.Error("EncodeIndex is not deterministic")
		}
	})
}

func TestReadHeaderMagicMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		writeMagic    string
		expectedMagic string
	}{
		{"blob as tree", MagicBlob, MagicTree},
		{"tree as index", MagicTree, MagicIndex},
		{"index as blob", MagicIndex, MagicBlob},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			if err := WriteHeader(&buf, tt.writeMagic); err != nil {
				t.Fatalf("WriteHeader() error = %v", err)
			}

			_, err := ReadHeader(&buf, tt.expectedMagic)
			if err == nil {
				t.Fatal("expected error for magic mismatch")
			}
			if !bytes.Contains([]byte(err.Error()), []byte("invalid magic")) {
				t.Errorf("error = %q, want containing 'invalid magic'", err.Error())
			}
		})
	}
}

func TestLargeTree(t *testing.T) {
	t.Parallel()

	entries := make([]Entry, 1000)
	for i := range entries {
		entries[i] = Entry{
			Name: "file_" + string(rune('a'+i%26)) + "_" + string(rune('0'+i%10)),
			Mode: Mode(i % 4), //nolint:gosec // i%4 is always 0-3, fits in uint8
			Size: int64(i * 100),
			Hash: HashBytes([]byte{byte(i)}),
		}
	}
	tree := &Tree{Entries: entries}

	encoded, err := EncodeTree(tree)
	if err != nil {
		t.Fatalf("EncodeTree() error = %v", err)
	}

	decoded, err := DecodeTree(encoded)
	if err != nil {
		t.Fatalf("DecodeTree() error = %v", err)
	}

	if len(decoded.Entries) != len(tree.Entries) {
		t.Fatalf("entry count = %d, want %d", len(decoded.Entries), len(tree.Entries))
	}

	if !slices.EqualFunc(decoded.Entries, tree.Entries, func(a, b Entry) bool {
		return a.Name == b.Name && a.Mode == b.Mode && a.Size == b.Size && a.Hash == b.Hash
	}) {
		t.Error("decoded entries do not match original")
	}
}

func TestLargeIndex(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := make([]IndexEntry, 1000)
	for i := range entries {
		entries[i] = IndexEntry{
			Path:    "path/to/file_" + string(rune('a'+i%26)),
			Size:    int64(i * 50),
			ModTime: baseTime.Add(time.Duration(i) * time.Second),
			Hash:    HashBytes([]byte{byte(i), byte(i >> 8)}),
		}
	}
	index := &Index{Entries: entries}

	encoded, err := EncodeIndex(index)
	if err != nil {
		t.Fatalf("EncodeIndex() error = %v", err)
	}

	decoded, err := DecodeIndex(encoded)
	if err != nil {
		t.Fatalf("DecodeIndex() error = %v", err)
	}

	if len(decoded.Entries) != len(index.Entries) {
		t.Fatalf("entry count = %d, want %d", len(decoded.Entries), len(index.Entries))
	}

	for i, want := range index.Entries {
		got := decoded.Entries[i]
		if got.Path != want.Path || got.Size != want.Size || !got.ModTime.Equal(want.ModTime) || got.Hash != want.Hash {
			t.Errorf("entry[%d] mismatch", i)
		}
	}
}
