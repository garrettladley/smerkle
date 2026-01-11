package ignore

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		wantPatterns int
	}{
		{
			name:         "empty input",
			input:        "",
			wantPatterns: 0,
		},
		{
			name:         "single pattern",
			input:        "*.log",
			wantPatterns: 1,
		},
		{
			name:         "multiple patterns",
			input:        "*.log\n*.tmp\nbuild/",
			wantPatterns: 3,
		},
		{
			name:         "blank lines ignored",
			input:        "*.log\n\n\n*.tmp",
			wantPatterns: 2,
		},
		{
			name:         "comments ignored",
			input:        "# This is a comment\n*.log\n# Another comment",
			wantPatterns: 1,
		},
		{
			name:         "trailing spaces stripped",
			input:        "*.log   \n*.tmp  ",
			wantPatterns: 2,
		},
		{
			name:         "whitespace-only lines ignored",
			input:        "   \n*.log\n\t\t\n*.tmp",
			wantPatterns: 2,
		},
		{
			name:         "escaped hash not a comment",
			input:        "\\#important.txt",
			wantPatterns: 1,
		},
		{
			name:         "negation pattern",
			input:        "*.log\n!important.log",
			wantPatterns: 2,
		},
		{
			name:         "mixed valid patterns",
			input:        "# Build artifacts\nbuild/\ndist/\n\n# But keep this\n!build/keep.txt\n\n*.log",
			wantPatterns: 4,
		},
		{
			name:         "carriage return line endings",
			input:        "*.log\r\n*.tmp\r\n",
			wantPatterns: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := strings.NewReader(tt.input)
			patterns, err := Parse(r)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			if len(patterns) != tt.wantPatterns {
				t.Errorf("Parse() got %d patterns, want %d", len(patterns), tt.wantPatterns)
				for i, p := range patterns {
					t.Logf("  pattern[%d]: %q", i, p.original)
				}
			}
		})
	}
}

func TestParseLineNumbers(t *testing.T) {
	t.Parallel()

	input := "# comment\n*.log\n\n!important.log"
	r := strings.NewReader(input)
	patterns, err := Parse(r)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	expectedLineNumbers := []int{2, 4} // *.log is line 2, !important.log is line 4
	if len(patterns) != len(expectedLineNumbers) {
		t.Fatalf("got %d patterns, want %d", len(patterns), len(expectedLineNumbers))
	}

	for i, want := range expectedLineNumbers {
		if patterns[i].lineNumber != want {
			t.Errorf("pattern[%d] lineNumber = %d, want %d", i, patterns[i].lineNumber, want)
		}
	}
}

func TestParsePatternAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantNegated bool
		wantAnchor  bool
		wantDirOnly bool
		wantPattern string
	}{
		{
			name:        "simple pattern",
			input:       "*.log",
			wantNegated: false,
			wantAnchor:  false,
			wantDirOnly: false,
			wantPattern: "*.log",
		},
		{
			name:        "negation pattern",
			input:       "!*.log",
			wantNegated: true,
			wantAnchor:  false,
			wantDirOnly: false,
			wantPattern: "*.log",
		},
		{
			name:        "anchored pattern",
			input:       "/build",
			wantNegated: false,
			wantAnchor:  true,
			wantDirOnly: false,
			wantPattern: "build",
		},
		{
			name:        "directory only pattern",
			input:       "build/",
			wantNegated: false,
			wantAnchor:  false,
			wantDirOnly: true,
			wantPattern: "build",
		},
		{
			name:        "anchored directory pattern",
			input:       "/build/",
			wantNegated: false,
			wantAnchor:  true,
			wantDirOnly: true,
			wantPattern: "build",
		},
		{
			name:        "negated anchored directory",
			input:       "!/build/",
			wantNegated: true,
			wantAnchor:  true,
			wantDirOnly: true,
			wantPattern: "build",
		},
		{
			name:        "pattern with slash is implicitly anchored",
			input:       "doc/frotz",
			wantNegated: false,
			wantAnchor:  true,
			wantDirOnly: false,
			wantPattern: "doc/frotz",
		},
		{
			name:        "doublestar not implicitly anchored",
			input:       "**/foo",
			wantNegated: false,
			wantAnchor:  false,
			wantDirOnly: false,
			wantPattern: "**/foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := strings.NewReader(tt.input)
			patterns, err := Parse(r)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			if len(patterns) != 1 {
				t.Fatalf("expected 1 pattern, got %d", len(patterns))
			}

			p := patterns[0]
			if p.negated != tt.wantNegated {
				t.Errorf("negated = %v, want %v", p.negated, tt.wantNegated)
			}
			if p.anchored != tt.wantAnchor {
				t.Errorf("anchored = %v, want %v", p.anchored, tt.wantAnchor)
			}
			if p.dirOnly != tt.wantDirOnly {
				t.Errorf("dirOnly = %v, want %v", p.dirOnly, tt.wantDirOnly)
			}
			if p.pattern != tt.wantPattern {
				t.Errorf("pattern = %q, want %q", p.pattern, tt.wantPattern)
			}
		})
	}
}

func TestParseEscaping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantPattern string
		wantNegated bool
	}{
		{
			name:        "escaped hash",
			input:       "\\#file.txt",
			wantPattern: "#file.txt",
			wantNegated: false,
		},
		{
			name:        "escaped exclamation",
			input:       "\\!important.txt",
			wantPattern: "!important.txt",
			wantNegated: false,
		},
		{
			name:        "escaped backslash",
			input:       "foo\\\\bar",
			wantPattern: "foo\\bar",
			wantNegated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := strings.NewReader(tt.input)
			patterns, err := Parse(r)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			if len(patterns) != 1 {
				t.Fatalf("expected 1 pattern, got %d", len(patterns))
			}

			p := patterns[0]
			if p.pattern != tt.wantPattern {
				t.Errorf("pattern = %q, want %q", p.pattern, tt.wantPattern)
			}
			if p.negated != tt.wantNegated {
				t.Errorf("negated = %v, want %v", p.negated, tt.wantNegated)
			}
		})
	}
}
