package tools

import (
	"os"
	"path/filepath"
	"strings"
)

// IgnoreMatcher checks paths against .gitignore patterns.
// It loads .gitignore files from the root directory upward, and also
// discovers nested .gitignore files as the walk progresses.
type IgnoreMatcher struct {
	root string
	// parentRules are loaded from the root's parent chain (ancestor .gitignore).
	// These apply to all paths below root.
	parentRules []ruleSet
	// dirRules caches per-directory parsed rules.
	dirRules map[string]ruleSet
	// allRules is a flat ordered list of all rules encountered so far.
	// Rules from parent .gitignore files come first, then root, then nested.
	// The last matching rule determines the result.
	allRules []rule
}

type ruleSet struct {
	dir   string
	rules []rule
}

type rule struct {
	pattern     string
	negate      bool
	isDirOnly   bool       // ends with /
	hasPathSep  bool       // contains / (excluding trailing /)
	isDoubleStar bool      // contains **
	dir         string     // gitignore location relative to root, e.g. "sub" or "" for root
}

// NewIgnoreMatcher creates an IgnoreMatcher rooted at the given directory.
func NewIgnoreMatcher(root string) (*IgnoreMatcher, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	m := &IgnoreMatcher{
		root:     root,
		dirRules: make(map[string]ruleSet),
	}
	// Walk from root up to filesystem root, collecting ancestor .gitignore files.
	m.parentRules = loadAncestorRules(root)
	// Flatten parent rules into allRules so they're checked first.
	for _, rs := range m.parentRules {
		m.allRules = append(m.allRules, rs.rules...)
	}
	// Load the root .gitignore.
	m.EnsureRulesLoaded("")
	return m, nil
}

// ShouldIgnore reports whether the given path (relative to root) should be ignored.
func (m *IgnoreMatcher) ShouldIgnore(relPath string, isDir bool) bool {
	// Always ignore .git directory.
	if relPath == ".git" || strings.HasPrefix(relPath, ".git/") {
		return true
	}

	// Ensure nested .gitignore files along the path are loaded.
	parts := strings.Split(relPath, string(filepath.Separator))
	for i := 0; i < len(parts); i++ {
		dir := strings.Join(parts[:i], string(filepath.Separator))
		m.EnsureRulesLoaded(dir)
	}

	negated := false
	matched := false

	for _, r := range m.allRules {
		if matchRule(r, relPath, isDir) {
			matched = true
			negated = r.negate
		}
	}

	if matched {
		return !negated
	}
	return false
}

// EnsureRulesLoaded ensures .gitignore rules for a given subdirectory are loaded.
func (m *IgnoreMatcher) EnsureRulesLoaded(dirRel string) {
	if _, ok := m.dirRules[dirRel]; ok {
		return
	}
	gitignorePath := filepath.Join(m.root, dirRel, ".gitignore")
	rs := loadRules(dirRel, gitignorePath)
	m.dirRules[dirRel] = rs
	m.allRules = append(m.allRules, rs.rules...)
}

// matchRule tests a single rule against a path.
// Rules from a nested .gitignore (r.dir != "") only match paths under that directory.
func matchRule(r rule, relPath string, isDir bool) bool {
	// Scope check: rules from a nested .gitignore only apply to paths under that directory.
	if r.dir != "" {
		if relPath == r.dir || strings.HasPrefix(relPath, r.dir+"/") {
			// OK, this path is under the rule's directory.
			// Strip the dir prefix for relative matching.
			if relPath == r.dir {
				relPath = filepath.Base(relPath)
			} else {
				relPath = strings.TrimPrefix(relPath, r.dir+"/")
			}
		} else {
			return false // path is not under this rule's directory
		}
	}

	pat := r.pattern

	// If the rule is directory-only (ends with /).
	// It matches the named directory and everything inside it.
	if r.isDirOnly {
		patNoSlash := strings.TrimSuffix(pat, "/")
		parts := strings.Split(relPath, string(filepath.Separator))
		for i := 0; i < len(parts); i++ {
			var matched bool
			if r.hasPathSep {
				prefix := strings.Join(parts[:i+1], string(filepath.Separator))
				matched = filepathMatch(patNoSlash, prefix)
			} else {
				matched = filepathMatch(patNoSlash, parts[i])
			}
			if matched {
				// If this is the last path component, check isDir.
				// For intermediate components (directory contents), always match.
				if i == len(parts)-1 {
					return isDir
				}
				return true
			}
		}
		return false
	}

	// Non-directory-only pattern.
	if r.hasPathSep || r.isDoubleStar {
		return filepathMatch(pat, relPath)
	}

	// Pattern without / matches basename.
	return filepathMatch(pat, filepath.Base(relPath))
}

// filepathMatch wraps filepath.Match with some gitignore-friendly behaviors.
func filepathMatch(pattern, path string) bool {
	// Handle ** patterns.
	if strings.Contains(pattern, "**") {
		return matchDoubleStar(pattern, path)
	}

	// Try direct match.
	match, _ := filepath.Match(pattern, path)
	if match {
		return true
	}

	// If pattern doesn't have /, also try matching with **/ prefix
	// (gitignore semantics: patterns without / match anywhere in the tree).
	if !strings.Contains(pattern, "/") && !strings.HasSuffix(pattern, "/") {
		match, _ = filepath.Match("**/"+pattern, path)
		return match
	}

	return false
}

// matchDoubleStar handles ** glob patterns.
func matchDoubleStar(pattern, path string) bool {
	parts := strings.Split(pattern, "**")
	if len(parts) < 2 {
		match, _ := filepath.Match(pattern, path)
		return match
	}

	prefix := parts[0]
	suffix := parts[1]

	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := path[len(prefix):]

	if suffix == "" {
		return true // pattern like prefix/**
	}

	if !strings.HasSuffix(rest, suffix) {
		return false
	}
	// The middle part can be anything, including empty and /.
	return true
}

// loadAncestorRules walks up from dir to the filesystem root, collecting .gitignore rules.
func loadAncestorRules(dir string) []ruleSet {
	var result []ruleSet
	current := dir
	for {
		parent := filepath.Dir(current)
		if parent == current {
			break // reached filesystem root
		}
		gitignorePath := filepath.Join(parent, ".gitignore")
		relDir, _ := filepath.Rel(dir, parent)
		rs := loadRules(relDir, gitignorePath)
		if len(rs.rules) > 0 {
			result = append(result, rs)
		}
		current = parent
	}
	return result
}

// loadRules parses a .gitignore file, returning rules relative to the given dir.
func loadRules(dir, gitignorePath string) ruleSet {
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		return ruleSet{dir: dir}
	}

	var rules []rule
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = line[1:]
		}

		// Strip trailing whitespace.
		line = strings.TrimRight(line, " \t")

		// Strip leading / for patterns like /foo (relative to gitignore dir).
		if strings.HasPrefix(line, "/") {
			line = line[1:]
		}

		isDirOnly := strings.HasSuffix(line, "/")

		// Determine if pattern has a path separator (excluding trailing /).
		hasPathSep := false
		checkStr := line
		if isDirOnly && len(line) > 1 {
			checkStr = line[:len(line)-1]
		}
		hasPathSep = strings.Contains(checkStr, "/")

		isDoubleStar := strings.Contains(line, "**")

		rules = append(rules, rule{
			pattern:      line,
			negate:       negate,
			isDirOnly:    isDirOnly,
			hasPathSep:   hasPathSep,
			isDoubleStar: isDoubleStar,
			dir:          dir,
		})
	}

	return ruleSet{dir: dir, rules: rules}
}
