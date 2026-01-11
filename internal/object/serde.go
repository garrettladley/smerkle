package object

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
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
	if err := binary.Write(w, binary.BigEndian, h); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	return nil
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
		return nil, fmt.Errorf("write content length: %w", err)
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

	if len(t.Entries) > math.MaxUint32 {
		return nil, fmt.Errorf("too many tree entries: %d", len(t.Entries))
	}
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(t.Entries))); err != nil { //nolint:gosec // bounds checked above
		return nil, fmt.Errorf("write entry count: %w", err)
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
		return fmt.Errorf("write mode: %w", err)
	}

	// size (8 bytes)
	if err := binary.Write(w, binary.BigEndian, e.Size); err != nil {
		return fmt.Errorf("write size: %w", err)
	}

	// name length + name
	nameBytes := []byte(e.Name)
	if len(nameBytes) > math.MaxUint16 {
		return fmt.Errorf("entry name too long: %d bytes", len(nameBytes))
	}
	if err := binary.Write(w, binary.BigEndian, uint16(len(nameBytes))); err != nil { //nolint:gosec // bounds checked above
		return fmt.Errorf("write name length: %w", err)
	}
	if _, err := w.Write(nameBytes); err != nil {
		return fmt.Errorf("write name: %w", err)
	}

	// hash (32 bytes)
	if _, err := w.Write(e.Hash[:]); err != nil {
		return fmt.Errorf("write hash: %w", err)
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
		return fmt.Errorf("read mode: %w", err)
	}

	// size
	if err := binary.Read(r, binary.BigEndian, &e.Size); err != nil {
		return fmt.Errorf("read size: %w", err)
	}

	// name
	var nameLen uint16
	if err := binary.Read(r, binary.BigEndian, &nameLen); err != nil {
		return fmt.Errorf("read name length: %w", err)
	}
	nameBytes := make([]byte, nameLen)
	if _, err := io.ReadFull(r, nameBytes); err != nil {
		return fmt.Errorf("read name: %w", err)
	}
	e.Name = string(nameBytes)

	// hash
	if _, err := io.ReadFull(r, e.Hash[:]); err != nil {
		return fmt.Errorf("read hash: %w", err)
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

	if len(idx.Entries) > math.MaxUint32 {
		return nil, fmt.Errorf("too many index entries: %d", len(idx.Entries))
	}
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(idx.Entries))); err != nil { //nolint:gosec // bounds checked above
		return nil, fmt.Errorf("write entry count: %w", err)
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
	if len(pathBytes) > math.MaxUint16 {
		return fmt.Errorf("index entry path too long: %d bytes", len(pathBytes))
	}
	if err := binary.Write(w, binary.BigEndian, uint16(len(pathBytes))); err != nil { //nolint:gosec // bounds checked above
		return fmt.Errorf("write path length: %w", err)
	}
	if _, err := w.Write(pathBytes); err != nil {
		return fmt.Errorf("write path: %w", err)
	}

	// size
	if err := binary.Write(w, binary.BigEndian, e.Size); err != nil {
		return fmt.Errorf("write size: %w", err)
	}

	// modTime as seconds + nanoseconds
	if err := binary.Write(w, binary.BigEndian, e.ModTime.Unix()); err != nil {
		return fmt.Errorf("write modtime seconds: %w", err)
	}
	if err := binary.Write(w, binary.BigEndian, int32(e.ModTime.Nanosecond())); err != nil { //nolint:gosec // Nanosecond() returns 0-999999999, always fits in int32
		return fmt.Errorf("write modtime nanoseconds: %w", err)
	}

	// hash
	if _, err := w.Write(e.Hash[:]); err != nil {
		return fmt.Errorf("write hash: %w", err)
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
		return fmt.Errorf("read path length: %w", err)
	}
	pathBytes := make([]byte, pathLen)
	if _, err := io.ReadFull(r, pathBytes); err != nil {
		return fmt.Errorf("read path: %w", err)
	}
	e.Path = string(pathBytes)

	// size
	if err := binary.Read(r, binary.BigEndian, &e.Size); err != nil {
		return fmt.Errorf("read size: %w", err)
	}

	// modTime as seconds + nanoseconds
	var secs int64
	if err := binary.Read(r, binary.BigEndian, &secs); err != nil {
		return fmt.Errorf("read modtime seconds: %w", err)
	}
	var nsec int32
	if err := binary.Read(r, binary.BigEndian, &nsec); err != nil {
		return fmt.Errorf("read modtime nanoseconds: %w", err)
	}
	e.ModTime = time.Unix(secs, int64(nsec))

	// hash
	if _, err := io.ReadFull(r, e.Hash[:]); err != nil {
		return fmt.Errorf("read hash: %w", err)
	}

	return nil
}
