package ignore

import (
	"fmt"
	"io"
	"os"
)

type Ignorer struct {
	patterns []Pattern
}

type Result struct {
	Ignored    bool
	Pattern    string // the pattern that matched (empty if no match)
	Negated    bool   // true if the final match was from a negation pattern
	LineNumber int    // line number of the matching pattern (0 if no match)
}

func New(r io.Reader) (*Ignorer, error) {
	patterns, err := Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse ignore patterns: %w", err)
	}

	return &Ignorer{
		patterns: patterns,
	}, nil
}

func NewFromFile(path string) (*Ignorer, error) {
	f, err := os.Open(path) //nolint:gosec // path is intentionally user-controlled
	if err != nil {
		return nil, fmt.Errorf("open ignore file: %w", err)
	}
	defer func() { _ = f.Close() }()

	return New(f)
}

// Match returns true if the path should be ignored.
// The path should be relative to the repository root.
// isDir indicates whether the path is a directory.
//
// Match implements ignore semantics where the last matching pattern wins,
// and negation patterns (!) can unignore previously ignored paths.
func (i *Ignorer) Match(path string, isDir bool) bool {
	return i.MatchResult(path, isDir).Ignored
}

// MatchResult returns detailed match information including which pattern matched.
func (i *Ignorer) MatchResult(path string, isDir bool) (result Result) {
	// process patterns in order - last match wins
	for _, p := range i.patterns {
		if p.Match(path, isDir) {
			if p.negated {
				result = Result{
					Ignored:    false,
					Pattern:    p.original,
					Negated:    true,
					LineNumber: p.lineNumber,
				}
			} else {
				result = Result{
					Ignored:    true,
					Pattern:    p.original,
					Negated:    false,
					LineNumber: p.lineNumber,
				}
			}
		}
	}

	return result
}
