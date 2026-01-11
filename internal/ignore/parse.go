package ignore

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Parse reads gitignore-style patterns from an io.Reader and compiles them.
// It handles:
// - Blank lines (ignored)
// - Comments starting with # (ignored)
// - Trailing spaces (stripped)
// - Escaped special characters (\# \! \\)
// - Negation patterns (!)
// - Anchored patterns (leading /)
// - Directory-only patterns (trailing /)
func Parse(r io.Reader) ([]Pattern, error) {
	var patterns []Pattern

	scanner := bufio.NewScanner(r)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		// handle CRLF line endings
		line = strings.TrimSuffix(line, "\r")

		// strip trailing whitespace
		line = strings.TrimRight(line, " \t")

		if line == "" {
			continue
		}

		// skip comments (lines starting with #, but not \#)
		if strings.HasPrefix(line, "#") {
			continue
		}

		line = processEscapes(line)

		p, err := Compile(line, lineNumber)
		if err != nil {
			continue
		}

		patterns = append(patterns, *p)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read ignore patterns: %w", err)
	}

	return patterns, nil
}

// processEscapes handles escape sequences in gitignore patterns.
// Supported escapes:
// - \# -> # (literal hash, at any position)
// - \! -> ! (literal exclamation at start, NOT a negation)
// - \\ -> \ (literal backslash)
//
// Note: \! at position 0 is handled specially - we keep a marker
// so that Compile knows this is not a negation pattern.
func processEscapes(line string) string {
	if !strings.Contains(line, "\\") {
		return line
	}

	var result strings.Builder
	result.Grow(len(line))

	for i := 0; i < len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) {
			next := line[i+1]
			switch next {
			case '#':
				// \# -> # at any position
				result.WriteByte(next)
				i++
				continue
			case '!':
				if i == 0 {
					// \! at start - keep as \! for Compile to handle
					// (so it knows this is NOT a negation)
					result.WriteByte('\\')
					result.WriteByte('!')
					i++
					continue
				}
				// \! not at start - just literal !
				result.WriteByte(next)
				i++
				continue
			case '\\':
				// \\ -> \
				result.WriteByte(next)
				i++
				continue
			}
		}
		result.WriteByte(line[i])
	}

	return result.String()
}
