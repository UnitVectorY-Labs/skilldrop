---
layout: default
title: Usage
nav_order: 2
permalink: /usage/
---

# Usage

`skilldrop` manages Git-backed skill catalogs and drops selected skills into agent-specific workspace paths.

```bash
skilldrop <command> [flags]
```

## Commands

| Command | Purpose |
| --- | --- |
| `skilldrop list [skills]` | List configured skills. |
| `skilldrop repo add <git-url>` | Register and scan a Git repository for skills. |
| `skilldrop drop` | Copy a configured skill into an agent destination. |
| `skilldrop pickup` | Remove a previously dropped skill from an agent destination. |
| `skilldrop tui` | Open the interactive terminal interface. |
| `skilldrop help` | Show command usage. |
| `skilldrop version` | Print the current version. |

## Storage

`skilldrop` uses XDG-style config and cache locations.

| Location | Purpose |
| --- | --- |
| `~/.config/skilldrop/agents.yaml` | Agent destination configuration. |
| `~/.config/skilldrop/repos/*.yaml` | Registered repository configuration. |
| `~/.cache/skilldrop/catalogs/<repo-id>/` | Cached Git repository clones. |

Operational commands automatically create the required config and cache directories when they are missing.

## `list`

List configured skills.

```bash
skilldrop list
skilldrop list skills
skilldrop list skills --json
```

Flags:

| Flag | Description |
| --- | --- |
| `--json` | Print machine-readable JSON output. |

## `repo add`

Register a Git repository, clone it into the local cache, scan for `SKILL.md` files, and write a repository config.

```bash
skilldrop repo add <git-url> --id <repo-id>
skilldrop repo add <git-url> --id <repo-id> --branch main --json
```

Flags:

| Flag | Description |
| --- | --- |
| `--id <repo-id>` | Set the stable repository ID used for config and cache paths. |
| `--branch <branch>` | Clone or update from a branch. Defaults to `main`. |
| `--json` | Print machine-readable JSON output. |
| `--no-interactive` | Run without prompts. Accepted for consistency with other commands. |

## `drop`

Copy an enabled catalog skill into one configured agent destination.

```bash
skilldrop drop --skill <skill-name> --agent <agent-id>
skilldrop drop
skilldrop drop --skill <skill-name> --agent <agent-id> --dry-run
skilldrop drop --skill <skill-name> --agent <agent-id> --force
skilldrop drop --skill <skill-name> --agent <agent-id> --json --no-interactive
```

Flags:

| Flag | Description |
| --- | --- |
| `--skill <skill-name>` | Select the catalog skill to drop. |
| `--agent <agent-id>` | Select the configured agent destination. |
| `--force` | Overwrite destination files with different content. |
| `--dry-run` | Report what would happen without writing files. |
| `--json` | Print machine-readable JSON output. |
| `--no-interactive` | Run without prompts. Missing or ambiguous inputs fail. |
| `--interactive` | Reserved for interactive behavior. |

By default, `drop` refuses to overwrite destination files with different content. Existing identical files are left unchanged.

When `--skill` is omitted in an interactive terminal, `drop` shows an inline list of registered catalog skills. Use `up`/`down` or `j`/`k` to move, `enter` to select, and `esc` to cancel. When `--agent` is omitted and multiple agents are configured, `drop` shows the same inline selector for agents. `--json` and `--no-interactive` never prompt.

Human-readable `drop` output lists each copied file relative to the skill destination with a fixed-width status tag:

```text
[add    ] examples/example.txt
[same   ] SKILL.md
[updated] scripts/install.sh
```

## `pickup`

Remove a previously dropped skill from an agent destination.

```bash
skilldrop pickup --skill <skill-name> --agent <agent-id>
skilldrop pickup --skill <skill-name> --agent <agent-id> --dry-run
skilldrop pickup --skill <skill-name> --agent <agent-id> --force
skilldrop pickup --skill <skill-name> --agent <agent-id> --json --no-interactive
```

Flags:

| Flag | Description |
| --- | --- |
| `--skill <skill-name>` | Select the installed skill to remove. |
| `--agent <agent-id>` | Select the configured agent destination. |
| `--force` | Remove the installed skill even when local changes are detected. |
| `--dry-run` | Report what would happen without removing files. |
| `--json` | Print machine-readable JSON output. |
| `--no-interactive` | Run without prompts. Missing or ambiguous inputs fail. |
| `--interactive` | Reserved for interactive behavior. |

By default, `pickup` refuses to remove an installed skill when local changes are detected.

## `tui`

Open the full-screen terminal interface.

```bash
skilldrop tui
```

The TUI currently provides tabs for:

| Tab | Purpose |
| --- | --- |
| `Catalog` | View enabled catalog skills in a table and disable a skill. The skill source column is labeled `Repo Path`. |
| `Repos` | View registered repositories in a table, add a new repository, review discovered skills, open repo details, sync from Git, and toggle discovered skills on or off. Rows show `details` in the Action column because they open a detail screen. |
| `Agents` | View configured agents in a table, add an agent path, or remove an agent path. |

Keyboard controls:

| Key | Action |
| --- | --- |
| `left`, `right`, `h`, `l` | Move between tabs. |
| `up`, `down`, `j`, `k` | Move within the focused list. |
| `a` | Add an agent path on the Agents tab or start adding a repository on the Repos tab. |
| `d` | Disable the selected Catalog skill or remove the selected agent path on the Agents tab. |
| `enter` | Open repo details, advance add forms, or register selected discovered skills. |
| `space` | Toggle a discovered skill on review/detail screens. |
| `s` | Sync the selected repo from Git on the repo detail screen. |
| `q`, `esc`, `ctrl+c` | Quit. |

The TUI also supports mouse hover and click targets on the tab headers and table rows. Clicking a tab switches to it. Clicking a row selects it; on the Repos tab, clicking a repository row opens its detail screen, and clicking a repo detail skill row toggles that skill on or off.

## `help`

Show usage information.

```bash
skilldrop help
skilldrop --help
```

## `version`

Print the current version.

```bash
skilldrop version
skilldrop --version
```
