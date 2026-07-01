# Skilldrop Requirements Document

## 1. Product Overview

Skilldrop is a Go-based command line application with both a script-friendly CLI and an interactive TUI. Its purpose is to let users maintain a global catalog of Git-backed skills and drop selected skills into the correct agent-specific folder inside any workspace.

A skill is content pulled from a Git repository. It may be a single file or a full directory tree, but once discovered, skilldrop treats the entire containing folder as the skill payload.

The core user goal is:

```text
Register skill repositories once.
Scan them into a unified global catalog.
Go into any workspace.
Drop a selected skill into a selected agent path.
```

Skilldrop should be usable by humans, scripts, and AI agents.

## 2. Core Concepts

### Skill

A skill is a folder from a registered Git repository that contains a `SKILL.md` file.

The entire folder containing `SKILL.md` is considered the skill. Skilldrop never drops only the `SKILL.md` file unless the skill folder itself only contains that file.

Example:

```text
repo/
  skills/
    go-cli-builder/
      SKILL.md
      examples/
      scripts/
```

Skill payload:

```text
skills/go-cli-builder/
```

The source repository may contain skills in a typical agent skill folder as in that repository is using the skill or the repository may contain a folder with one or multiple skills that are just maintained there for cataloging and dropping into other workspaces.

### Catalog

Skilldrop exposes one global catalog of skills.

Internally, the catalog may be backed by many Git repositories, but that implementation detail should be hidden during normal use. Users should not need to remember which repository a skill came from when dropping it.

The assumption here is that the names of the skills are unique across all registered repositories. If a name conflict is detected, the user should be prompted to resolve it during onboarding.

### Registered Repository

A registered repository is a Git repo that skilldrop clones into its local cache and scans for skills.

Each repository has its own config file.

The cloned Git repositories are stored under `~/.cache/skilldrop/catalogs/<repo-id>/`.

The config file for each registered repository is stored under `~/.config/skilldrop/repos/<repo-id>.yaml`.

### Agent

An agent is a named destination profile. Each agent defines where skills should be placed relative to the current workspace.

Examples:

```text
`claude-code`.   -> `.claude/skills/`
`cursor`         -> `.cursor/skills/`
`codex`          -> `.codex/skills/`
`github-copilot` -> `.github/skills/`
```

As part of the setup the user can configure one or more agent paths that are defined in `~/.config/skilldrop/agents.yaml`.

A single drop operation should target exactly one agent destination. Multiple drops can be performed in sequence to place the same skill into multiple agent destinations.

## 3. File System Layout

Skilldrop should use XDG-style locations.

### Config

```text
~/.config/skilldrop/
  config.yaml
  repos/
    personal.yaml
    team.yaml
    external.yaml
  agents.yaml
```

### Cache

```text
~/.cache/skilldrop/
  catalogs/
    personal/
      <git clone>
    team/
      <git clone>
    external/
      <git clone>
```

Notes:

- `~/.config/skilldrop/` stores durable user configuration.
- `~/.cache/skilldrop/` stores cloned repositories and derived cache data.
- Cached Git repos can be deleted and rebuilt from config.
- Config should be treated as the source of truth.

## 4. Technology Requirements

Skilldrop is written in Go.


The TUI should use Bubble Tea and Lip Gloss. Bubble Tea is a Go TUI framework based on The Elm Architecture, and Lip Gloss provides declarative terminal styling and layout primitives. ([GitHub](https://github.com/charmbracelet/bubbletea))

Recommended stack:

```text
Language: Go
TUI: Bubble Tea
Styling: Lip Gloss
Config: YAML
Git operations: go-git or shelling out to git
Front matter parsing: YAML front matter parser or custom parser
```

## 5. Configuration Model

### Repo Config

Each registered repo gets one file under:

```text
~/.config/skilldrop/repos/<repo-id>.yaml
```

Example:

```yaml
version: 1

id: personal
name: Personal Skills

git:
  url: git@github.com:jared/personal-skills.git
  branch: main

skills:
  - name: go-cli-builder
    source_path: skills/go-cli-builder
    enabled: true

  - name: repo-auditor
    source_path: skills/repo-auditor
    enabled: true
```

### Agent Config

All agents are defined in a single file:

```text
~/.config/skilldrop/agents.yaml
```

Example:

```yaml
version: 1

agents:
  - .claude/skills
  - .cursor/skills
  - .codex/skills
  - .github/skills
```

Destination paths are relative to the current working directory unless explicitly configured otherwise in a future version.

## 6. Catalog Scanning

Skilldrop should scan registered repositories for `SKILL.md` files.

Default scan behavior:

```text
Find every SKILL.md file.
Treat the containing folder as the skill source path.
Read the front matter from SKILL.md.
Use the front matter name as the default skill name.
Present discovered skills for review during interactive onboarding.
```

### Onboarding Flow

When adding a repository interactively:

```bash
skilldrop repo add git@github.com:org/skills.git
```

You can optionally provide `--branch` to override the default branch which is `main`.

Skilldrop should:

1. Clone the repo into the cache.
2. Scan for `SKILL.md` files.
3. Present detected skills in a TUI review screen.
4. Allow the user to check or uncheck discovered skills.
5. Detect naming conflicts.
6. Allow conflict resolution before saving.
7. Write accepted skills to the repo config file.

Example TUI review:

```text
Found 14 possible skills

[x] go-cli-builder       skills/go-cli-builder/SKILL.md
[x] repo-auditor         skills/repo-auditor/SKILL.md
[ ] old-python-helper    archive/python-helper/SKILL.md
[x] docx-editor          tools/docx/SKILL.md
```

## 7. Skill Naming and Front Matter

After onboarding, the skill name used by Skilldrop should be the same as the skill front matter name.

The skill name is the stable identifier used by:

```bash
skilldrop drop --skill <skill-name>
skilldrop show skill <skill-name>
skilldrop pickup --skill <skill-name>
```

### Rename During Onboarding

If a skill name conflicts with an existing skill the user should be prompted to rename the skill during onboarding. Optionally a user should be presented with the option to rename any skill during onboarding if they wish.

The renamed skill will be stored in the destination under the renamed skill name and the frontmatter name will be rewritten to match the renamed skill name.

## 8. Drop Behavior

The primary operation is dropping a skill into a workspace.

Example:

```bash
skilldrop drop --skill go-cli-builder --agent claude-code
```

The destination folder name must match the final skill name.

Example:

```text
Current workspace:
  /projects/my-app

Agent destination:
  .claude/skills

Skill:
  go-cli-builder

Final destination:
  /projects/my-app/.claude/skills/go-cli-builder/
```

Skilldrop should copy the entire skill source folder into the destination.

## 9. Overwrite and Conflict Behavior

Skilldrop must not overwrite different existing content unless `--force` is provided.

Default behavior:

```text
If a destination file does not exist:
  write it

If a destination file exists and has identical content:
  allow it

If a destination file exists and has different content:
  fail unless --force is set
```

This applies per file.

Example failure:

```text
Refusing to overwrite existing file with different content:

  .claude/skills/go-cli-builder/SKILL.md

Use --force to overwrite.
```

With force:

```bash
skilldrop drop --skill go-cli-builder --agent claude-code --force
```

Skilldrop may overwrite files with different content.

The goal is for the command to be atomic with a check then apply, on the check phase if any file would overwrite different content the command should fail and not write any files.

### Directory Conflicts

If the destination directory exists, Skilldrop should compare files recursively.

It should fail if any file would overwrite different content without `--force`.

It may create missing files and missing directories.

It should not delete extra files unless a future `--prune` flag is added.

## 10. CLI and Interactive Split

Skilldrop has two surfaces over the same core engine:

```text
Interactive TUI surface:
  Guided setup
  Repo onboarding
  Skill selection
  Agent selection
  Conflict resolution

Command-line surface:
  Stable commands
  Stable flags
  Predictable exit codes
  JSON output
  Shell completion
```

Core rule:

```text
Every interactive action should have an equivalent command-line operation.
Every command-line operation should be usable without prompts when all required inputs are provided.
```

### Drop Interaction Rules

`drop` has two important selectors:

```text
--skill
--agent
```

Behavior:

```text
--skill provided and --agent provided:
  run non-interactively

--skill omitted:
  open interactive skill picker

--agent omitted and exactly one agent is configured:
  use the only configured agent

--agent omitted and multiple agents are configured:
  open interactive agent picker

--agent omitted and no agents are configured:
  fail with setup guidance, unless running a broader interactive setup flow
```

Examples:

```bash
skilldrop drop --skill go-cli-builder --agent claude-code
```

Runs without prompting.

```bash
skilldrop drop --skill go-cli-builder
```

Uses the only configured agent, or prompts if multiple agents exist.

```bash
skilldrop drop --agent claude-code
```

Prompts for skill.

```bash
skilldrop drop
```

Prompts for skill and agent.

### Strict Mode

For scripts and AI agents, Skilldrop should support no-prompt behavior.

```bash
skilldrop drop --skill go-cli-builder --agent claude-code --no-interactive
```

In strict mode:

```text
No prompts are allowed.
Missing or ambiguous inputs cause a non-zero exit.
Errors should be explicit and machine-readable with --json.
```

The purpose of `--no-interactive` is to allow scripts and AI agents to run Skilldrop without human intervention where the default behavior would be to prompt for missing inputs.

## 11. JSON Output

Commands should support `--json` for AI agents and scripts.

Example:

```bash
skilldrop list skills --json
```

Output:

```json
{
  "skills": [
    {
      "name": "go-cli-builder",
      "repo": "personal",
      "source_path": "skills/go-cli-builder",
      "enabled": true
    }
  ]
}
```

Drop success:

```json
{
  "status": "dropped",
  "skill": "go-cli-builder",
  "agent": "claude-code",
  "destination": ".claude/skills/go-cli-builder",
  "files_written": 4,
  "files_unchanged": 1,
  "files_overwritten": 0
}
```

Drop conflict:

```json
{
  "status": "error",
  "error": "would_overwrite",
  "message": "Refusing to overwrite existing file with different content.",
  "path": ".claude/skills/go-cli-builder/SKILL.md",
  "hint": "Use --force to overwrite."
}
```

## 12. Proposed Commands

### Root

```bash
skilldrop
```

Prints help and usage information.  Equivalent to `skilldrop --help`.

### TUI

```bash
skilldrop tui
```

Launches the TUI. The TUI allows for configuring the agents as well as onboarding new repositories and skills walking through that onboarding process.

### Catalog

```bash
skilldrop list
```

Lists all of the skills.

### Drop

The drop command is the primary operation of skilldrop.  This is the purpose of this application. To drop skills into a workspace.  If the `--skill` and `--agent` flags are provided, the command should run non-interactively.  If either of those flags are omitted, the command should prompt for the missing input.

```bash
skilldrop drop --skill <skill-name> --agent <agent-id>
skilldrop drop --skill <skill-name>
skilldrop drop --agent <agent-id>
skilldrop drop
```

Flags:

```text
--skill <name>
--agent <id>
--force
--dry-run
--json
--interactive
--no-interactive
```

### Pickup

Pickup removes a previously dropped skill from an agent destination. This is the inverse of a drop. By default it will prompt for confirmation before deleting any files confirming the files it will delete.  If the `--force` flag is provided, it will skip the confirmation prompt and delete the files.

```bash
skilldrop pickup --skill <skill-name> --agent <agent-id>
```

Suggested behavior:

```text
Remove the skill folder from the selected agent destination.
Fail if the folder does not exist.
Fail if the folder contains local changes unless --force is provided.
```

Pickup should mirror drop:

```text
--skill provided and --agent provided:
  run non-interactively

--skill omitted:
  prompt for installed skill

--agent omitted and one agent exists:
  use it

--agent omitted and multiple agents exist:
  prompt for agent
```

## 13. TUI Requirements

The TUI should provide the same operations as the CLI but optimized for human selection.

Required TUI screens:

```text
Home
First-time setup
Repo list
Repo add
Repo scan review
Skill catalog browser
Skill detail view
Drop skill flow
Pickup skill flow
Agent configuration
Conflict review
```


### Drop Flow

The drop flow should allow:

```text
Select skill
Select agent if needed
Preview destination
Preview files to be written
Show conflicts
Confirm drop
```

### Conflict UI

If files would be overwritten with different content:

```text
Show conflicting files
Allow cancel
Allow force overwrite after explicit confirmation
```

## 14. Shell Completion

Skilldrop should support shell completion for commands, flags, skills, and agents.

Example:

```bash
skilldrop drop --skill go<TAB>
```

Should complete known skill names.

Example:

```bash
skilldrop drop --agent cl<TAB>
```

Should complete configured agent IDs.

Recommended command shape:

```bash
skilldrop completion bash
skilldrop completion zsh
skilldrop completion fish
skilldrop completion powershell
```

The completion scripts can call back into Skilldrop to dynamically complete skills and agents from the current config. Cobra’s shell completion support is designed for this style of CLI completion. ([Cobra](https://cobra.dev/docs/how-to-guides/shell-completion/))

## 15. Dry Run

All mutating operations should support `--dry-run`.

For drop:

```bash
skilldrop drop --skill go-cli-builder --agent claude-code --dry-run
```

Should report:

```text
Source path
Destination path
Files to create
Files unchanged
Files that would overwrite
Whether operation would fail without --force
```

No files should be modified.

## 16. Exit Codes

Suggested exit codes:

```text
0  success
1  general error
2  invalid usage
3  missing configuration
4  skill not found
5  agent not found
6  overwrite conflict
7  git sync failure
8  front matter parse failure
```

For AI agents and scripts, exit codes plus `--json` should be sufficient to recover or decide the next action.
