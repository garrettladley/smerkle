package main

import (
	"encoding/hex"
	"fmt"

	"github.com/garrettladley/smerkle/internal/diff"
	"github.com/garrettladley/smerkle/internal/object"
	"github.com/garrettladley/smerkle/internal/store"
)

func parseHash(s string) (object.Hash, error) {
	if len(s) != 64 {
		return object.ZeroHash, fmt.Errorf("invalid hash length: expected 64 hex characters, got %d", len(s))
	}

	bytes, err := hex.DecodeString(s)
	if err != nil {
		return object.ZeroHash, fmt.Errorf("invalid hex string: %w", err)
	}

	var h object.Hash
	copy(h[:], bytes)
	return h, nil
}

func openStore() (*store.Store, error) {
	s, err := store.Open(storeDir)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	return s, nil
}

func changeTypePrefix(ct diff.ChangeType) string {
	switch ct {
	case diff.ChangeAdded:
		return "A"
	case diff.ChangeDeleted:
		return "D"
	case diff.ChangeModified:
		return "M"
	case diff.ChangeTypeChange:
		return "T"
	default:
		return "?"
	}
}
