package skilldrop

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tuiTab int

const (
	tuiTabCatalog tuiTab = iota
	tuiTabRepos
	tuiTabAgents
)

type tuiMode int

const (
	tuiModeNormal tuiMode = iota
	tuiModeAddAgent
	tuiModeAddRepo
	tuiModeReviewRepo
	tuiModeRepoDetail
)

type tuiModel struct {
	paths       paths
	workspace   string
	skills      []Skill
	agents      []Agent
	repos       []repoConfig
	tab         tuiTab
	mode        tuiMode
	skillIdx    int
	agentIdx    int
	repoIdx     int
	repoField   int
	reviewIdx   int
	detailIdx   int
	width       int
	height      int
	status      string
	err         error
	quitting    bool
	agentInput  textinput.Model
	repoInputs  []textinput.Model
	pendingRepo repoConfig
}

const tuiAccentColor = "208"

func tuiColorEnabled() bool {
	_, noColor := os.LookupEnv("NO_COLOR")
	return !noColor
}

func tuiAccentStyle() lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true)
	if tuiColorEnabled() {
		style = style.Foreground(lipgloss.Color(tuiAccentColor))
	}
	return style
}

func tuiMutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Faint(true)
}

func tuiErrorStyle() lipgloss.Style {
	style := lipgloss.NewStyle()
	if tuiColorEnabled() {
		style = style.Foreground(lipgloss.Color("9"))
	}
	return style
}

const skilldropASCII = `     _    _ _ _     _                  
 ___| | _(_) | | __| |_ __ ___  _ __  
/ __| |/ / | | |/ _` + "`" + ` | '__/ _ \| '_ \ 
\__ \   <| | | | (_| | | | (_) | |_) |
|___/_|\_\_|_|_|\__,_|_|  \___/| .__/ 
                               |_|    `

func (a *App) runTUI() error {
	model := newTUIModel(a.paths, a.wd)
	program := tea.NewProgram(model, tea.WithOutput(a.out), tea.WithAltScreen())
	_, err := program.Run()
	if err != nil {
		return &ExitError{Code: ExitGeneral, Err: err}
	}
	return nil
}

func newTUIModel(p paths, workspace string) tuiModel {
	model := tuiModel{
		paths:      p,
		workspace:  workspace,
		agentInput: newInput("Agent path, e.g. .codex/skills"),
		repoInputs: []textinput.Model{
			newInput("Git URL"),
			newInput("Repo ID, optional"),
			newInput("Branch, default main"),
		},
	}
	model.refresh()
	return model
}

func newInput(placeholder string) textinput.Model {
	input := textinput.New()
	input.Placeholder = placeholder
	input.CharLimit = 512
	input.Width = 72
	return input
}

func (m *tuiModel) refresh() {
	catalog, err := loadCatalogAllowMissing(m.paths)
	if err != nil {
		m.err = err
		return
	}
	agents, err := loadAgentsAllowMissing(m.paths)
	if err != nil {
		m.err = err
		return
	}
	repos, err := loadRepoConfigsAllowMissing(m.paths)
	if err != nil {
		m.err = err
		return
	}
	m.skills = catalog.Skills
	m.agents = agents
	m.repos = repos
	m.skillIdx = clampIndex(m.skillIdx, len(m.skills))
	m.agentIdx = clampIndex(m.agentIdx, len(m.agents))
	m.repoIdx = clampIndex(m.repoIdx, len(m.repos))
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	default:
		return m, nil
	}
}

func (m tuiModel) updateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == tuiModeAddAgent {
		return m.updateAddAgent(key)
	}
	if m.mode == tuiModeAddRepo {
		return m.updateAddRepo(key)
	}
	if m.mode == tuiModeReviewRepo {
		return m.updateReviewRepo(key)
	}
	if m.mode == tuiModeRepoDetail {
		return m.updateRepoDetail(key)
	}

	switch key.String() {
	case "q", "ctrl+c", "esc":
		m.quitting = true
		return m, tea.Quit
	case "left", "h":
		m.moveTab(-1)
	case "right", "l", "tab":
		m.moveTab(1)
	case "up", "k":
		m.moveSelection(-1)
	case "down", "j":
		m.moveSelection(1)
	case "enter":
		if m.tab == tuiTabRepos && len(m.repos) > 0 {
			m.startRepoDetail()
		}
	case " ":
		if m.tab == tuiTabRepos && len(m.repos) > 0 {
			m.startRepoDetail()
		}
	case "a":
		switch m.tab {
		case tuiTabAgents:
			m.startAddAgent()
		case tuiTabRepos:
			m.startAddRepo()
		}
	case "d", "delete", "backspace":
		if m.tab == tuiTabCatalog {
			m.disableSelectedCatalogSkill()
		} else if m.tab == tuiTabAgents {
			m.removeSelectedAgent()
		}
	}
	return m, nil
}

func (m tuiModel) updateAddAgent(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "ctrl+c":
		m.mode = tuiModeNormal
		m.agentInput.Blur()
		m.status = "Agent add canceled."
		return m, nil
	case "enter":
		m.submitAgent()
		return m, nil
	}
	var cmd tea.Cmd
	m.agentInput, cmd = m.agentInput.Update(key)
	return m, cmd
}

func (m tuiModel) updateAddRepo(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "ctrl+c":
		m.mode = tuiModeNormal
		m.blurRepoInputs()
		m.status = "Repo add canceled."
		return m, nil
	case "tab", "down":
		m.focusRepoField(1)
		return m, nil
	case "shift+tab", "up":
		m.focusRepoField(-1)
		return m, nil
	case "enter":
		if m.repoField < len(m.repoInputs)-1 {
			m.focusRepoField(1)
			return m, nil
		}
		m.submitRepo()
		return m, nil
	}
	var cmd tea.Cmd
	m.repoInputs[m.repoField], cmd = m.repoInputs[m.repoField].Update(key)
	return m, cmd
}

func (m tuiModel) updateReviewRepo(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "ctrl+c":
		m.mode = tuiModeNormal
		m.status = "Repo registration canceled."
	case "up", "k":
		m.reviewIdx = clampIndex(m.reviewIdx-1, len(m.pendingRepo.Skills))
	case "down", "j":
		m.reviewIdx = clampIndex(m.reviewIdx+1, len(m.pendingRepo.Skills))
	case " ":
		if len(m.pendingRepo.Skills) > 0 {
			m.pendingRepo.Skills[m.reviewIdx].Enabled = !m.pendingRepo.Skills[m.reviewIdx].Enabled
		}
	case "enter":
		m.registerPendingRepo()
	}
	return m, nil
}

func (m tuiModel) updateRepoDetail(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "left", "h":
		m.mode = tuiModeNormal
		m.status = "Returned to repositories."
	case "up", "k":
		m.detailIdx = clampIndex(m.detailIdx-1, len(m.currentRepo().Skills))
	case "down", "j":
		m.detailIdx = clampIndex(m.detailIdx+1, len(m.currentRepo().Skills))
	case " ":
		m.toggleCurrentRepoSkill()
	case "s":
		m.syncCurrentRepo()
	}
	return m, nil
}

func (m *tuiModel) moveTab(delta int) {
	next := int(m.tab) + delta
	if next < 0 {
		next = 2
	}
	if next > 2 {
		next = 0
	}
	m.tab = tuiTab(next)
}

func (m *tuiModel) moveSelection(delta int) {
	switch m.tab {
	case tuiTabCatalog:
		m.skillIdx = clampIndex(m.skillIdx+delta, len(m.skills))
	case tuiTabRepos:
		m.repoIdx = clampIndex(m.repoIdx+delta, len(m.repos))
	case tuiTabAgents:
		m.agentIdx = clampIndex(m.agentIdx+delta, len(m.agents))
	}
}

func clampIndex(index int, length int) int {
	if length == 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func (m *tuiModel) startAddAgent() {
	m.err = nil
	m.mode = tuiModeAddAgent
	m.agentInput = newInput("Agent path, e.g. .codex/skills")
	m.agentInput.Focus()
	m.status = "Enter an agent path, then press enter."
}

func (m *tuiModel) submitAgent() {
	agents, err := addAgent(m.paths, m.agentInput.Value())
	if err != nil {
		m.err = err
		m.status = ""
		return
	}
	m.err = nil
	m.agents = agents
	m.agentIdx = len(m.agents) - 1
	m.mode = tuiModeNormal
	m.agentInput.Blur()
	m.status = "Agent added."
}

func (m *tuiModel) removeSelectedAgent() {
	if len(m.agents) == 0 {
		m.status = "No agent selected."
		return
	}
	removed := m.agents[m.agentIdx].Path
	agents, err := removeAgent(m.paths, m.agentIdx)
	if err != nil {
		m.err = err
		m.status = ""
		return
	}
	m.err = nil
	m.agents = agents
	m.agentIdx = clampIndex(m.agentIdx, len(m.agents))
	m.status = "Removed agent " + removed + "."
}

func (m *tuiModel) startAddRepo() {
	m.err = nil
	m.mode = tuiModeAddRepo
	m.repoInputs = []textinput.Model{
		newInput("Git URL"),
		newInput("Repo ID, optional"),
		newInput("Branch, default main"),
	}
	m.repoField = 0
	m.repoInputs[0].Focus()
	m.status = "Enter repo details. Enter advances fields; enter on Branch registers the repo."
}

func (m *tuiModel) focusRepoField(delta int) {
	m.repoInputs[m.repoField].Blur()
	m.repoField += delta
	if m.repoField < 0 {
		m.repoField = len(m.repoInputs) - 1
	}
	if m.repoField >= len(m.repoInputs) {
		m.repoField = 0
	}
	m.repoInputs[m.repoField].Focus()
}

func (m *tuiModel) blurRepoInputs() {
	for i := range m.repoInputs {
		m.repoInputs[i].Blur()
	}
}

func (m *tuiModel) submitRepo() {
	req := RepoAddRequest{
		URL:    strings.TrimSpace(m.repoInputs[0].Value()),
		ID:     strings.TrimSpace(m.repoInputs[1].Value()),
		Branch: strings.TrimSpace(m.repoInputs[2].Value()),
	}
	repo, err := DiscoverRepo(m.paths, req)
	if err != nil {
		m.err = err
		m.status = ""
		return
	}
	m.err = nil
	m.blurRepoInputs()
	m.mode = tuiModeReviewRepo
	m.pendingRepo = repo
	m.reviewIdx = 0
	m.status = fmt.Sprintf("Found %d skills. Toggle selections with space, then press enter to register.", len(repo.Skills))
}

func (m *tuiModel) registerPendingRepo() {
	result, err := RegisterRepo(m.paths, m.pendingRepo)
	if err != nil {
		m.err = err
		m.status = ""
		return
	}
	m.err = nil
	m.mode = tuiModeNormal
	m.status = fmt.Sprintf("Registered repo %s with %d enabled skills.", result.Repo, countEnabled(result.Skills))
	m.pendingRepo = repoConfig{}
	m.refresh()
	for i, repo := range m.repos {
		if repo.ID == result.Repo {
			m.repoIdx = i
			break
		}
	}
}

func (m *tuiModel) disableSelectedCatalogSkill() {
	if len(m.skills) == 0 {
		m.status = "No skill selected."
		return
	}
	skill := m.skills[m.skillIdx]
	if err := disableCatalogSkill(m.paths, skill); err != nil {
		m.err = err
		m.status = ""
		return
	}
	m.err = nil
	m.status = "Disabled skill " + skill.Name + "."
	m.refresh()
}

func (m *tuiModel) startRepoDetail() {
	m.mode = tuiModeRepoDetail
	m.detailIdx = 0
	m.status = "Space toggles skill enablement. Press s to sync from Git."
}

func (m tuiModel) currentRepo() repoConfig {
	if len(m.repos) == 0 {
		return repoConfig{}
	}
	return m.repos[clampIndex(m.repoIdx, len(m.repos))]
}

func (m *tuiModel) toggleCurrentRepoSkill() {
	repo := m.currentRepo()
	if len(repo.Skills) == 0 {
		m.status = "No skills discovered for this repo."
		return
	}
	enabled := !repo.Skills[m.detailIdx].Enabled
	updated, err := setRepoSkillEnabled(m.paths, repo.ID, m.detailIdx, enabled)
	if err != nil {
		m.err = err
		m.status = ""
		return
	}
	m.err = nil
	m.repos[m.repoIdx] = updated
	m.status = "Updated skill " + updated.Skills[m.detailIdx].Name + "."
	m.refresh()
}

func (m *tuiModel) syncCurrentRepo() {
	repo := m.currentRepo()
	if repo.ID == "" {
		m.status = "No repo selected."
		return
	}
	updated, err := SyncRepo(m.paths, repo.ID)
	if err != nil {
		m.err = err
		m.status = ""
		return
	}
	m.err = nil
	m.repos[m.repoIdx] = updated
	m.detailIdx = clampIndex(m.detailIdx, len(updated.Skills))
	m.status = fmt.Sprintf("Synced repo %s. %d skills discovered.", updated.ID, len(updated.Skills))
	m.refresh()
}

func countEnabled(skills []Skill) int {
	count := 0
	for _, skill := range skills {
		if skill.Enabled {
			count++
		}
	}
	return count
}

func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}
	view := "\n" + tuiAccentStyle().Render(skilldropASCII) + "\n"
	view += m.renderTabs() + "\n\n"
	if m.err != nil {
		view += tuiErrorStyle().Render(m.err.Error()) + "\n\n"
	} else if m.status != "" {
		view += tuiMutedStyle().Render(m.status) + "\n\n"
	}
	switch m.tab {
	case tuiTabCatalog:
		view += m.renderCatalog()
	case tuiTabRepos:
		if m.mode == tuiModeReviewRepo {
			view += m.renderRepoReview()
		} else if m.mode == tuiModeRepoDetail {
			view += m.renderRepoDetail()
		} else {
			view += m.renderRepos()
		}
	case tuiTabAgents:
		view += m.renderAgents()
	}
	view += "\n" + tuiMutedStyle().Render(m.helpText()) + "\n"
	return fillScreen(view, m.height)
}

func (m tuiModel) renderTabs() string {
	names := []string{"Catalog", "Repos", "Agents"}
	parts := make([]string, 0, len(names))
	for i, name := range names {
		label := " " + name + " "
		if m.tab == tuiTab(i) {
			label = tuiAccentStyle().Render("[" + name + "]")
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " ")
}

func (m tuiModel) renderCatalog() string {
	out := tuiAccentStyle().Render("Registered Skills") + "\n"
	if len(m.skills) == 0 {
		return out + "  " + tuiMutedStyle().Render("No skills registered yet. Add a repo from the Repos tab.") + "\n"
	}
	for i, skill := range m.skills {
		cursor := " "
		if i == m.skillIdx {
			cursor = ">"
		}
		line := fmt.Sprintf("%s %s  %s  %s", cursor, skill.Name, skill.Repo, skill.SourcePath)
		if i == m.skillIdx {
			line = tuiAccentStyle().Render(line)
		}
		out += line + "\n"
	}
	return out
}

func (m tuiModel) renderRepoReview() string {
	out := tuiAccentStyle().Render("Review Discovered Skills") + "\n"
	out += fmt.Sprintf("  Repo: %s  %s  %s\n\n", m.pendingRepo.ID, m.pendingRepo.Git.URL, m.pendingRepo.Git.Branch)
	for i, skill := range m.pendingRepo.Skills {
		cursor := " "
		if i == m.reviewIdx {
			cursor = ">"
		}
		checked := "[ ]"
		if skill.Enabled {
			checked = "[x]"
		}
		line := fmt.Sprintf("%s %s %s  %s", cursor, checked, skill.Name, skill.SourcePath)
		if i == m.reviewIdx {
			line = tuiAccentStyle().Render(line)
		}
		out += line + "\n"
	}
	return out
}

func (m tuiModel) renderRepos() string {
	if m.mode == tuiModeAddRepo {
		return m.renderRepoForm()
	}
	out := tuiAccentStyle().Render("Registered Repositories") + "\n"
	if len(m.repos) == 0 {
		return out + "  " + tuiMutedStyle().Render("No repositories registered yet. Press a to add one.") + "\n"
	}
	for i, repo := range m.repos {
		cursor := " "
		if i == m.repoIdx {
			cursor = ">"
		}
		line := fmt.Sprintf("%s %s  %s  %s  %d skills", cursor, repo.ID, repo.Git.URL, repo.Git.Branch, len(repo.Skills))
		if i == m.repoIdx {
			line = tuiAccentStyle().Render(line)
		}
		out += line + "\n"
	}
	return out
}

func (m tuiModel) renderRepoDetail() string {
	repo := m.currentRepo()
	out := tuiAccentStyle().Render("Repository Details") + "\n"
	out += fmt.Sprintf("  ID:     %s\n", repo.ID)
	out += fmt.Sprintf("  URL:    %s\n", repo.Git.URL)
	out += fmt.Sprintf("  Branch: %s\n", repo.Git.Branch)
	out += fmt.Sprintf("  Skills: %d enabled, %d disabled\n\n", countEnabled(repo.Skills), len(repo.Skills)-countEnabled(repo.Skills))
	if len(repo.Skills) == 0 {
		return out + "  " + tuiMutedStyle().Render("No skills discovered.") + "\n"
	}
	for i, skill := range repo.Skills {
		cursor := " "
		if i == m.detailIdx {
			cursor = ">"
		}
		checked := "[ ]"
		if skill.Enabled {
			checked = "[x]"
		}
		line := fmt.Sprintf("%s %s %s  %s", cursor, checked, skill.Name, skill.SourcePath)
		if i == m.detailIdx {
			line = tuiAccentStyle().Render(line)
		}
		out += line + "\n"
	}
	return out
}

func (m tuiModel) renderRepoForm() string {
	labels := []string{"Git URL", "Repo ID", "Branch"}
	out := tuiAccentStyle().Render("Add Repository") + "\n"
	for i, input := range m.repoInputs {
		label := labels[i]
		if i == m.repoField {
			label = tuiAccentStyle().Render(label)
		}
		out += fmt.Sprintf("  %s: %s\n", label, input.View())
	}
	return out
}

func (m tuiModel) renderAgents() string {
	if m.mode == tuiModeAddAgent {
		return tuiAccentStyle().Render("Add Agent") + "\n  Path: " + m.agentInput.View() + "\n"
	}
	out := tuiAccentStyle().Render("Configured Agents") + "\n"
	if len(m.agents) == 0 {
		return out + "  " + tuiMutedStyle().Render("No agents configured yet. Press a to add one.") + "\n"
	}
	for i, agent := range m.agents {
		cursor := " "
		if i == m.agentIdx {
			cursor = ">"
		}
		line := fmt.Sprintf("%s %s  %s", cursor, agent.ID, agent.Path)
		if i == m.agentIdx {
			line = tuiAccentStyle().Render(line)
		}
		out += line + "\n"
	}
	return out
}

func (m tuiModel) helpText() string {
	switch m.mode {
	case tuiModeAddAgent:
		return "enter save agent  esc cancel"
	case tuiModeAddRepo:
		return "enter next/save  tab switch field  esc cancel"
	case tuiModeReviewRepo:
		return "space toggle skill  enter register selected skills  esc cancel"
	case tuiModeRepoDetail:
		return "space toggle skill  s sync repo  esc back"
	}
	switch m.tab {
	case tuiTabCatalog:
		return "d disable skill  left/right switch tabs  up/down move  q quit"
	case tuiTabRepos:
		return "a add repo  enter details  left/right switch tabs  up/down move  q quit"
	case tuiTabAgents:
		return "a add agent  d remove agent  left/right switch tabs  up/down move  q quit"
	default:
		return "left/right switch tabs  q quit"
	}
}

func fillScreen(view string, height int) string {
	if height <= 0 {
		return view
	}
	lines := strings.Count(view, "\n")
	if lines >= height {
		return view
	}
	return view + strings.Repeat("\n", height-lines)
}
