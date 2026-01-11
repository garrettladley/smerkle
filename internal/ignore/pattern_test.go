package ignore

import (
	"testing"
)

func mustCompile(t *testing.T, pattern string) *Pattern {
	t.Helper()
	p, err := Compile(pattern, 1)
	if err != nil {
		t.Fatalf("Compile(%q) error = %v", pattern, err)
	}
	return p
}

func TestPatternMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		{
			name:    "literal match",
			pattern: "foo.txt",
			path:    "foo.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "literal no match",
			pattern: "foo.txt",
			path:    "bar.txt",
			isDir:   false,
			want:    false,
		},
		{
			name:    "literal in subdirectory",
			pattern: "foo.txt",
			path:    "subdir/foo.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "literal deep subdirectory",
			pattern: "foo.txt",
			path:    "a/b/c/foo.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "star matches extension",
			pattern: "*.log",
			path:    "debug.log",
			isDir:   false,
			want:    true,
		},
		{
			name:    "star in subdirectory",
			pattern: "*.log",
			path:    "logs/debug.log",
			isDir:   false,
			want:    true,
		},
		{
			name:    "star does not match slash",
			pattern: "foo*bar",
			path:    "foo/bar",
			isDir:   false,
			want:    false,
		},
		{
			name:    "star matches empty",
			pattern: "foo*.txt",
			path:    "foo.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "star at start",
			pattern: "*bar.txt",
			path:    "foobar.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "star matches multiple chars",
			pattern: "*.log",
			path:    "very-long-name.log",
			isDir:   false,
			want:    true,
		},
		{
			name:    "question matches single char",
			pattern: "file?.txt",
			path:    "file1.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "question does not match slash",
			pattern: "file?txt",
			path:    "file/txt",
			isDir:   false,
			want:    false,
		},
		{
			name:    "question does not match empty",
			pattern: "file?.txt",
			path:    "file.txt",
			isDir:   false,
			want:    false,
		},
		{
			name:    "multiple questions",
			pattern: "f??e.txt",
			path:    "file.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "doublestar matches directories",
			pattern: "**/foo.txt",
			path:    "a/b/c/foo.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "doublestar at root",
			pattern: "**/foo.txt",
			path:    "foo.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "doublestar in middle",
			pattern: "a/**/z",
			path:    "a/b/c/z",
			isDir:   false,
			want:    true,
		},
		{
			name:    "doublestar matches zero dirs",
			pattern: "a/**/z",
			path:    "a/z",
			isDir:   false,
			want:    true,
		},
		{
			name:    "trailing doublestar",
			pattern: "src/**",
			path:    "src/foo/bar/baz.go",
			isDir:   false,
			want:    true,
		},
		{
			name:    "trailing doublestar matches file in dir",
			pattern: "src/**",
			path:    "src/main.go",
			isDir:   false,
			want:    true,
		},
		{
			name:    "doublestar with pattern after",
			pattern: "**/test/*.go",
			path:    "pkg/test/main.go",
			isDir:   false,
			want:    true,
		},
		{
			name:    "doublestar does not match partial",
			pattern: "**/foo.txt",
			path:    "bar.txt",
			isDir:   false,
			want:    false,
		},
		{
			name:    "dir only matches directory",
			pattern: "build/",
			path:    "build",
			isDir:   true,
			want:    true,
		},
		{
			name:    "dir only does not match file",
			pattern: "build/",
			path:    "build",
			isDir:   false,
			want:    false,
		},
		{
			name:    "dir only in subdirectory",
			pattern: "node_modules/",
			path:    "project/node_modules",
			isDir:   true,
			want:    true,
		},
		{
			name:    "dir only with star",
			pattern: "*_cache/",
			path:    "pip_cache",
			isDir:   true,
			want:    true,
		},
		{
			name:    "anchored matches at root",
			pattern: "/build",
			path:    "build",
			isDir:   false,
			want:    true,
		},
		{
			name:    "anchored does not match in subdir",
			pattern: "/build",
			path:    "src/build",
			isDir:   false,
			want:    false,
		},
		{
			name:    "anchored with glob",
			pattern: "/*.log",
			path:    "debug.log",
			isDir:   false,
			want:    true,
		},
		{
			name:    "anchored glob not in subdir",
			pattern: "/*.log",
			path:    "logs/debug.log",
			isDir:   false,
			want:    false,
		},
		{
			name:    "path with slash is anchored",
			pattern: "doc/frotz",
			path:    "doc/frotz",
			isDir:   false,
			want:    true,
		},
		{
			name:    "path with slash not in wrong dir",
			pattern: "doc/frotz",
			path:    "other/doc/frotz",
			isDir:   false,
			want:    false,
		},
		{
			name:    "multi-level path anchored",
			pattern: "a/b/c",
			path:    "a/b/c",
			isDir:   false,
			want:    true,
		},
		{
			name:    "multi-level path not in subdir",
			pattern: "a/b/c",
			path:    "x/a/b/c",
			isDir:   false,
			want:    false,
		},
		{
			name:    "bracket matches char",
			pattern: "file[0-9].txt",
			path:    "file5.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "bracket no match",
			pattern: "file[0-9].txt",
			path:    "filea.txt",
			isDir:   false,
			want:    false,
		},
		{
			name:    "bracket with multiple chars",
			pattern: "file[abc].txt",
			path:    "fileb.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "bracket negation",
			pattern: "file[!0-9].txt",
			path:    "filea.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "bracket negation no match",
			pattern: "file[!0-9].txt",
			path:    "file5.txt",
			isDir:   false,
			want:    false,
		},
		{
			name:    "empty path no match",
			pattern: "*.log",
			path:    "",
			isDir:   false,
			want:    false,
		},
		{
			name:    "just star matches any file",
			pattern: "*",
			path:    "anything.txt",
			isDir:   false,
			want:    true,
		},
		{
			name:    "just star matches basename in path with slash",
			pattern: "*",
			path:    "dir/file.txt",
			isDir:   false,
			want:    true, // gitignore: * matches any filename, so matches file.txt
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := mustCompile(t, tt.pattern)
			got := p.Match(tt.path, tt.isDir)
			if got != tt.want {
				t.Errorf("Pattern(%q).Match(%q, isDir=%v) = %v, want %v",
					tt.pattern, tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestPatternNegated(t *testing.T) {
	t.Parallel()

	p, err := Compile("!*.log", 1)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if !p.negated {
		t.Error("expected pattern to be negated")
	}

	// pattern should still match the path (negation is handled by Ignorer)
	if !p.Match("debug.log", false) {
		t.Error("negated pattern should still match the path")
	}
}

func TestCompileInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
	}{
		{
			name:    "unclosed bracket",
			pattern: "file[abc.txt",
		},
		// note: invalid ranges like [z-a] are not detected by filepath.Match
		// and would need additional validation if required
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Compile(tt.pattern, 1)
			if err == nil {
				t.Errorf("Compile(%q) expected error", tt.pattern)
			}
		})
	}
}
