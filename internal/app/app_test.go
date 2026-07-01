package skilldrop

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestListSkillsJSON(t *testing.T) {
	env := newTestEnv(t)
	env.writeConfig("agents.yaml", "version: 1\nagents:\n  - id: codex\n    path: .codex/skills\n")
	env.writeConfig("repos/personal.yaml", "version: 1\nid: personal\nname: Personal Skills\ngit:\n  url: example\n  branch: main\nskills:\n  - name: go-cli-builder\n    source_path: skills/go-cli-builder\n    enabled: true\n")

	var out bytes.Buffer
	err := Run([]string{"list", "skills", "--json"}, &out, &bytes.Buffer{}, "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var payload struct {
		Skills []Skill `json:"skills"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if len(payload.Skills) != 1 || payload.Skills[0].Name != "go-cli-builder" || payload.Skills[0].Repo != "personal" {
		t.Fatalf("unexpected skills: %+v", payload.Skills)
	}
}

func TestRunBootstrapsStorageAndListCanBeEmpty(t *testing.T) {
	env := newTestEnv(t)
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"list", "skills", "--json"}, &out, &bytes.Buffer{}, "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for _, rel := range []string{
		"skilldrop",
		"skilldrop/repos",
	} {
		if info, err := os.Stat(filepath.Join(env.configHome, filepath.FromSlash(rel))); err != nil || !info.IsDir() {
			t.Fatalf("expected config directory %s, info=%v err=%v", rel, info, err)
		}
	}
	for _, rel := range []string{
		"skilldrop",
		"skilldrop/catalogs",
	} {
		if info, err := os.Stat(filepath.Join(env.cacheHome, filepath.FromSlash(rel))); err != nil || !info.IsDir() {
			t.Fatalf("expected cache directory %s, info=%v err=%v", rel, info, err)
		}
	}
	if got := env.readConfig("agents.yaml"); !strings.Contains(got, "agents: []") {
		t.Fatalf("expected default empty agents config, got:\n%s", got)
	}
	if !strings.Contains(out.String(), `"skills": []`) {
		t.Fatalf("expected empty skills json, got %s", out.String())
	}
}

func TestDropDryRunDoesNotWriteFiles(t *testing.T) {
	env := newTestEnv(t)
	env.writeStandardConfig()
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/SKILL.md", "---\nname: go-cli-builder\n---\n")
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"drop", "--skill", "go-cli-builder", "--agent", "codex", "--dry-run", "--json", "--no-interactive"}, &out, &bytes.Buffer{}, "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.workspace, ".codex/skills/go-cli-builder/SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote destination file, stat err=%v", err)
	}
	if !strings.Contains(out.String(), `"status": "dry_run"`) {
		t.Fatalf("expected dry_run json, got %s", out.String())
	}
}

func TestDropWritesFilesAndLeavesIdenticalFilesUnchanged(t *testing.T) {
	env := newTestEnv(t)
	env.writeStandardConfig()
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/SKILL.md", "same\n")
	env.writeWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md", "same\n")
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"drop", "--skill", "go-cli-builder", "--agent", "codex", "--json", "--no-interactive"}, &out, &bytes.Buffer{}, "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), `"files_unchanged": 1`) {
		t.Fatalf("expected unchanged file count, got %s", out.String())
	}
}

func TestDropConflictIsAtomic(t *testing.T) {
	env := newTestEnv(t)
	env.writeStandardConfig()
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/SKILL.md", "new\n")
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/examples/example.txt", "example\n")
	env.writeWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md", "old\n")
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"drop", "--skill", "go-cli-builder", "--agent", "codex", "--json", "--no-interactive"}, &out, &bytes.Buffer{}, "test")
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if ExitCode(err) != ExitOverwriteConflict {
		t.Fatalf("expected exit code %d, got %d: %v", ExitOverwriteConflict, ExitCode(err), err)
	}
	if got := env.readWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md"); got != "old\n" {
		t.Fatalf("conflicting file changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(env.workspace, ".codex/skills/go-cli-builder/examples/example.txt")); !os.IsNotExist(err) {
		t.Fatalf("atomic check wrote non-conflicting file, stat err=%v", err)
	}
}

func TestDropForceOverwritesConflict(t *testing.T) {
	env := newTestEnv(t)
	env.writeStandardConfig()
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/SKILL.md", "new\n")
	env.writeWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md", "old\n")
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"drop", "--skill", "go-cli-builder", "--agent", "codex", "--force", "--json", "--no-interactive"}, &out, &bytes.Buffer{}, "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := env.readWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md"); got != "new\n" {
		t.Fatalf("file was not overwritten: %q", got)
	}
	if !strings.Contains(out.String(), `"files_overwritten": 1`) {
		t.Fatalf("expected overwritten count, got %s", out.String())
	}
}

func TestDropHumanOutputListsRelativeFileActions(t *testing.T) {
	env := newTestEnv(t)
	t.Setenv("NO_COLOR", "1")
	env.writeStandardConfig()
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/SKILL.md", "new\n")
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/examples/example.txt", "example\n")
	env.writeWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md", "old\n")
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"drop", "--skill", "go-cli-builder", "--agent", "codex", "--force"}, &out, &bytes.Buffer{}, "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"[updated] SKILL.md",
		"[add    ] examples/example.txt",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, env.workspace+"/.codex/skills/go-cli-builder/SKILL.md") {
		t.Fatalf("file list should use relative paths, got:\n%s", got)
	}
}

func TestDropMissingSkillStrictModeReturnsJSONError(t *testing.T) {
	env := newTestEnv(t)
	env.writeStandardConfig()
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"drop", "--agent", "codex", "--json", "--no-interactive"}, &out, &bytes.Buffer{}, "test")
	if err == nil {
		t.Fatal("expected missing skill error")
	}
	if ExitCode(err) != ExitInvalidUsage {
		t.Fatalf("expected invalid usage, got %d", ExitCode(err))
	}
	if !strings.Contains(out.String(), `"error": "missing_skill"`) {
		t.Fatalf("expected json missing skill error, got %s", out.String())
	}
}

func TestDropAutoSelectsSingleMissingSkillAndAgent(t *testing.T) {
	env := newTestEnv(t)
	env.writeStandardConfig()
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/SKILL.md", "same\n")
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"drop"}, &out, &bytes.Buffer{}, "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := env.readWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md"); got != "same\n" {
		t.Fatalf("unexpected dropped file: %q", got)
	}
}

func TestPickerModelSelectsAndCancels(t *testing.T) {
	model := newPickerModel("Select", []pickerItem{
		{Label: "one"},
		{Label: "two"},
	})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(pickerModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(pickerModel)

	if model.selected != 1 || model.canceled {
		t.Fatalf("expected selected index 1, got selected=%d canceled=%v", model.selected, model.canceled)
	}

	model = newPickerModel("Select", []pickerItem{{Label: "one"}})
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(pickerModel)
	if !model.canceled {
		t.Fatal("expected picker cancel")
	}
}

func TestPickerViewMarksSelectedRow(t *testing.T) {
	model := newPickerModel("Select", []pickerItem{
		{Label: "one"},
		{Label: "two"},
	})
	if !strings.Contains(model.View(), "> one") {
		t.Fatalf("expected selected picker row marker:\n%s", model.View())
	}
}

func TestPickupRemovesInstalledSkill(t *testing.T) {
	env := newTestEnv(t)
	env.writeStandardConfig()
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/SKILL.md", "same\n")
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/examples/example.txt", "example\n")
	env.writeWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md", "same\n")
	env.writeWorkspaceFile(".codex/skills/go-cli-builder/examples/example.txt", "example\n")
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"pickup", "--skill", "go-cli-builder", "--agent", "codex", "--json", "--no-interactive"}, &out, &bytes.Buffer{}, "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.workspace, ".codex/skills/go-cli-builder")); !os.IsNotExist(err) {
		t.Fatalf("pickup did not remove skill directory, stat err=%v", err)
	}
	if !strings.Contains(out.String(), `"status": "picked_up"`) || !strings.Contains(out.String(), `"files_removed": 2`) {
		t.Fatalf("unexpected pickup json: %s", out.String())
	}
}

func TestPickupDryRunDoesNotRemoveFiles(t *testing.T) {
	env := newTestEnv(t)
	env.writeStandardConfig()
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/SKILL.md", "same\n")
	env.writeWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md", "same\n")
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"pickup", "--skill", "go-cli-builder", "--agent", "codex", "--dry-run", "--json", "--no-interactive"}, &out, &bytes.Buffer{}, "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := env.readWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md"); got != "same\n" {
		t.Fatalf("dry run removed or changed file: %q", got)
	}
	if !strings.Contains(out.String(), `"status": "dry_run"`) {
		t.Fatalf("expected dry_run json, got %s", out.String())
	}
}

func TestPickupRefusesLocalChanges(t *testing.T) {
	env := newTestEnv(t)
	env.writeStandardConfig()
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/SKILL.md", "source\n")
	env.writeWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md", "modified\n")
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"pickup", "--skill", "go-cli-builder", "--agent", "codex", "--json", "--no-interactive"}, &out, &bytes.Buffer{}, "test")
	if err == nil {
		t.Fatal("expected local changes error")
	}
	if ExitCode(err) != ExitOverwriteConflict {
		t.Fatalf("expected overwrite conflict exit code, got %d: %v", ExitCode(err), err)
	}
	if got := env.readWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md"); got != "modified\n" {
		t.Fatalf("local change was removed: %q", got)
	}
	if !strings.Contains(out.String(), `"error": "local_changes"`) {
		t.Fatalf("expected local_changes json, got %s", out.String())
	}
}

func TestPickupForceRemovesLocalChanges(t *testing.T) {
	env := newTestEnv(t)
	env.writeStandardConfig()
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/SKILL.md", "source\n")
	env.writeWorkspaceFile(".codex/skills/go-cli-builder/SKILL.md", "modified\n")
	env.chdirWorkspace()

	err := Run([]string{"pickup", "--skill", "go-cli-builder", "--agent", "codex", "--force", "--no-interactive"}, &bytes.Buffer{}, &bytes.Buffer{}, "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.workspace, ".codex/skills/go-cli-builder")); !os.IsNotExist(err) {
		t.Fatalf("force pickup did not remove skill directory, stat err=%v", err)
	}
}

func TestPickupMissingDestinationFails(t *testing.T) {
	env := newTestEnv(t)
	env.writeStandardConfig()
	env.writeCacheFile("catalogs/personal/skills/go-cli-builder/SKILL.md", "same\n")
	env.chdirWorkspace()

	err := Run([]string{"pickup", "--skill", "go-cli-builder", "--agent", "codex", "--json", "--no-interactive"}, &bytes.Buffer{}, &bytes.Buffer{}, "test")
	if err == nil {
		t.Fatal("expected missing destination error")
	}
	if ExitCode(err) != ExitMissingConfiguration {
		t.Fatalf("expected missing configuration exit code, got %d: %v", ExitCode(err), err)
	}
}

func TestRepoAddClonesScansAndWritesConfig(t *testing.T) {
	env := newTestEnv(t)
	source := env.createGitRepo("source")
	env.writeRepoFile(source, "skills/go-cli-builder/SKILL.md", "---\nname: go-cli-builder\n---\nBuild Go CLIs.\n")
	env.git(source, "add", ".")
	env.git(source, "commit", "-m", "add skill")
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"repo", "add", source, "--id", "personal", "--branch", "main", "--json", "--no-interactive"}, &out, &bytes.Buffer{}, "test")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), `"repo": "personal"`) || !strings.Contains(out.String(), `"name": "go-cli-builder"`) {
		t.Fatalf("unexpected repo add output: %s", out.String())
	}
	config := env.readConfig("repos/personal.yaml")
	if !strings.Contains(config, "name: go-cli-builder") || !strings.Contains(config, "source_path: skills/go-cli-builder") {
		t.Fatalf("repo config missing discovered skill:\n%s", config)
	}
	if _, err := os.Stat(filepath.Join(env.cacheHome, "skilldrop/catalogs/personal/.git")); err != nil {
		t.Fatalf("expected cached clone: %v", err)
	}
}

func TestRepoAddUpdatesExistingClone(t *testing.T) {
	env := newTestEnv(t)
	source := env.createGitRepo("source")
	env.writeRepoFile(source, "skills/one/SKILL.md", "---\nname: one\n---\n")
	env.git(source, "add", ".")
	env.git(source, "commit", "-m", "add first skill")
	env.chdirWorkspace()

	if err := Run([]string{"repo", "add", source, "--id", "personal", "--branch", "main", "--no-interactive"}, &bytes.Buffer{}, &bytes.Buffer{}, "test"); err != nil {
		t.Fatalf("first repo add returned error: %v", err)
	}

	env.writeRepoFile(source, "skills/two/SKILL.md", "---\nname: two\n---\n")
	env.git(source, "add", ".")
	env.git(source, "commit", "-m", "add second skill")

	if err := Run([]string{"repo", "add", source, "--id", "personal", "--branch", "main", "--no-interactive"}, &bytes.Buffer{}, &bytes.Buffer{}, "test"); err != nil {
		t.Fatalf("second repo add returned error: %v", err)
	}
	config := env.readConfig("repos/personal.yaml")
	if !strings.Contains(config, "name: one") || !strings.Contains(config, "name: two") {
		t.Fatalf("updated repo config missing skills:\n%s", config)
	}
}

func TestRepoAddRejectsDuplicateSkillNames(t *testing.T) {
	env := newTestEnv(t)
	env.writeConfig("repos/existing.yaml", "version: 1\nid: existing\nname: Existing\ngit:\n  url: example\n  branch: main\nskills:\n  - name: duplicated\n    source_path: skills/duplicated\n    enabled: true\n")
	source := env.createGitRepo("source")
	env.writeRepoFile(source, "skills/duplicated/SKILL.md", "---\nname: duplicated\n---\n")
	env.git(source, "add", ".")
	env.git(source, "commit", "-m", "add duplicate skill")
	env.chdirWorkspace()

	var out bytes.Buffer
	err := Run([]string{"repo", "add", source, "--id", "newrepo", "--branch", "main", "--json", "--no-interactive"}, &out, &bytes.Buffer{}, "test")
	if err == nil {
		t.Fatal("expected duplicate skill error")
	}
	if ExitCode(err) != ExitInvalidUsage {
		t.Fatalf("expected invalid usage, got %d", ExitCode(err))
	}
	if !strings.Contains(out.String(), `"error": "duplicate_skill"`) {
		t.Fatalf("expected duplicate json error, got %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(env.configHome, "skilldrop/repos/newrepo.yaml")); !os.IsNotExist(err) {
		t.Fatalf("duplicate repo wrote config, stat err=%v", err)
	}
}

func TestTUIModelCatalogIsReadOnly(t *testing.T) {
	env := newTestEnv(t)
	env.writeConfig("repos/personal.yaml", "version: 1\nid: personal\nname: Personal Skills\ngit:\n  url: example\n  branch: main\nskills:\n  - name: first\n    source_path: skills/first\n    enabled: true\n  - name: second\n    source_path: skills/second\n    enabled: true\n")

	model := newTUIModel(paths{
		configDir: filepath.Join(env.configHome, "skilldrop"),
		cacheDir:  filepath.Join(env.cacheHome, "skilldrop"),
	}, env.workspace)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(tuiModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)

	if model.err != nil {
		t.Fatalf("unexpected TUI error: %v", model.err)
	}
	if model.skillIdx != 1 {
		t.Fatalf("expected selected second skill, got %d", model.skillIdx)
	}
	if _, err := os.Stat(filepath.Join(env.workspace, ".codex/skills/second")); !os.IsNotExist(err) {
		t.Fatalf("catalog enter should not drop skills, stat err=%v", err)
	}
}

func TestTUIModelAddsAndRemovesAgents(t *testing.T) {
	env := newTestEnv(t)
	p := paths{
		configDir: filepath.Join(env.configHome, "skilldrop"),
		cacheDir:  filepath.Join(env.cacheHome, "skilldrop"),
	}
	if err := ensureStorage(p); err != nil {
		t.Fatalf("ensureStorage returned error: %v", err)
	}

	model := newTUIModel(p, env.workspace)
	model.tab = tuiTabAgents
	model.startAddAgent()
	model.agentInput.SetValue(".codex/skills")
	model.submitAgent()

	if model.err != nil {
		t.Fatalf("unexpected add agent error: %v", model.err)
	}
	if len(model.agents) != 1 || model.agents[0].Path != ".codex/skills" {
		t.Fatalf("unexpected agents after add: %+v", model.agents)
	}
	if got := env.readConfig("agents.yaml"); !strings.Contains(got, ".codex/skills") {
		t.Fatalf("agents.yaml missing added path:\n%s", got)
	}

	model.removeSelectedAgent()
	if model.err != nil {
		t.Fatalf("unexpected remove agent error: %v", model.err)
	}
	if len(model.agents) != 0 {
		t.Fatalf("expected no agents after remove: %+v", model.agents)
	}
	if got := env.readConfig("agents.yaml"); strings.Contains(got, ".codex/skills") {
		t.Fatalf("agents.yaml still has removed path:\n%s", got)
	}
}

func TestTUIModelReviewsRepoSkillsBeforeRegistering(t *testing.T) {
	env := newTestEnv(t)
	source := env.createGitRepo("source")
	env.writeRepoFile(source, "skills/go-cli-builder/SKILL.md", "---\nname: go-cli-builder\n---\nBuild Go CLIs.\n")
	env.writeRepoFile(source, "skills/repo-auditor/SKILL.md", "---\nname: repo-auditor\n---\nAudit repos.\n")
	env.git(source, "add", ".")
	env.git(source, "commit", "-m", "add skills")
	p := paths{
		configDir: filepath.Join(env.configHome, "skilldrop"),
		cacheDir:  filepath.Join(env.cacheHome, "skilldrop"),
	}
	if err := ensureStorage(p); err != nil {
		t.Fatalf("ensureStorage returned error: %v", err)
	}

	model := newTUIModel(p, env.workspace)
	model.tab = tuiTabRepos
	model.startAddRepo()
	model.repoInputs[0].SetValue(source)
	model.repoInputs[1].SetValue("personal")
	model.repoInputs[2].SetValue("main")
	model.submitRepo()

	if model.err != nil {
		t.Fatalf("unexpected repo discovery error: %v", model.err)
	}
	if model.mode != tuiModeReviewRepo {
		t.Fatalf("expected review mode, got %v", model.mode)
	}
	if len(model.pendingRepo.Skills) != 2 {
		t.Fatalf("expected two discovered skills, got %+v", model.pendingRepo.Skills)
	}
	model.pendingRepo.Skills[1].Enabled = false
	model.registerPendingRepo()

	if model.err != nil {
		t.Fatalf("unexpected repo register error: %v", model.err)
	}
	if len(model.repos) != 1 || model.repos[0].ID != "personal" {
		t.Fatalf("unexpected repos after register: %+v", model.repos)
	}
	if len(model.skills) != 1 || model.skills[0].Name != "go-cli-builder" {
		t.Fatalf("catalog should include only enabled skills: %+v", model.skills)
	}
	config := env.readConfig("repos/personal.yaml")
	if !strings.Contains(config, "name: repo-auditor") || !strings.Contains(config, "enabled: false") {
		t.Fatalf("repo config should preserve disabled discovered skill:\n%s", config)
	}
	if !strings.Contains(model.status, "Registered repo personal with 1 enabled skills.") {
		t.Fatalf("unexpected status: %q", model.status)
	}
}

func TestTUIModelCanDisableCatalogSkill(t *testing.T) {
	env := newTestEnv(t)
	env.writeStandardConfig()
	p := paths{
		configDir: filepath.Join(env.configHome, "skilldrop"),
		cacheDir:  filepath.Join(env.cacheHome, "skilldrop"),
	}

	model := newTUIModel(p, env.workspace)
	model.disableSelectedCatalogSkill()

	if model.err != nil {
		t.Fatalf("unexpected disable error: %v", model.err)
	}
	if len(model.skills) != 0 {
		t.Fatalf("catalog should be empty after disabling skill: %+v", model.skills)
	}
	if got := env.readConfig("repos/personal.yaml"); !strings.Contains(got, "enabled: false") {
		t.Fatalf("repo config did not disable skill:\n%s", got)
	}
}

func TestTUIModelRepoDetailTogglesSkillEnablement(t *testing.T) {
	env := newTestEnv(t)
	env.writeConfig("repos/personal.yaml", "version: 1\nid: personal\nname: Personal Skills\ngit:\n  url: example\n  branch: main\nskills:\n  - name: go-cli-builder\n    source_path: skills/go-cli-builder\n    enabled: false\n")
	p := paths{
		configDir: filepath.Join(env.configHome, "skilldrop"),
		cacheDir:  filepath.Join(env.cacheHome, "skilldrop"),
	}

	model := newTUIModel(p, env.workspace)
	model.tab = tuiTabRepos
	model.startRepoDetail()
	model.toggleCurrentRepoSkill()

	if model.err != nil {
		t.Fatalf("unexpected toggle error: %v", model.err)
	}
	if len(model.skills) != 1 || model.skills[0].Name != "go-cli-builder" {
		t.Fatalf("catalog did not include enabled skill: %+v", model.skills)
	}
	if got := env.readConfig("repos/personal.yaml"); !strings.Contains(got, "enabled: true") {
		t.Fatalf("repo config did not enable skill:\n%s", got)
	}
}

func TestTUIModelRepoDetailSyncAddsNewSkillsDisabled(t *testing.T) {
	env := newTestEnv(t)
	source := env.createGitRepo("source")
	env.writeRepoFile(source, "skills/one/SKILL.md", "---\nname: one\n---\n")
	env.git(source, "add", ".")
	env.git(source, "commit", "-m", "add first skill")
	p := paths{
		configDir: filepath.Join(env.configHome, "skilldrop"),
		cacheDir:  filepath.Join(env.cacheHome, "skilldrop"),
	}
	if err := ensureStorage(p); err != nil {
		t.Fatalf("ensureStorage returned error: %v", err)
	}
	if _, err := RepoAdd(p, RepoAddRequest{URL: source, ID: "personal", Branch: "main"}); err != nil {
		t.Fatalf("RepoAdd returned error: %v", err)
	}

	env.writeRepoFile(source, "skills/two/SKILL.md", "---\nname: two\n---\n")
	env.git(source, "add", ".")
	env.git(source, "commit", "-m", "add second skill")

	model := newTUIModel(p, env.workspace)
	model.tab = tuiTabRepos
	model.startRepoDetail()
	model.syncCurrentRepo()

	if model.err != nil {
		t.Fatalf("unexpected sync error: %v", model.err)
	}
	if len(model.repos[0].Skills) != 2 {
		t.Fatalf("expected two discovered skills after sync: %+v", model.repos[0].Skills)
	}
	if len(model.skills) != 1 || model.skills[0].Name != "one" {
		t.Fatalf("new skill should stay disabled until selected: %+v", model.skills)
	}
	config := env.readConfig("repos/personal.yaml")
	if !strings.Contains(config, "name: two") || !strings.Contains(config, "enabled: false") {
		t.Fatalf("synced config should include new disabled skill:\n%s", config)
	}
}

func TestTUIModelFreshStorageHasNoSetupTabAndShowsASCII(t *testing.T) {
	env := newTestEnv(t)
	p := paths{
		configDir: filepath.Join(env.configHome, "skilldrop"),
		cacheDir:  filepath.Join(env.cacheHome, "skilldrop"),
	}
	if err := ensureStorage(p); err != nil {
		t.Fatalf("ensureStorage returned error: %v", err)
	}

	model := newTUIModel(p, env.workspace)
	if model.err != nil {
		t.Fatalf("unexpected TUI error: %v", model.err)
	}
	view := model.View()
	if !strings.HasPrefix(view, "\n") {
		t.Fatalf("view should start with a blank line before ASCII art:\n%s", view)
	}
	for _, want := range []string{"___| |", "[Catalog]", " Repos ", " Agents ", "No skills registered yet."} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Use left/right to move tabs") {
		t.Fatalf("view should not include redundant default status text:\n%s", view)
	}
	if strings.Contains(view, "Setup") {
		t.Fatalf("view should not include Setup tab:\n%s", view)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(tuiModel)
	if model.tab != tuiTabRepos {
		t.Fatalf("expected repos tab after right key, got %v", model.tab)
	}
	if !strings.Contains(model.View(), "Registered Repositories") {
		t.Fatalf("expected repos tab content:\n%s", model.View())
	}
}

func TestTUIColorPolicyUsesOrangeAndHonorsNoColor(t *testing.T) {
	originalNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadNoColor {
			_ = os.Setenv("NO_COLOR", originalNoColor)
		} else {
			_ = os.Unsetenv("NO_COLOR")
		}
	})

	color, ok := tuiAccentStyle().GetForeground().(lipgloss.Color)
	if !ok || color != lipgloss.Color(tuiAccentColor) {
		t.Fatalf("expected orange accent color %q, got %#v", tuiAccentColor, tuiAccentStyle().GetForeground())
	}

	if err := os.Setenv("NO_COLOR", "1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tuiAccentStyle().GetForeground().(lipgloss.NoColor); !ok {
		t.Fatalf("expected no foreground color with NO_COLOR, got %#v", tuiAccentStyle().GetForeground())
	}
	if _, ok := tuiErrorStyle().GetForeground().(lipgloss.NoColor); !ok {
		t.Fatalf("expected no error foreground color with NO_COLOR, got %#v", tuiErrorStyle().GetForeground())
	}
}

type testEnv struct {
	t          *testing.T
	root       string
	configHome string
	cacheHome  string
	workspace  string
	oldWd      string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	root := t.TempDir()
	env := &testEnv{
		t:          t,
		root:       root,
		configHome: filepath.Join(root, "config"),
		cacheHome:  filepath.Join(root, "cache"),
		workspace:  filepath.Join(root, "workspace"),
	}
	t.Setenv("XDG_CONFIG_HOME", env.configHome)
	t.Setenv("XDG_CACHE_HOME", env.cacheHome)
	if err := os.MkdirAll(env.workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	env.oldWd = wd
	t.Cleanup(func() {
		_ = os.Chdir(env.oldWd)
	})
	return env
}

func (e *testEnv) writeStandardConfig() {
	e.writeConfig("agents.yaml", "version: 1\nagents:\n  - id: codex\n    path: .codex/skills\n")
	e.writeConfig("repos/personal.yaml", "version: 1\nid: personal\nname: Personal Skills\ngit:\n  url: example\n  branch: main\nskills:\n  - name: go-cli-builder\n    source_path: skills/go-cli-builder\n    enabled: true\n")
}

func (e *testEnv) writeConfig(rel string, data string) {
	e.t.Helper()
	path := filepath.Join(e.configHome, "skilldrop", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *testEnv) readConfig(rel string) string {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(e.configHome, "skilldrop", filepath.FromSlash(rel)))
	if err != nil {
		e.t.Fatal(err)
	}
	return string(data)
}

func (e *testEnv) writeCacheFile(rel string, data string) {
	e.t.Helper()
	path := filepath.Join(e.cacheHome, "skilldrop", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *testEnv) writeWorkspaceFile(rel string, data string) {
	e.t.Helper()
	path := filepath.Join(e.workspace, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *testEnv) readWorkspaceFile(rel string) string {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(e.workspace, filepath.FromSlash(rel)))
	if err != nil {
		e.t.Fatal(err)
	}
	return string(data)
}

func (e *testEnv) chdirWorkspace() {
	e.t.Helper()
	if err := os.Chdir(e.workspace); err != nil {
		e.t.Fatal(err)
	}
}

func (e *testEnv) createGitRepo(name string) string {
	e.t.Helper()
	path := filepath.Join(e.root, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		e.t.Fatal(err)
	}
	e.git(path, "init", "-b", "main")
	e.git(path, "config", "user.email", "test@example.com")
	e.git(path, "config", "user.name", "Test User")
	return path
}

func (e *testEnv) writeRepoFile(repo string, rel string, data string) {
	e.t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *testEnv) git(dir string, args ...string) {
	e.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}
