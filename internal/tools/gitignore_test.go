package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIgnoreMatcher_Basic(t *testing.T) {
	dir := t.TempDir()
	// Create a .gitignore at the root.
	gi := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gi, []byte("*.log\nbuild/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := NewIgnoreMatcher(dir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"foo.go", false, false},
		{"foo.log", false, true},
		{"bar/baz.log", false, true},
		{"build", true, true},
		{"build/output.exe", false, true},
		{"src/main.go", false, false},
		{".git", true, true},
		{".git/config", false, true},
	}

	for _, tt := range tests {
		got := m.ShouldIgnore(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("ShouldIgnore(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestIgnoreMatcher_Negation(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gi, []byte("*.log\n!important.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := NewIgnoreMatcher(dir)
	if err != nil {
		t.Fatal(err)
	}

	if !m.ShouldIgnore("debug.log", false) {
		t.Error("expected debug.log to be ignored")
	}
	if m.ShouldIgnore("important.log", false) {
		t.Error("expected important.log to NOT be ignored (negation)")
	}
}

func TestIgnoreMatcher_Nested(t *testing.T) {
	dir := t.TempDir()
	// Root .gitignore ignores nothing by default.
	rootGI := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(rootGI, []byte("*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create subdir with its own .gitignore.
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	subGI := filepath.Join(subDir, ".gitignore")
	if err := os.WriteFile(subGI, []byte("*.local\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := NewIgnoreMatcher(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Root .gitignore should catch .tmp in subdir.
	if !m.ShouldIgnore("sub/foo.tmp", false) {
		t.Error("expected sub/foo.tmp to be ignored by root .gitignore")
	}

	// Nested .gitignore should catch .local.
	if !m.ShouldIgnore("sub/foo.local", false) {
		t.Error("expected sub/foo.local to be ignored by nested .gitignore")
	}

	// Root .gitignore should NOT catch .local outside subdir.
	if m.ShouldIgnore("foo.local", false) {
		t.Error("expected foo.local to NOT be ignored (only in nested .gitignore)")
	}
}

func TestIgnoreMatcher_NoGitignore(t *testing.T) {
	dir := t.TempDir()
	m, err := NewIgnoreMatcher(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Without a .gitignore, only .git should be ignored.
	if !m.ShouldIgnore(".git", true) {
		t.Error("expected .git to be ignored")
	}
	if m.ShouldIgnore("foo.go", false) {
		t.Error("expected foo.go to NOT be ignored")
	}
}

func TestIgnoreMatcher_EmptyGitignore(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gi, []byte("# just a comment\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := NewIgnoreMatcher(dir)
	if err != nil {
		t.Fatal(err)
	}

	if m.ShouldIgnore("foo.go", false) {
		t.Error("expected foo.go to NOT be ignored with empty .gitignore")
	}
}

func TestIgnoreMatcher_DirectoryPattern(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gi, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := NewIgnoreMatcher(dir)
	if err != nil {
		t.Fatal(err)
	}

	if !m.ShouldIgnore("node_modules", true) {
		t.Error("expected node_modules/ to be ignored")
	}
	if !m.ShouldIgnore("node_modules/package/index.js", false) {
		t.Error("expected files inside node_modules to be ignored")
	}
}

func TestIgnoreMatcher_MultipleNested(t *testing.T) {
	dir := t.TempDir()

	// Root gitignore.
	gi := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gi, []byte("*.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Nested gitignore in pkg/.
	pkgDir := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pkgGI := filepath.Join(pkgDir, ".gitignore")
	if err := os.WriteFile(pkgGI, []byte("!important.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := NewIgnoreMatcher(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Root .gitignore ignores *.txt.
	if !m.ShouldIgnore("pkg/foo.txt", false) {
		t.Error("expected pkg/foo.txt to be ignored by root .gitignore")
	}
	// Nested negation should un-ignore important.txt.
	if m.ShouldIgnore("pkg/important.txt", false) {
		t.Error("expected pkg/important.txt to NOT be ignored (negation in nested .gitignore)")
	}
}

func TestIgnoreMatcher_DotGitAlwaysIgnored(t *testing.T) {
	dir := t.TempDir()
	m, err := NewIgnoreMatcher(dir)
	if err != nil {
		t.Fatal(err)
	}

	cases := []string{".git", ".git/", ".git/config", ".git/objects/pack/pack-xxx.pack"}
	for _, c := range cases {
		clean := strings.TrimSuffix(c, "/")
		if !m.ShouldIgnore(clean, c[len(c)-1:] == "/") {
			t.Errorf("expected %q to be ignored", c)
		}
	}
}

func TestMatchRule(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		{"*.go", "main.go", false, true},
		{"*.go", "main.rs", false, false},
		{"build/", "build/output.o", false, true},
		{"build/", "build", true, true},
		{"build/", "build", false, false}, // dir-only pattern: file named "build" does NOT match
		{"a/b/c", "a/b/c", false, true},
	}

	for _, tt := range tests {
		// Parse the pattern into a rule like loadRules does.
		line := tt.pattern
		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = line[1:]
		}
		isDirOnly := strings.HasSuffix(line, "/")
		hasPathSep := false
		checkStr := line
		if isDirOnly && len(line) > 1 {
			checkStr = line[:len(line)-1]
		}
		hasPathSep = strings.Contains(checkStr, "/")
		isDoubleStar := strings.Contains(line, "**")

		r := rule{
			pattern:      line,
			negate:       negate,
			isDirOnly:    isDirOnly,
			hasPathSep:   hasPathSep,
			isDoubleStar: isDoubleStar,
			dir:          "", // root-level rules
		}

		got := matchRule(r, tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("matchRule(%q, %q, isDir=%v) = %v, want %v", tt.pattern, tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestIgnoreMatcher_EnsureRulesLoaded(t *testing.T) {
	dir := t.TempDir()

	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	subGI := filepath.Join(subDir, ".gitignore")
	if err := os.WriteFile(subGI, []byte("secret*\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := NewIgnoreMatcher(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Pre-load rules for sub/.
	m.EnsureRulesLoaded("sub")

	// Now check ShouldIgnore - the rules should already be cached.
	if !m.ShouldIgnore("sub/secret.txt", false) {
		t.Error("expected sub/secret.txt to be ignored")
	}
}

func TestIgnoreMatcher_TrailingSlash(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gi, []byte("tempdir/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := NewIgnoreMatcher(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Directory with trailing slash pattern should ignore the dir and its contents.
	if !m.ShouldIgnore("tempdir", true) {
		t.Error("expected tempdir (dir) to be ignored by trailing-slash pattern")
	}
	if !m.ShouldIgnore("tempdir/foo.txt", false) {
		t.Error("expected tempdir/foo.txt to be ignored by trailing-slash pattern")
	}
	// A file with the same name should NOT be ignored - trailing slash means directory-only.
	if m.ShouldIgnore("tempdir", false) {
		t.Error("expected tempdir (file) to NOT be ignored by trailing-slash pattern")
	}
}
