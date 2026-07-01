# Skilldrop Implementation Plan

## Assumptions

- The documented YAML files are the source of truth for Phase 1.
- Agent config examples only show destination paths, while command examples use agent IDs. Phase 1 supports both forms:
  - `- .codex/skills`
  - `- id: codex`
    `path: .codex/skills`
- Interactive TUI, repository onboarding, conflict rename flows, and shell completion are later phases.
- Phase 1 avoids speculative abstractions and implements the smallest usable non-interactive drop path.

## Phase 1: Core CLI, Config, Catalog, and Safe Drop

Status: complete.

Goal: provide a script-friendly vertical slice over existing config and cached repositories.

1. Implement command entrypoint and help.
   - Verify: `go test ./...`
   - Verify: `go run . --help`
2. Load XDG config and cache locations.
   - Read repo configs from `~/.config/skilldrop/repos/*.yaml`.
   - Read agents from `~/.config/skilldrop/agents.yaml`.
   - Automatically create `~/.config/skilldrop/`, `repos/`, `agents.yaml`, `~/.cache/skilldrop/`, and `catalogs/` when missing.
   - Verify: tests use temporary `XDG_CONFIG_HOME` and `XDG_CACHE_HOME`.
3. Build a catalog from repo configs.
   - Support `skilldrop list` and `skilldrop list skills`.
   - Support `--json`.
   - Verify: tests cover text and JSON output.
4. Implement non-interactive `drop`.
   - Resolve skill and agent.
   - Copy the full source folder from the cached repo into the current workspace.
   - Refuse differing existing files unless `--force` is set.
   - Support `--dry-run`, `--json`, and `--no-interactive`.
   - Verify: tests cover dry-run, successful writes, unchanged files, conflicts, and force.

## Phase 2: Repository Add and Scan

Goal: make catalog setup possible from the CLI without manually writing YAML.

Status: complete for non-interactive CLI onboarding. Interactive review and rename flows remain in Phase 3.

1. Implement `repo add <git-url> [--branch main] [--id <repo-id>]`.
   - Status: complete.
   - Verify: tests cover local Git repository onboarding.
2. Clone or update repos under `~/.cache/skilldrop/catalogs/<repo-id>/`.
   - Status: complete.
   - Verify: tests cover initial clone and existing clone update.
3. Scan for `SKILL.md`, parse front matter, and write repo config.
   - Status: complete.
   - Verify: tests cover discovered skill names and source paths.
4. Add conflict detection for duplicate skill names.
   - Status: complete.
   - Verify: tests cover conflicts with existing repo configs.
5. Add non-interactive flags for accepting discovered skills.
   - Status: complete.
   - Verify: `repo add ... --no-interactive` completes without prompts.

## Phase 3: Interactive TUI

Goal: provide human-friendly setup and catalog management over the Phase 1 and 2 engine.

Status: in progress. The full-screen tabbed shell, read-only catalog, agent add/remove flow, and repo add/register flow are complete; pickup and richer conflict review screens are still pending.

1. Add Bubble Tea and Lip Gloss TUI shell.
   - Status: complete.
   - Verify: tests cover TUI model navigation, tab rendering, fresh-storage startup, and command wiring builds.
2. Implement read-only catalog browser.
   - Status: complete.
   - Verify: tests cover catalog navigation without dropping skills.
3. Implement agent list/add/remove flow.
   - Status: complete.
   - Verify: tests cover adding and removing agent paths through the TUI model.
4. Implement repo list/add flow.
   - Status: complete.
   - Verify: tests cover adding a Git repo through the TUI model, scanning skills, writing repo config, and refreshing the catalog.
5. Implement skill detail, pickup, and conflict review screens.
   - Status: pending.
   - Verify: future focused tests per screen.
6. Keep every TUI action backed by the same core operations used by CLI commands.
   - Status: complete for repo add and config-backed agent management.
   - Verify: TUI repo add uses the same `RepoAdd` core operation as CLI repo add.

## Phase 4: Pickup, Completion, and Polish

Goal: complete the remaining command surface.

Status: in progress. Pickup is complete; completion, broader error polish, and docs remain.

1. Implement `pickup`.
   - Status: complete.
   - Verify: tests cover success, dry-run, missing destination, local-change refusal, and force removal.
2. Implement shell completion for commands, flags, skills, and agents.
   - Status: pending.
3. Harden exit codes and JSON errors across all commands.
   - Status: pending.
4. Expand documentation and examples.
   - Status: pending.
