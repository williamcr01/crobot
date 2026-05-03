# Agent Skills

Crobot implements the Agent Skills specification for loading specialized instructions from SKILL.md files. Skills give the model domain-specific knowledge without bloating the system prompt.

## Skill locations

Skills are discovered in these directories, in ascending priority order (last writer wins on name collision):

```text
~/.agents/skills/         (1 — shared across agents, lowest priority)
~/.crobot/skills/         (2 — crobot-specific)
./.crobot/skills/         (3 — project-local)
--skill <path>            (4 — explicit CLI flag, highest priority)
```

Each skill is a directory containing a `SKILL.md` file:

```text
~/.crobot/skills/
  git-commit/
    SKILL.md
  obsidian-markdown/
    SKILL.md
```

## Skill file format

A SKILL.md file has YAML frontmatter followed by Markdown content:

```markdown
---
name: git-commit
description: Execute git commit with conventional commit message analysis
disable-model-invocation: false
---

# git-commit skill

Instructions for the model on how to create conventional commits...
```

### Frontmatter fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | No | Skill name. Must be lowercase a-z, 0-9, hyphens only. Defaults to parent directory name. |
| `description` | **Yes** | Short description shown in the system prompt and `/skills` list. Max 1024 characters. |
| `disable-model-invocation` | No | If `true`, the skill is not shown in the system prompt and can only be invoked manually. Defaults to `false`. |

### Validation rules

Skills that fail validation are skipped with a warning diagnostic:

- Name must be lowercase a-z, 0-9, hyphens only
- Name must not start or end with a hyphen
- Name must not contain consecutive hyphens (`--`)
- Name must match the parent directory name (if explicitly set in frontmatter)
- Name max length: 64 characters
- Description is required and max 1024 characters

### Name collisions

When two skills with the same name are loaded from different sources, the higher-priority skill wins. A warning diagnostic is emitted for the collision.

Duplicate file paths (e.g. via symlinks) are de-duplicated silently.

## How skills work

### System prompt injection

Skills without `disable-model-invocation` are injected into the system prompt as metadata only:

```xml
<available_skills>
  <skill>
    <name>git-commit</name>
    <description>Execute git commit with conventional commit message analysis</description>
    <location>/home/user/.crobot/skills/git-commit/SKILL.md</location>
  </skill>
</available_skills>
```

The system prompt instructs the model to use the `file read` tool to load a skill's full content when the task matches its description. Relative paths in a skill file should be resolved against the skill's directory.

### Manual invocation

Users can inline-expand a skill manually with the `/skill:name` syntax:

```text
/skill:git-commit
```

This replaces the command with the full skill content (minus frontmatter) wrapped in `<skill>` tags, plus an optional user message:

```text
/skill:git-commit please create a commit
```

Skills with `disable-model-invocation: true` can only be invoked this way — they never appear in the system prompt.

## CLI flags

```text
--skill <path>    Load a skill from a directory or .md file (repeatable)
```

Can be used multiple times:

```sh
crobot --skill ~/my-skill --skill ./project-skill.md
```

If the path is a directory, Crobot scans it for a `SKILL.md` file. If it's a `.md` file, that file is loaded directly.

## Slash commands

```text
/skills          List loaded skills with names, descriptions, sources, and file paths
/skill:name      Inline-expand a skill's content into the input (with optional args)
```

## Diagnostics

Skill loading issues are printed to stderr at startup:

```text
skills warning: skill skipped: description is required (~/.crobot/skills/broken/SKILL.md)
skills warning: skill name "my-skill" collision: "~/.crobot/skills/my-skill/SKILL.md" overrides "~/.agents/skills/my-skill/SKILL.md" (~/.crobot/skills/my-skill/SKILL.md)
```
