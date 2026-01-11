package ignore

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

type Pattern struct {
	original   string // original pattern text as written
	pattern    string // normalized pattern for matching
	negated    bool   // ! prefix - negates the match
	anchored   bool   // / at start or contains / - only matches at root
	dirOnly    bool   // / at end - only matches directories
	lineNumber int    // source line number for debugging
}

func Compile(pattern string, lineNumber int) (*Pattern, error) {
	if pattern == "" {
		return nil, errors.New("empty pattern")
	}

	p := &Pattern{
		original:   pattern,
		pattern:    pattern,
		lineNumber: lineNumber,
	}

	// check for escaped exclamation at start (\! means literal !, not negation)
	if strings.HasPrefix(p.pattern, "\\!") {
		p.negated = false
		p.pattern = p.pattern[1:] // remove the backslash, keep the !
	} else if strings.HasPrefix(p.pattern, "!") {
		p.negated = true
		p.pattern = p.pattern[1:]
	}

	// check for directory-only suffix
	if strings.HasSuffix(p.pattern, "/") {
		p.dirOnly = true
		p.pattern = strings.TrimSuffix(p.pattern, "/")
	}

	// check for anchored prefix
	if strings.HasPrefix(p.pattern, "/") {
		p.anchored = true
		p.pattern = p.pattern[1:]
	}

	// pattern with / in it (but not just **/) is implicitly anchored
	// unless it starts with **
	if !p.anchored && strings.Contains(p.pattern, "/") {
		if !strings.HasPrefix(p.pattern, "**/") && !strings.HasPrefix(p.pattern, "**") {
			p.anchored = true
		}
	}

	if err := validatePattern(p.pattern); err != nil {
		return nil, err
	}

	return p, nil
}

func validatePattern(pattern string) error {
	testPattern := strings.ReplaceAll(pattern, "**", "*")

	_, err := filepath.Match(testPattern, "test")
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}
	return nil
}

func (p *Pattern) Match(path string, isDir bool) bool {
	if path == "" {
		return false
	}

	// directory-only patterns only match directories
	if p.dirOnly && !isDir {
		return false
	}

	return p.matchPath(path)
}

func (p *Pattern) matchPath(path string) bool {
	pattern := p.pattern

	if strings.Contains(pattern, "**") {
		return p.matchDoublestar(path)
	}

	// for anchored patterns, match against the full path
	if p.anchored {
		return matchGlob(pattern, path)
	}

	// for non-anchored patterns without slashes, match against basename
	// at any level of the path hierarchy
	if !strings.Contains(pattern, "/") {
		// try matching the basename
		if matchGlob(pattern, filepath.Base(path)) {
			return true
		}
		// for deeper paths, try matching each component
		for part := range strings.SplitSeq(path, "/") {
			if matchGlob(pattern, part) {
				return true
			}
		}
		return false
	}

	// pattern contains / but is not anchored - this shouldn't happen
	// as patterns with / are implicitly anchored, but handle it anyway
	return matchGlob(pattern, path)
}

func (p *Pattern) matchDoublestar(path string) bool {
	pattern := p.pattern

	// handle patterns like **/foo/** (leading and trailing doublestar)
	if strings.HasPrefix(pattern, "**/") && strings.HasSuffix(pattern, "/**") {
		return p.matchBothDoublestars(path, pattern)
	}

	// handle leading **/ - matches any prefix
	if strings.HasPrefix(pattern, "**/") {
		return p.matchLeadingDoublestar(path, pattern)
	}

	// handle trailing /** - matches any suffix
	if strings.HasSuffix(pattern, "/**") {
		return p.matchTrailingDoublestar(path, pattern)
	}

	// handle ** in the middle
	idx := strings.Index(pattern, "/**/")
	if idx != -1 {
		prefix := pattern[:idx]
		suffix := pattern[idx+4:]
		return matchWithMiddleDoublestar(path, prefix, suffix, p.anchored)
	}

	// handle pattern like "a/**/z" where ** is not surrounded by /
	parts := strings.SplitN(pattern, "**", 2)
	if len(parts) == 2 {
		prefix := strings.TrimSuffix(parts[0], "/")
		suffix := strings.TrimPrefix(parts[1], "/")
		return matchWithMiddleDoublestar(path, prefix, suffix, p.anchored)
	}

	return false
}

func (p *Pattern) matchBothDoublestars(path, pattern string) bool {
	middle := pattern[3 : len(pattern)-3]
	parts := strings.Split(path, "/")

	if strings.Contains(middle, "/") {
		return p.matchMultiComponentMiddle(parts, middle)
	}
	return p.matchSingleComponentMiddle(parts, middle)
}

func (p *Pattern) matchMultiComponentMiddle(parts []string, middle string) bool {
	middleParts := strings.Split(middle, "/")
	maxStart := len(parts) - len(middleParts) - 1
	for i := 0; i <= maxStart; i++ {
		if matchSequence(parts[i:i+len(middleParts)], middleParts) {
			return true
		}
	}
	return false
}

func (p *Pattern) matchSingleComponentMiddle(parts []string, middle string) bool {
	for i, part := range parts {
		if matchGlob(middle, part) && i < len(parts)-1 {
			return true
		}
	}
	return false
}

func matchSequence(pathParts, patternParts []string) bool {
	for j, mp := range patternParts {
		if !matchGlob(mp, pathParts[j]) {
			return false
		}
	}
	return true
}

func (p *Pattern) matchLeadingDoublestar(path, pattern string) bool {
	rest := pattern[3:]
	parts := strings.Split(path, "/")

	// if rest still contains **, recurse
	if strings.Contains(rest, "**") {
		subPattern := &Pattern{pattern: rest, anchored: false}
		for i := range parts {
			if subPattern.matchDoublestar(strings.Join(parts[i:], "/")) {
				return true
			}
		}
		return false
	}

	// try matching from any position
	if matchGlob(rest, path) {
		return true
	}
	for i := range parts {
		if matchGlob(rest, strings.Join(parts[i:], "/")) {
			return true
		}
	}
	return false
}

func (p *Pattern) matchTrailingDoublestar(path, pattern string) bool {
	prefix := pattern[:len(pattern)-3]
	parts := strings.Split(path, "/")

	// if prefix contains **, handle recursively
	if strings.Contains(prefix, "**") {
		subPattern := &Pattern{pattern: prefix, anchored: p.anchored}
		for i := range parts {
			if subPattern.matchDoublestar(strings.Join(parts[:i+1], "/")) {
				return true
			}
		}
		return false
	}

	if p.anchored {
		return strings.HasPrefix(path, prefix+"/") || path == prefix
	}

	// for non-anchored, check any position
	for i := 0; i < len(parts)-1; i++ {
		subpath := strings.Join(parts[:i+1], "/")
		if matchGlob(prefix, subpath) {
			return true
		}
		if !strings.Contains(prefix, "/") && matchGlob(prefix, parts[i]) {
			return true
		}
	}
	return false
}

func matchWithMiddleDoublestar(path, prefix, suffix string, anchored bool) bool {
	pathParts := strings.Split(path, "/")

	// find all positions where prefix matches
	var prefixMatches []int
	for i := range pathParts {
		var subpath string
		if i == 0 {
			subpath = pathParts[0]
		} else {
			subpath = strings.Join(pathParts[:i+1], "/")
		}

		if anchored {
			// for anchored patterns, only match at start
			if i == 0 && matchGlob(prefix, subpath) {
				prefixMatches = append(prefixMatches, i)
			} else if prefix == "" {
				prefixMatches = append(prefixMatches, -1) // match before first element
			}
		} else {
			if matchGlob(prefix, pathParts[i]) {
				prefixMatches = append(prefixMatches, i)
			}
		}
	}

	// special case: empty prefix matches at start
	if prefix == "" {
		prefixMatches = append(prefixMatches, -1)
	}

	// for each prefix match, try matching suffix at any subsequent position
	for _, prefixEnd := range prefixMatches {
		for suffixStart := prefixEnd + 1; suffixStart < len(pathParts); suffixStart++ {
			remaining := strings.Join(pathParts[suffixStart:], "/")
			if matchGlob(suffix, remaining) {
				return true
			}
			// also try matching just the component
			if !strings.Contains(suffix, "/") && matchGlob(suffix, pathParts[suffixStart]) {
				// only match if this is the last component or suffix matches rest
				if suffixStart == len(pathParts)-1 {
					return true
				}
			}
		}
		// also try matching suffix against the remaining path
		if prefixEnd+1 < len(pathParts) {
			remaining := strings.Join(pathParts[prefixEnd+1:], "/")
			if matchGlob(suffix, remaining) {
				return true
			}
		}
	}

	return false
}

func matchGlob(pattern, name string) bool {
	// handle empty cases
	if pattern == "" && name == "" {
		return true
	}
	if pattern == "" || name == "" {
		return pattern == "" && name == ""
	}

	// convert ! in character classes to ^ for filepath.Match
	pattern = convertCharClassNegation(pattern)

	matched, err := filepath.Match(pattern, name)
	if err != nil {
		return false
	}
	return matched
}

func convertCharClassNegation(pattern string) string {
	var result strings.Builder
	result.Grow(len(pattern))

	var (
		inBracket = false
		afterOpen = false
	)

	for i := 0; i < len(pattern); i++ {
		c := pattern[i]

		if c == '[' && !inBracket {
			inBracket = true
			afterOpen = true
			result.WriteByte(c)
			continue
		}

		if inBracket {
			if afterOpen && c == '!' {
				result.WriteByte('^')
				afterOpen = false
				continue
			}
			if c == ']' {
				inBracket = false
			}
			afterOpen = false
		}

		result.WriteByte(c)
	}

	return result.String()
}
