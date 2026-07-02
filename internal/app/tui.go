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
	hover       tuiHoverTarget
	agentInput  textinput.Model
	repoInputs  []textinput.Model
	pendingRepo repoConfig
}

const tuiAccentColor = "208"
const tuiScreenTopPadding = 1

const (
	tuiHoverNone = iota
	tuiHoverTab
	tuiHoverRow
)

type tuiHoverTarget struct {
	kind int
	tab  tuiTab
	row  int
}

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

func tuiTabActiveStyle() lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true)
	if tuiColorEnabled() {
		style = style.Background(lipgloss.Color(tuiAccentColor)).Foreground(lipgloss.Color("0"))
	}
	return style
}

func tuiTabActiveHoverStyle() lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true).Reverse(true)
	if tuiColorEnabled() {
		style = style.Background(lipgloss.Color("229")).Foreground(lipgloss.Color("0"))
	}
	return style
}

func tuiTabHoverStyle() lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true).Reverse(true)
	if tuiColorEnabled() {
		style = style.Background(lipgloss.Color("236")).Foreground(lipgloss.Color("229"))
	}
	return style
}

func tuiHoverStyle() lipgloss.Style {
	style := lipgloss.NewStyle().Reverse(true)
	if tuiColorEnabled() {
		style = style.Background(lipgloss.Color("236")).Foreground(lipgloss.Color("229"))
	}
	return style
}

func tuiSelectedHoverStyle() lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true).Reverse(true)
	if tuiColorEnabled() {
		style = style.Background(lipgloss.Color("236")).Foreground(lipgloss.Color("229"))
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
	program := tea.NewProgram(model, tea.WithOutput(a.out), tea.WithAltScreen(), tea.WithMouseAllMotion())
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
	case tea.MouseMsg:
		return m.updateMouse(msg)
	default:
		return m, nil
	}
}

func (m tuiModel) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	target := m.hitTarget(int(msg.X), int(msg.Y))
	m.hover = target
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	switch target.kind {
	case tuiHoverTab:
		m.tab = target.tab
		m.mode = tuiModeNormal
	case tuiHoverRow:
		m.selectHoveredRow(target.row)
		if m.mode == tuiModeRepoDetail && len(m.currentRepo().Skills) > 0 {
			m.toggleCurrentRepoSkill()
		} else if m.mode == tuiModeNormal && m.tab == tuiTabRepos && len(m.repos) > 0 {
			m.startRepoDetail()
		}
	}
	return m, nil
}

func (m *tuiModel) selectHoveredRow(row int) {
	switch m.mode {
	case tuiModeReviewRepo:
		m.reviewIdx = clampIndex(row, len(m.pendingRepo.Skills))
	case tuiModeRepoDetail:
		m.detailIdx = clampIndex(row, len(m.currentRepo().Skills))
	case tuiModeNormal:
		switch m.tab {
		case tuiTabCatalog:
			m.skillIdx = clampIndex(row, len(m.skills))
		case tuiTabRepos:
			m.repoIdx = clampIndex(row, len(m.repos))
		case tuiTabAgents:
			m.agentIdx = clampIndex(row, len(m.agents))
		}
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
		tab := tuiTab(i)
		label := "[" + name + "]"
		hovered := m.hover.kind == tuiHoverTab && m.hover.tab == tab
		if m.tab == tab && hovered {
			label = tuiTabActiveHoverStyle().Render(label)
		} else if m.tab == tab {
			label = tuiTabActiveStyle().Render(label)
		} else if hovered {
			label = tuiTabHoverStyle().Render(label)
		} else {
			label = tuiAccentStyle().Render(label)
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
	rows := make([][]string, 0, len(m.skills))
	for i, skill := range m.skills {
		cursor := " "
		if i == m.skillIdx {
			cursor = withTUIStyle("selected", ">")
		}
		rows = append(rows, []string{
			cursor,
			m.tuiRowValue(i, skill.Name),
			skill.Repo,
			skill.SourcePath,
		})
	}
	return out + renderTUITable(m.tableWidth(), []tuiColumn{
		{name: " ", width: 1},
		{name: "Skill", width: 24},
		{name: "Repo", width: 16},
		{name: "Repo Path", width: 24, flex: true},
	}, rows)
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
	rows := make([][]string, 0, len(m.repos))
	for i, repo := range m.repos {
		cursor := " "
		if i == m.repoIdx {
			cursor = withTUIStyle("selected", ">")
		}
		rows = append(rows, []string{
			cursor,
			m.tuiRowValue(i, repo.ID),
			repo.Git.URL,
			repo.Git.Branch,
			fmt.Sprintf("%d", len(repo.Skills)),
			"details",
		})
	}
	return out + renderTUITable(m.tableWidth(), []tuiColumn{
		{name: " ", width: 1},
		{name: "Repo", width: 18},
		{name: "URL", width: 24, flex: true},
		{name: "Branch", width: 12},
		{name: "Skills", width: 6, right: true},
		{name: "Action", width: 8},
	}, rows)
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
	rows := make([][]string, 0, len(repo.Skills))
	for i, skill := range repo.Skills {
		cursor := " "
		if i == m.detailIdx {
			cursor = withTUIStyle("selected", ">")
		}
		checked := "[ ]"
		if skill.Enabled {
			checked = "[x]"
		}
		rows = append(rows, []string{
			cursor,
			checked,
			m.tuiRowValue(i, skill.Name),
			skill.SourcePath,
		})
	}
	return out + renderTUITable(m.tableWidth(), []tuiColumn{
		{name: " ", width: 1},
		{name: "Enabled", width: 7},
		{name: "Skill", width: 24},
		{name: "Repo Path", width: 24, flex: true},
	}, rows)
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
	rows := make([][]string, 0, len(m.agents))
	for i, agent := range m.agents {
		cursor := " "
		if i == m.agentIdx {
			cursor = withTUIStyle("selected", ">")
		}
		rows = append(rows, []string{
			cursor,
			m.tuiRowValue(i, agent.ID),
			agent.Path,
		})
	}
	return out + renderTUITable(m.tableWidth(), []tuiColumn{
		{name: " ", width: 1},
		{name: "Agent", width: 18},
		{name: "Path", width: 24, flex: true},
	}, rows)
}

func (m tuiModel) tuiRowValue(row int, value string) string {
	hovered := m.hover.kind == tuiHoverRow && m.hover.row == row
	selected := false
	switch m.mode {
	case tuiModeNormal:
		switch m.tab {
		case tuiTabCatalog:
			selected = row == m.skillIdx
		case tuiTabRepos:
			selected = row == m.repoIdx
		case tuiTabAgents:
			selected = row == m.agentIdx
		}
	case tuiModeReviewRepo:
		selected = row == m.reviewIdx
	case tuiModeRepoDetail:
		selected = row == m.detailIdx
	}
	if selected && hovered {
		return withTUIStyle("selected-hover", value)
	}
	if selected {
		return withTUIStyle("selected", value)
	}
	if hovered {
		return withTUIStyle("hover", value)
	}
	return value
}

func (m tuiModel) tableWidth() int {
	if m.width <= 0 {
		return 100
	}
	if m.width < 40 {
		return m.width
	}
	return m.width - 1
}

type tuiColumn struct {
	name  string
	width int
	right bool
	flex  bool
	min   int
}

func renderTUITable(targetWidth int, columns []tuiColumn, rows [][]string) string {
	var b strings.Builder
	columns = fitTUIColumns(columns, rows, targetWidth)
	header := make([]string, len(columns))
	for i, col := range columns {
		header[i] = tuiCell(col.name, col.width, col.right)
	}
	b.WriteString(tuiHeaderStyle().Render(strings.Join(header, "  ")))
	b.WriteByte('\n')
	b.WriteString(tuiLineStyle().Render(strings.Repeat("-", tuiTableLineWidth(columns))))
	b.WriteByte('\n')
	for _, row := range rows {
		values := make([]string, len(columns))
		for i, col := range columns {
			value := ""
			if i < len(row) {
				value = row[i]
			}
			values[i] = tuiStyledCell(value, col.width, col.right)
		}
		b.WriteString(strings.Join(values, "  "))
		b.WriteByte('\n')
	}
	return b.String()
}

func tuiHeaderStyle() lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true)
	if tuiColorEnabled() {
		style = style.Foreground(lipgloss.Color("250"))
	}
	return style
}

func tuiLineStyle() lipgloss.Style {
	style := lipgloss.NewStyle()
	if tuiColorEnabled() {
		style = style.Foreground(lipgloss.Color("238"))
	}
	return style
}

func fitTUIColumns(columns []tuiColumn, rows [][]string, targetWidth int) []tuiColumn {
	columns = sizeTUIColumnsToContent(columns, rows)
	if targetWidth <= 0 {
		return columns
	}
	width := tuiTableLineWidth(columns)
	if width <= targetWidth {
		for i := range columns {
			if columns[i].flex {
				columns[i].width += targetWidth - width
				break
			}
		}
		return columns
	}
	for width > targetWidth {
		flexIndex := -1
		for i, col := range columns {
			if col.flex {
				flexIndex = i
				break
			}
		}
		if flexIndex == -1 || columns[flexIndex].width <= minTUIColumnWidth(columns[flexIndex]) {
			break
		}
		columns[flexIndex].width--
		width--
	}
	for width > targetWidth && len(columns) > 3 {
		columns = columns[:len(columns)-1]
		width = tuiTableLineWidth(columns)
	}
	return columns
}

func sizeTUIColumnsToContent(columns []tuiColumn, rows [][]string) []tuiColumn {
	for i := range columns {
		contentWidth := len(columns[i].name)
		for _, row := range rows {
			if i >= len(row) {
				continue
			}
			_, plain := splitTUIStyle(row[i])
			if len(plain) > contentWidth {
				contentWidth = len(plain)
			}
		}
		if columns[i].flex {
			if columns[i].min == 0 {
				columns[i].min = columns[i].width
			}
			if contentWidth > columns[i].width {
				columns[i].width = contentWidth
			}
			continue
		}
		if contentWidth < columns[i].width {
			columns[i].width = contentWidth
		}
	}
	return columns
}

func minTUIColumnWidth(col tuiColumn) int {
	minWidth := col.min
	if minWidth == 0 {
		minWidth = col.width
	}
	if len(col.name) > minWidth {
		minWidth = len(col.name)
	}
	return minWidth
}

func tuiTableLineWidth(columns []tuiColumn) int {
	width := 0
	for i, col := range columns {
		width += col.width
		if i > 0 {
			width += 2
		}
	}
	return width
}

func tuiStyledCell(value string, width int, right bool) string {
	styleName, plain := splitTUIStyle(value)
	rendered := tuiCell(plain, width, right)
	switch styleName {
	case "selected":
		return tuiAccentStyle().Render(rendered)
	case "hover":
		return tuiHoverStyle().Render(rendered)
	case "selected-hover":
		return tuiSelectedHoverStyle().Render(rendered)
	default:
		return rendered
	}
}

func tuiCell(value string, width int, right bool) string {
	if width <= 0 {
		return ""
	}
	if len(value) > width {
		if width <= 3 {
			value = value[:width]
		} else {
			value = value[:width-3] + "..."
		}
	}
	if right {
		return fmt.Sprintf("%*s", width, value)
	}
	return fmt.Sprintf("%-*s", width, value)
}

func withTUIStyle(styleName, value string) string {
	return "\x00" + styleName + "\x00" + value
}

func splitTUIStyle(value string) (string, string) {
	if !strings.HasPrefix(value, "\x00") {
		return "", value
	}
	rest := strings.TrimPrefix(value, "\x00")
	parts := strings.SplitN(rest, "\x00", 2)
	if len(parts) != 2 {
		return "", value
	}
	return parts[0], parts[1]
}

func (m tuiModel) hitTarget(x, y int) tuiHoverTarget {
	if x < 0 || y < 0 {
		return tuiHoverTarget{}
	}
	y += tuiScreenTopPadding
	lines := m.hitTestLines()
	if y >= len(lines) {
		return tuiHoverTarget{}
	}
	line := lines[y]
	for i, name := range []string{"Catalog", "Repos", "Agents"} {
		label := "[" + name + "]"
		if tabX := strings.Index(line, label); tabX >= 0 && x >= tabX && x < tabX+len(label) {
			return tuiHoverTarget{kind: tuiHoverTab, tab: tuiTab(i)}
		}
	}
	return m.rowHitTarget(x, y, lines)
}

func (m tuiModel) hitTestLines() []string {
	plain := m
	plain.hover = tuiHoverTarget{}
	return strings.Split(stripTUIANSIEscapes(plain.View()), "\n")
}

func (m tuiModel) rowHitTarget(x, y int, lines []string) tuiHoverTarget {
	headerY, rows := m.hitTableBounds(lines)
	if headerY < 0 || rows == 0 {
		return tuiHoverTarget{}
	}
	rowY := headerY + 2
	if y < rowY || y >= rowY+rows {
		return tuiHoverTarget{}
	}
	if x >= len(lines[y]) {
		return tuiHoverTarget{}
	}
	return tuiHoverTarget{kind: tuiHoverRow, row: y - rowY}
}

func (m tuiModel) hitTableBounds(lines []string) (int, int) {
	var headers []string
	var rows int
	switch m.mode {
	case tuiModeReviewRepo:
		headers = []string{"Skill", "Source"}
		rows = len(m.pendingRepo.Skills)
	case tuiModeRepoDetail:
		headers = []string{"Enabled", "Skill", "Repo Path"}
		rows = len(m.currentRepo().Skills)
	case tuiModeNormal:
		switch m.tab {
		case tuiTabCatalog:
			headers = []string{"Skill", "Repo", "Repo Path"}
			rows = len(m.skills)
		case tuiTabRepos:
			headers = []string{"Repo", "URL", "Action"}
			rows = len(m.repos)
		case tuiTabAgents:
			headers = []string{"Agent", "Path"}
			rows = len(m.agents)
		}
	}
	for i, line := range lines {
		found := true
		for _, header := range headers {
			if !strings.Contains(line, header) {
				found = false
				break
			}
		}
		if found {
			return i, rows
		}
	}
	return -1, 0
}

func stripTUIANSIEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\x1b' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			break
		}
		switch s[i] {
		case '[':
			for i+1 < len(s) {
				i++
				if s[i] >= '@' && s[i] <= '~' {
					break
				}
			}
		case ']':
			for i+1 < len(s) {
				i++
				if s[i] == '\a' {
					break
				}
				if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
					i++
					break
				}
			}
		}
	}
	return b.String()
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
