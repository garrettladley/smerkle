package result

import (
	"github.com/garrettladley/smerkle/internal/object"
	"github.com/garrettladley/smerkle/internal/xerrors"
)

type Result struct {
	Hash   object.Hash
	Errors []xerrors.HashError
}

func (r *Result) Ok() bool {
	return len(r.Errors) == 0
}

func (r *Result) Err() error {
	if r.Ok() {
		return nil
	}
	return &xerrors.MultiError{Errors: r.Errors}
}
