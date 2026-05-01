package skills

// Diagnostic describes a loading issue.
type Diagnostic struct {
	Level   string // "warning" or "error"
	Message string
	Path    string
	// Collision is set when a name collision occurs during loading.
	Collision *CollisionDiagnostic
}

// CollisionDiagnostic details a name collision between two skills.
type CollisionDiagnostic struct {
	Name       string // the skill name that collided
	WinnerPath string // path of the skill that took precedence
	LoserPath  string // path of the skill that was overridden
}

// SkillFrontmatter represents the YAML frontmatter in a SKILL.md file.
type SkillFrontmatter struct {
	Name                   string `yaml:"name"`
	Description            string `yaml:"description"`
	DisableModelInvocation bool   `yaml:"disable-model-invocation"`
}

// Skill represents a loaded skill with metadata only (not full content).
type Skill struct {
	Name                  string // validated skill name
	Description           string // validated description
	FilePath              string // absolute path to SKILL.md
	BaseDir               string // parent directory (for resolving relative paths)
	DisableModelInvocation bool  // if true, only invocable via /skill:name
	Source                string // "agents" | "crobot" | "project" | "explicit"
}

// LoadResult bundles loaded skills with diagnostics.
type LoadResult struct {
	Skills      []Skill
	Diagnostics []Diagnostic
}

// Source constants for skill loading priority.
const (
	SourceAgents   = "agents"
	SourceCrobot   = "crobot"
	SourceProject  = "project"
	SourceExplicit = "explicit"
)
