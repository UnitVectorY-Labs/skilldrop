package skilldrop

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	ExitGeneral                 = 1
	ExitInvalidUsage            = 2
	ExitMissingConfiguration    = 3
	ExitSkillNotFound           = 4
	ExitAgentNotFound           = 5
	ExitOverwriteConflict       = 6
	ExitGitSyncFailure          = 7
	ExitFrontMatterParseFailure = 8
)

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return ExitGeneral
}

type App struct {
	out     io.Writer
	errOut  io.Writer
	version string
	wd      string
	paths   paths
}

func Run(args []string, out io.Writer, errOut io.Writer, version string) error {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	wd, err := os.Getwd()
	if err != nil {
		return &ExitError{Code: ExitGeneral, Err: err}
	}
	paths, err := defaultPaths()
	if err != nil {
		return &ExitError{Code: ExitMissingConfiguration, Err: err}
	}
	app := &App{out: out, errOut: errOut, version: version, wd: wd, paths: paths}
	return app.run(args)
}

func (a *App) run(args []string) error {
	if len(args) == 0 {
		a.printHelp()
		return nil
	}

	switch args[0] {
	case "-h", "--help", "help":
		a.printHelp()
		return nil
	case "--version", "version":
		fmt.Fprintln(a.out, a.version)
		return nil
	case "list":
		if err := ensureStorage(a.paths); err != nil {
			return err
		}
		return a.runList(args[1:])
	case "drop":
		if err := ensureStorage(a.paths); err != nil {
			return err
		}
		return a.runDrop(args[1:])
	case "pickup":
		if err := ensureStorage(a.paths); err != nil {
			return err
		}
		return a.runPickup(args[1:])
	case "repo":
		if err := ensureStorage(a.paths); err != nil {
			return err
		}
		return a.runRepo(args[1:])
	case "tui":
		if err := ensureStorage(a.paths); err != nil {
			return err
		}
		return a.runTUI()
	default:
		return &ExitError{Code: ExitInvalidUsage, Err: fmt.Errorf("unknown command: %s", args[0])}
	}
}

func (a *App) printHelp() {
	fmt.Fprint(a.out, `skilldrop manages Git-backed skills and drops them into agent workspaces.

Usage:
  skilldrop list [skills] [--json]
  skilldrop repo add <git-url> [--branch main] [--id repo-id] [--json] [--no-interactive]
  skilldrop drop --skill <name> --agent <id> [--force] [--dry-run] [--json] [--no-interactive]
  skilldrop pickup --skill <name> --agent <id> [--force] [--dry-run] [--json] [--no-interactive]
  skilldrop tui

`)
}

func (a *App) runRepo(args []string) error {
	if len(args) == 0 {
		return &ExitError{Code: ExitInvalidUsage, Err: errors.New("usage: skilldrop repo add <git-url> [--branch main] [--id repo-id]")}
	}
	switch args[0] {
	case "add":
		return a.runRepoAdd(args[1:])
	default:
		return &ExitError{Code: ExitInvalidUsage, Err: fmt.Errorf("unknown repo command: %s", args[0])}
	}
}

func (a *App) runRepoAdd(args []string) error {
	flags, positional, err := parseFlags(args)
	if err != nil {
		return &ExitError{Code: ExitInvalidUsage, Err: err}
	}
	if len(positional) != 1 {
		return &ExitError{Code: ExitInvalidUsage, Err: errors.New("usage: skilldrop repo add <git-url> [--branch main] [--id repo-id]")}
	}
	result, err := RepoAdd(a.paths, RepoAddRequest{
		URL:    positional[0],
		ID:     flags.ID,
		Branch: flags.Branch,
	})
	if err != nil {
		var duplicate *DuplicateSkillError
		if errors.As(err, &duplicate) {
			return writeJSONOrTextError(a.out, flags.JSON, ExitInvalidUsage, "duplicate_skill", duplicate.Error(), "Rename or disable one of the conflicting skills.")
		}
		return err
	}
	if flags.JSON {
		return writeJSON(a.out, result)
	}
	fmt.Fprintf(a.out, "Added repo %s with %d skills\n", result.Repo, len(result.Skills))
	for _, skill := range result.Skills {
		fmt.Fprintf(a.out, "%s\t%s\n", skill.Name, skill.SourcePath)
	}
	return nil
}

func (a *App) runList(args []string) error {
	flags, positional, err := parseFlags(args)
	if err != nil {
		return &ExitError{Code: ExitInvalidUsage, Err: err}
	}
	if len(positional) > 1 || (len(positional) == 1 && positional[0] != "skills") {
		return &ExitError{Code: ExitInvalidUsage, Err: errors.New("usage: skilldrop list [skills] [--json]")}
	}

	catalog, err := loadCatalog(a.paths)
	if err != nil {
		return err
	}
	if flags.JSON {
		return writeJSON(a.out, map[string][]Skill{"skills": catalog.Skills})
	}
	for _, skill := range catalog.Skills {
		fmt.Fprintf(a.out, "%s\t%s\t%s\n", skill.Name, skill.Repo, skill.SourcePath)
	}
	return nil
}

func (a *App) runDrop(args []string) error {
	flags, positional, err := parseFlags(args)
	if err != nil {
		return &ExitError{Code: ExitInvalidUsage, Err: err}
	}
	if len(positional) != 0 {
		return &ExitError{Code: ExitInvalidUsage, Err: errors.New("drop does not accept positional arguments")}
	}
	skill, agent, err := a.resolveSkillAndAgent(flags, "drop")
	if err != nil {
		return err
	}

	result, err := Drop(DropRequest{
		Skill:     skill,
		Agent:     agent,
		CacheDir:  a.paths.cacheDir,
		Workspace: a.wd,
		Force:     flags.Force,
		DryRun:    flags.DryRun,
	})
	if err != nil {
		var conflict *ConflictError
		if errors.As(err, &conflict) {
			if flags.JSON {
				return writeDropConflict(a.out, conflict)
			}
			return &ExitError{Code: ExitOverwriteConflict, Err: fmt.Errorf("refusing to overwrite existing file with different content:\n\n  %s\n\nUse --force to overwrite.", conflict.Path)}
		}
		return err
	}

	if flags.JSON {
		return writeJSON(a.out, result)
	}
	writeDropSummary(a.out, result, flags.DryRun)
	return nil
}

func (a *App) runPickup(args []string) error {
	flags, positional, err := parseFlags(args)
	if err != nil {
		return &ExitError{Code: ExitInvalidUsage, Err: err}
	}
	if len(positional) != 0 {
		return &ExitError{Code: ExitInvalidUsage, Err: errors.New("pickup does not accept positional arguments")}
	}
	skill, agent, err := a.resolveSkillAndAgent(flags, "pickup")
	if err != nil {
		return err
	}

	result, err := Pickup(PickupRequest{
		Skill:     skill,
		Agent:     agent,
		CacheDir:  a.paths.cacheDir,
		Workspace: a.wd,
		Force:     flags.Force,
		DryRun:    flags.DryRun,
	})
	if err != nil {
		var localChange *LocalChangeError
		if errors.As(err, &localChange) {
			if flags.JSON {
				return writePickupLocalChange(a.out, localChange)
			}
			return &ExitError{Code: ExitOverwriteConflict, Err: fmt.Errorf("refusing to remove skill with local changes:\n\n  %s\n\nUse --force to remove anyway.", localChange.Path)}
		}
		return err
	}

	if flags.JSON {
		return writeJSON(a.out, result)
	}
	if flags.DryRun {
		fmt.Fprintf(a.out, "Dry run: pickup %s from %s\n", result.Skill, result.Destination)
	} else {
		fmt.Fprintf(a.out, "Picked up %s from %s\n", result.Skill, result.Destination)
	}
	fmt.Fprintf(a.out, "Files removed: %d\n", result.FilesRemoved)
	return nil
}

func (a *App) resolveSkillAndAgent(flags parsedFlags, command string) (Skill, Agent, error) {
	interactiveDrop := command == "drop" && !flags.NoInteractive && !flags.JSON
	catalog, err := loadCatalog(a.paths)
	if err != nil {
		return Skill{}, Agent{}, err
	}

	var skill Skill
	if flags.Skill == "" {
		if !interactiveDrop {
			return Skill{}, Agent{}, writeDropInputError(a.out, flags.JSON, "missing_skill", fmt.Sprintf("Missing required --skill. Interactive %s skill selection is not implemented yet.", command))
		}
		selected, err := a.selectSkill(catalog.Skills)
		if err != nil {
			return Skill{}, Agent{}, err
		}
		skill = selected
	} else {
		var ok bool
		skill, ok = catalog.Find(flags.Skill)
		if !ok {
			return Skill{}, Agent{}, writeJSONOrTextError(a.out, flags.JSON, ExitSkillNotFound, "skill_not_found", fmt.Sprintf("Skill not found: %s", flags.Skill), "")
		}
	}

	agents, err := loadAgents(a.paths)
	if err != nil {
		return Skill{}, Agent{}, err
	}
	agentID := flags.Agent
	if agentID == "" {
		if len(agents) == 1 {
			agentID = agents[0].ID
		} else if interactiveDrop {
			selected, err := a.selectAgent(agents)
			if err != nil {
				return Skill{}, Agent{}, err
			}
			return skill, selected, nil
		} else {
			return Skill{}, Agent{}, writeDropInputError(a.out, flags.JSON, "missing_agent", fmt.Sprintf("Missing required --agent. Interactive %s agent selection is not implemented yet.", command))
		}
	}
	agent, ok := findAgent(agents, agentID)
	if !ok {
		return Skill{}, Agent{}, writeJSONOrTextError(a.out, flags.JSON, ExitAgentNotFound, "agent_not_found", fmt.Sprintf("Agent not found: %s", agentID), "")
	}
	return skill, agent, nil
}

func (a *App) selectSkill(skills []Skill) (Skill, error) {
	if len(skills) == 0 {
		return Skill{}, &ExitError{Code: ExitSkillNotFound, Err: errors.New("no registered skills found")}
	}
	if len(skills) == 1 {
		return skills[0], nil
	}
	items := make([]pickerItem, 0, len(skills))
	for _, skill := range skills {
		items = append(items, pickerItem{
			Label:  skill.Name,
			Detail: skill.Repo + "  " + skill.SourcePath,
		})
	}
	selected, err := runInlinePicker(a.out, "Select a skill", items)
	if err != nil {
		return Skill{}, err
	}
	return skills[selected], nil
}

func (a *App) selectAgent(agents []Agent) (Agent, error) {
	if len(agents) == 0 {
		return Agent{}, &ExitError{Code: ExitAgentNotFound, Err: errors.New("no configured agents found")}
	}
	if len(agents) == 1 {
		return agents[0], nil
	}
	items := make([]pickerItem, 0, len(agents))
	for _, agent := range agents {
		items = append(items, pickerItem{
			Label:  agent.ID,
			Detail: agent.Path,
		})
	}
	selected, err := runInlinePicker(a.out, "Select an agent", items)
	if err != nil {
		return Agent{}, err
	}
	return agents[selected], nil
}

func writeDropInputError(out io.Writer, jsonOutput bool, code string, message string) error {
	return writeJSONOrTextError(out, jsonOutput, ExitInvalidUsage, code, message, "")
}

func writeJSONOrTextError(out io.Writer, jsonOutput bool, exitCode int, code string, message string, hint string) error {
	if jsonOutput {
		payload := map[string]string{
			"status":  "error",
			"error":   code,
			"message": message,
		}
		if hint != "" {
			payload["hint"] = hint
		}
		_ = writeJSON(out, payload)
	}
	return &ExitError{Code: exitCode, Err: errors.New(message)}
}

func writeDropConflict(out io.Writer, conflict *ConflictError) error {
	_ = writeJSON(out, map[string]string{
		"status":  "error",
		"error":   "would_overwrite",
		"message": "Refusing to overwrite existing file with different content.",
		"path":    filepath.ToSlash(conflict.Path),
		"hint":    "Use --force to overwrite.",
	})
	return &ExitError{Code: ExitOverwriteConflict, Err: conflict}
}

func writeDropSummary(out io.Writer, result DropResult, dryRun bool) {
	if dryRun {
		fmt.Fprintf(out, "Dry run: %s -> %s\n", result.Skill, result.Destination)
	} else {
		fmt.Fprintf(out, "Dropped %s into %s\n", result.Skill, result.Destination)
	}
	for _, file := range result.Files {
		fmt.Fprintf(out, "%s %s\n", renderDropTag(file.Action), file.Path)
	}
}

func renderDropTag(action string) string {
	tag := fmt.Sprintf("[%-7s]", action)
	if tuiColorEnabled() {
		return tuiAccentStyle().Render(tag)
	}
	return tag
}

func writePickupLocalChange(out io.Writer, localChange *LocalChangeError) error {
	_ = writeJSON(out, map[string]string{
		"status":  "error",
		"error":   "local_changes",
		"message": "Refusing to remove skill with local changes.",
		"path":    filepath.ToSlash(localChange.Path),
		"hint":    "Use --force to remove anyway.",
	})
	return &ExitError{Code: ExitOverwriteConflict, Err: localChange}
}

func writeJSON(out io.Writer, v any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

type parsedFlags struct {
	Skill         string
	Agent         string
	Force         bool
	DryRun        bool
	JSON          bool
	NoInteractive bool
	ID            string
	Branch        string
}

func parseFlags(args []string) (parsedFlags, []string, error) {
	var flags parsedFlags
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--skill":
			i++
			if i >= len(args) || strings.HasPrefix(args[i], "--") {
				return flags, nil, errors.New("--skill requires a value")
			}
			flags.Skill = args[i]
		case "--agent":
			i++
			if i >= len(args) || strings.HasPrefix(args[i], "--") {
				return flags, nil, errors.New("--agent requires a value")
			}
			flags.Agent = args[i]
		case "--force":
			flags.Force = true
		case "--dry-run":
			flags.DryRun = true
		case "--json":
			flags.JSON = true
		case "--no-interactive":
			flags.NoInteractive = true
		case "--interactive":
		case "--id":
			i++
			if i >= len(args) || strings.HasPrefix(args[i], "--") {
				return flags, nil, errors.New("--id requires a value")
			}
			flags.ID = args[i]
		case "--branch":
			i++
			if i >= len(args) || strings.HasPrefix(args[i], "--") {
				return flags, nil, errors.New("--branch requires a value")
			}
			flags.Branch = args[i]
		default:
			if strings.HasPrefix(arg, "--") {
				return flags, nil, fmt.Errorf("unknown flag: %s", arg)
			}
			positional = append(positional, arg)
		}
	}
	return flags, positional, nil
}
