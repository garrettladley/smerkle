package ignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustNew(t *testing.T, input string) *Ignorer {
	t.Helper()
	ign, err := New(strings.NewReader(input))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return ign
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("from strings.Reader", func(t *testing.T) {
		t.Parallel()

		input := "*.log\nbuild/"
		ign, err := New(strings.NewReader(input))
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if ign == nil {
			t.Fatal("New() returned nil")
		}
	})

	t.Run("empty reader", func(t *testing.T) {
		t.Parallel()

		ign, err := New(strings.NewReader(""))
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if ign == nil {
			t.Fatal("New() returned nil")
		}
	})

	t.Run("only comments and blanks", func(t *testing.T) {
		t.Parallel()

		input := "# comment\n\n# another comment\n   "
		ign, err := New(strings.NewReader(input))
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if ign == nil {
			t.Fatal("New() returned nil")
		}
	})
}

func TestNewFromFile(t *testing.T) {
	t.Parallel()

	t.Run("valid file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ignorePath := filepath.Join(dir, ".smerkleignore")
		if err := os.WriteFile(ignorePath, []byte("*.log\nbuild/"), 0o600); err != nil {
			t.Fatalf("failed to create ignore file: %v", err)
		}

		ign, err := NewFromFile(ignorePath)
		if err != nil {
			t.Fatalf("NewFromFile() error = %v", err)
		}
		if ign == nil {
			t.Fatal("NewFromFile() returned nil")
		}

		if !ign.Match("debug.log", false) {
			t.Error("expected *.log to match debug.log")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		_, err := NewFromFile("/nonexistent/.smerkleignore")
		if err == nil {
			t.Fatal("NewFromFile() expected error for missing file")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ignorePath := filepath.Join(dir, ".smerkleignore")
		if err := os.WriteFile(ignorePath, []byte(""), 0o600); err != nil {
			t.Fatalf("failed to create ignore file: %v", err)
		}

		ign, err := NewFromFile(ignorePath)
		if err != nil {
			t.Fatalf("NewFromFile() error = %v", err)
		}
		if ign == nil {
			t.Fatal("NewFromFile() returned nil")
		}
	})
}

func TestIgnorerMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		patterns string
		path     string
		isDir    bool
		want     bool
	}{
		{
			name:     "simple glob match",
			patterns: "*.log",
			path:     "debug.log",
			isDir:    false,
			want:     true,
		},
		{
			name:     "simple glob no match",
			patterns: "*.log",
			path:     "main.go",
			isDir:    false,
			want:     false,
		},
		{
			name:     "glob matches in subdir",
			patterns: "*.log",
			path:     "logs/debug.log",
			isDir:    false,
			want:     true,
		},
		{
			name:     "directory pattern matches dir",
			patterns: "build/",
			path:     "build",
			isDir:    true,
			want:     true,
		},
		{
			name:     "directory pattern does not match file",
			patterns: "build/",
			path:     "build",
			isDir:    false,
			want:     false,
		},
		{
			name:     "multiple patterns first matches",
			patterns: "*.log\n*.tmp",
			path:     "debug.log",
			isDir:    false,
			want:     true,
		},
		{
			name:     "multiple patterns second matches",
			patterns: "*.log\n*.tmp",
			path:     "cache.tmp",
			isDir:    false,
			want:     true,
		},
		{
			name:     "multiple patterns none match",
			patterns: "*.log\n*.tmp",
			path:     "main.go",
			isDir:    false,
			want:     false,
		},
		{
			name:     "no patterns matches nothing",
			patterns: "",
			path:     "anything.txt",
			isDir:    false,
			want:     false,
		},
		{
			name:     "only comments matches nothing",
			patterns: "# comment\n# another",
			path:     "anything.txt",
			isDir:    false,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ign := mustNew(t, tt.patterns)
			got := ign.Match(tt.path, tt.isDir)
			if got != tt.want {
				t.Errorf("Match(%q, isDir=%v) = %v, want %v",
					tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestNegationPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		patterns []string
		path     string
		isDir    bool
		want     bool // true = ignored
	}{
		{
			name:     "simple ignore",
			patterns: []string{"*.log"},
			path:     "debug.log",
			isDir:    false,
			want:     true,
		},
		{
			name:     "negation unignores",
			patterns: []string{"*.log", "!important.log"},
			path:     "important.log",
			isDir:    false,
			want:     false,
		},
		{
			name:     "negation only affects matched files",
			patterns: []string{"*.log", "!important.log"},
			path:     "debug.log",
			isDir:    false,
			want:     true,
		},
		{
			name:     "re-ignore after negation",
			patterns: []string{"*.log", "!important.log", "important.log"},
			path:     "important.log",
			isDir:    false,
			want:     true,
		},
		{
			name:     "negation before ignore has no effect",
			patterns: []string{"!*.log", "*.log"},
			path:     "debug.log",
			isDir:    false,
			want:     true,
		},
		{
			name:     "directory negation",
			patterns: []string{"build/", "!build/keep/"},
			path:     "build/keep",
			isDir:    true,
			want:     false,
		},
		{
			name:     "negation with glob",
			patterns: []string{"*.txt", "!important*.txt"},
			path:     "important_notes.txt",
			isDir:    false,
			want:     false,
		},
		{
			name:     "negation with doublestar",
			patterns: []string{"**/test/**", "!**/test/fixtures/**"},
			path:     "pkg/test/fixtures/data.json",
			isDir:    false,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := strings.Join(tt.patterns, "\n")
			ign := mustNew(t, input)

			got := ign.Match(tt.path, tt.isDir)
			if got != tt.want {
				t.Errorf("Match(%q, isDir=%v) = %v, want %v",
					tt.path, tt.isDir, got, tt.want)
			}
		})
	}
}

func TestIgnorerIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		patterns string
		checks   []struct {
			path  string
			isDir bool
			want  bool
		}
	}{
		{
			name:     "typical gitignore",
			patterns: "# Build output\nbuild/\ndist/\n\n# Dependencies\nnode_modules/\nvendor/\n\n# Logs\n*.log\n!important.log\n\n# IDE\n.idea/\n.vscode/\n*.swp",
			checks: []struct {
				path  string
				isDir bool
				want  bool
			}{
				{"build", true, true},
				{"src/build", true, true},
				{"node_modules", true, true},
				{"project/node_modules", true, true},
				{"debug.log", false, true},
				{"logs/error.log", false, true},
				{"important.log", false, false},
				{".idea", true, true},
				{"main.go", false, false},
				{"src/main.go", false, false},
				{"file.swp", false, true},
			},
		},
		{
			name:     "doublestar patterns",
			patterns: "**/test/**\n**/node_modules/**\ndocs/**/*.md",
			checks: []struct {
				path  string
				isDir bool
				want  bool
			}{
				{"test/unit.go", false, true},
				{"src/test/unit.go", false, true},
				{"src/pkg/test/unit.go", false, true},
				{"testing/unit.go", false, false},
				{"node_modules/pkg/index.js", false, true},
				{"app/node_modules/pkg/index.js", false, true},
				{"docs/readme.md", false, true},
				{"docs/api/reference.md", false, true},
				{"docs/readme.txt", false, false},
			},
		},
		{
			name:     "anchored patterns",
			patterns: "/root.txt\n/build/\nsrc/generated/",
			checks: []struct {
				path  string
				isDir bool
				want  bool
			}{
				{"root.txt", false, true},
				{"subdir/root.txt", false, false},
				{"build", true, true},
				{"subdir/build", true, false},
				{"src/generated", true, true},
				{"other/src/generated", true, false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ign := mustNew(t, tt.patterns)

			for _, check := range tt.checks {
				got := ign.Match(check.path, check.isDir)
				if got != check.want {
					t.Errorf("Match(%q, isDir=%v) = %v, want %v",
						check.path, check.isDir, got, check.want)
				}
			}
		})
	}
}

func TestMatchResult(t *testing.T) {
	t.Parallel()

	input := "*.log\n!important.log\nbuild/"
	ign := mustNew(t, input)

	tests := []struct {
		path       string
		isDir      bool
		wantIgnore bool
		wantNegate bool
		wantLine   int
	}{
		{"debug.log", false, true, false, 1},
		{"important.log", false, false, true, 2},
		{"build", true, true, false, 3},
		{"main.go", false, false, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			result := ign.MatchResult(tt.path, tt.isDir)
			if result.Ignored != tt.wantIgnore {
				t.Errorf("Ignored = %v, want %v", result.Ignored, tt.wantIgnore)
			}
			if result.Negated != tt.wantNegate {
				t.Errorf("Negated = %v, want %v", result.Negated, tt.wantNegate)
			}
			if tt.wantLine > 0 && result.LineNumber != tt.wantLine {
				t.Errorf("LineNumber = %d, want %d", result.LineNumber, tt.wantLine)
			}
		})
	}
}
