package object

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

const (
	MagicBlob  = "MRKB"
	MagicTree  = "MRKT"
	MagicIndex = "MRKI"
)

const CurrentVersion uint16 = 1

type Header struct {
	Magic   [4]byte
	Version uint16
}

func WriteHeader(w io.Writer, magic string) error {
	var h Header
	copy(h.Magic[:], magic)
	h.Version = CurrentVersion
	return binary.Write(w, binary.BigEndian, h)
}

func ReadHeader(r io.Reader, expectedMagic string) (uint16, error) {
	var h Header
	if err := binary.Read(r, binary.BigEndian, &h); err != nil {
		return 0, fmt.Errorf("read header: %w", err)
	}

	if string(h.Magic[:]) != expectedMagic {
		return 0, fmt.Errorf("invalid magic: got %q, want %q", h.Magic[:], expectedMagic)
	}

	if h.Version > CurrentVersion {
		return 0, fmt.Errorf("unsupported version: got %d, max supported %d", h.Version, CurrentVersion)
	}

	return h.Version, nil
}

func EncodeBlob(b *Blob) ([]byte, error) {
	var buf bytes.Buffer
	if err := WriteHeader(&buf, MagicBlob); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.BigEndian, uint64(len(b.Content))); err != nil {
		return nil, err
	}
	buf.Write(b.Content)

	return buf.Bytes(), nil
}

func DecodeBlob(data []byte) (*Blob, error) {
	r := bytes.NewReader(data)

	version, err := ReadHeader(r, MagicBlob)
	if err != nil {
		return nil, err
	}

	switch version {
	case 1:
		return decodeBlobV1(r)
	default:
		return nil, fmt.Errorf("unknown blob version: %d", version)
	}
}

func decodeBlobV1(r io.Reader) (*Blob, error) {
	var length uint64
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("read content length: %w", err)
	}

	content := make([]byte, length)
	if _, err := io.ReadFull(r, content); err != nil {
		return nil, fmt.Errorf("read content: %w", err)
	}

	return &Blob{Content: content}, nil
}

func EncodeTree(t *Tree) ([]byte, error) {
	var buf bytes.Buffer
	if err := WriteHeader(&buf, MagicTree); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.BigEndian, uint32(len(t.Entries))); err != nil {
		return nil, err
	}

	for _, e := range t.Entries {
		if err := encodeEntry(&buf, &e); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func encodeEntry(w io.Writer, e *Entry) error {
	// mode (1 byte)
	if err := binary.Write(w, binary.BigEndian, e.Mode); err != nil {
		return err
	}

	// size (8 bytes)
	if err := binary.Write(w, binary.BigEndian, e.Size); err != nil {
		return err
	}

	// name length + name
	nameBytes := []byte(e.Name)
	if err := binary.Write(w, binary.BigEndian, uint16(len(nameBytes))); err != nil {
		return err
	}
	if _, err := w.Write(nameBytes); err != nil {
		return err
	}

	// hash (32 bytes)
	if _, err := w.Write(e.Hash[:]); err != nil {
		return err
	}

	return nil
}

func DecodeTree(data []byte) (*Tree, error) {
	r := bytes.NewReader(data)

	version, err := ReadHeader(r, MagicTree)
	if err != nil {
		return nil, err
	}

	switch version {
	case 1:
		return decodeTreeV1(r)
	default:
		return nil, fmt.Errorf("unknown tree version: %d", version)
	}
}

func decodeTreeV1(r io.Reader) (*Tree, error) {
	var count uint32
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return nil, fmt.Errorf("read entry count: %w", err)
	}

	entries := make([]Entry, count)
	for i := range entries {
		if err := decodeEntryV1(r, &entries[i]); err != nil {
			return nil, fmt.Errorf("decode entry %d: %w", i, err)
		}
	}

	return &Tree{Entries: entries}, nil
}

func decodeEntryV1(r io.Reader, e *Entry) error {
	// mode
	if err := binary.Read(r, binary.BigEndian, &e.Mode); err != nil {
		return err
	}

	// size
	if err := binary.Read(r, binary.BigEndian, &e.Size); err != nil {
		return err
	}

	// name
	var nameLen uint16
	if err := binary.Read(r, binary.BigEndian, &nameLen); err != nil {
		return err
	}
	nameBytes := make([]byte, nameLen)
	if _, err := io.ReadFull(r, nameBytes); err != nil {
		return err
	}
	e.Name = string(nameBytes)

	// hash
	if _, err := io.ReadFull(r, e.Hash[:]); err != nil {
		return err
	}

	return nil
}

type Index struct {
	Entries []IndexEntry
}

func EncodeIndex(idx *Index) ([]byte, error) {
	var buf bytes.Buffer
	if err := WriteHeader(&buf, MagicIndex); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.BigEndian, uint32(len(idx.Entries))); err != nil {
		return nil, err
	}

	for _, e := range idx.Entries {
		if err := encodeIndexEntry(&buf, &e); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func encodeIndexEntry(w io.Writer, e *IndexEntry) error {
	// path length + path
	pathBytes := []byte(e.Path)
	if err := binary.Write(w, binary.BigEndian, uint16(len(pathBytes))); err != nil {
		return err
	}
	if _, err := w.Write(pathBytes); err != nil {
		return err
	}

	// size
	if err := binary.Write(w, binary.BigEndian, e.Size); err != nil {
		return err
	}

	// modTime as seconds + nanoseconds
	if err := binary.Write(w, binary.BigEndian, e.ModTime.Unix()); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, int32(e.ModTime.Nanosecond())); err != nil {
		return err
	}

	// hash
	if _, err := w.Write(e.Hash[:]); err != nil {
		return err
	}

	return nil
}

func DecodeIndex(data []byte) (*Index, error) {
	r := bytes.NewReader(data)

	version, err := ReadHeader(r, MagicIndex)
	if err != nil {
		return nil, err
	}

	switch version {
	case 1:
		return decodeIndexV1(r)
	default:
		return nil, fmt.Errorf("unknown index version: %d", version)
	}
}

func decodeIndexV1(r io.Reader) (*Index, error) {
	var count uint32
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return nil, fmt.Errorf("read entry count: %w", err)
	}

	entries := make([]IndexEntry, count)
	for i := range entries {
		if err := decodeIndexEntryV1(r, &entries[i]); err != nil {
			return nil, fmt.Errorf("decode entry %d: %w", i, err)
		}
	}

	return &Index{Entries: entries}, nil
}

func decodeIndexEntryV1(r io.Reader, e *IndexEntry) error {
	// path
	var pathLen uint16
	if err := binary.Read(r, binary.BigEndian, &pathLen); err != nil {
		return err
	}
	pathBytes := make([]byte, pathLen)
	if _, err := io.ReadFull(r, pathBytes); err != nil {
		return err
	}
	e.Path = string(pathBytes)

	// size
	if err := binary.Read(r, binary.BigEndian, &e.Size); err != nil {
		return err
	}

	// modTime as seconds + nanoseconds
	var secs int64
	if err := binary.Read(r, binary.BigEndian, &secs); err != nil {
		return err
	}
	var nsec int32
	if err := binary.Read(r, binary.BigEndian, &nsec); err != nil {
		return err
	}
	e.ModTime = time.Unix(secs, int64(nsec))

	// hash
	if _, err := io.ReadFull(r, e.Hash[:]); err != nil {
		return err
	}

	return nil
}
