package skilldrop

import (
	"fmt"
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
)

type tuiModel struct {
	paths      paths
	workspace  string
	skills     []Skill
	agents     []Agent
	repos      []repoConfig
	tab        tuiTab
	mode       tuiMode
	skillIdx   int
	agentIdx   int
	repoIdx    int
	repoField  int
	width      int
	height     int
	status     string
	err        error
	quitting   bool
	agentInput textinput.Model
	repoInputs []textinput.Model
}

var (
	tuiTitleStyle    = lipgloss.NewStyle().Bold(true)
	tuiActiveStyle   = lipgloss.NewStyle().Bold(true)
	tuiMutedStyle    = lipgloss.NewStyle().Faint(true)
	tuiSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	tuiErrorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

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
	if m.status == "" {
		m.status = "Use left/right to move tabs. Use a/d in Agents, a in Repos."
	}
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
	case "a":
		switch m.tab {
		case tuiTabAgents:
			m.startAddAgent()
		case tuiTabRepos:
			m.startAddRepo()
		}
	case "d", "delete", "backspace":
		if m.tab == tuiTabAgents {
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
	result, err := RepoAdd(m.paths, req)
	if err != nil {
		m.err = err
		m.status = ""
		return
	}
	m.err = nil
	m.mode = tuiModeNormal
	m.blurRepoInputs()
	m.status = fmt.Sprintf("Registered repo %s with %d skills.", result.Repo, len(result.Skills))
	m.refresh()
	for i, repo := range m.repos {
		if repo.ID == result.Repo {
			m.repoIdx = i
			break
		}
	}
}

func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}
	view := tuiTitleStyle.Render(skilldropASCII) + "\n"
	view += m.renderTabs() + "\n\n"
	if m.err != nil {
		view += tuiErrorStyle.Render(m.err.Error()) + "\n\n"
	} else if m.status != "" {
		view += tuiMutedStyle.Render(m.status) + "\n\n"
	}
	switch m.tab {
	case tuiTabCatalog:
		view += m.renderCatalog()
	case tuiTabRepos:
		view += m.renderRepos()
	case tuiTabAgents:
		view += m.renderAgents()
	}
	view += "\n" + tuiMutedStyle.Render(m.helpText()) + "\n"
	return fillScreen(view, m.height)
}

func (m tuiModel) renderTabs() string {
	names := []string{"Catalog", "Repos", "Agents"}
	parts := make([]string, 0, len(names))
	for i, name := range names {
		label := " " + name + " "
		if m.tab == tuiTab(i) {
			label = tuiSelectedStyle.Render("[" + name + "]")
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " ")
}

func (m tuiModel) renderCatalog() string {
	out := tuiActiveStyle.Render("Registered Skills") + "\n"
	if len(m.skills) == 0 {
		return out + "  " + tuiMutedStyle.Render("No skills registered yet. Add a repo from the Repos tab.") + "\n"
	}
	for i, skill := range m.skills {
		cursor := " "
		if i == m.skillIdx {
			cursor = ">"
		}
		line := fmt.Sprintf("%s %s  %s  %s", cursor, skill.Name, skill.Repo, skill.SourcePath)
		if i == m.skillIdx {
			line = tuiSelectedStyle.Render(line)
		}
		out += line + "\n"
	}
	return out
}

func (m tuiModel) renderRepos() string {
	if m.mode == tuiModeAddRepo {
		return m.renderRepoForm()
	}
	out := tuiActiveStyle.Render("Registered Repositories") + "\n"
	if len(m.repos) == 0 {
		return out + "  " + tuiMutedStyle.Render("No repositories registered yet. Press a to add one.") + "\n"
	}
	for i, repo := range m.repos {
		cursor := " "
		if i == m.repoIdx {
			cursor = ">"
		}
		line := fmt.Sprintf("%s %s  %s  %s  %d skills", cursor, repo.ID, repo.Git.URL, repo.Git.Branch, len(repo.Skills))
		if i == m.repoIdx {
			line = tuiSelectedStyle.Render(line)
		}
		out += line + "\n"
	}
	return out
}

func (m tuiModel) renderRepoForm() string {
	labels := []string{"Git URL", "Repo ID", "Branch"}
	out := tuiActiveStyle.Render("Add Repository") + "\n"
	for i, input := range m.repoInputs {
		label := labels[i]
		if i == m.repoField {
			label = tuiSelectedStyle.Render(label)
		}
		out += fmt.Sprintf("  %s: %s\n", label, input.View())
	}
	return out
}

func (m tuiModel) renderAgents() string {
	if m.mode == tuiModeAddAgent {
		return tuiActiveStyle.Render("Add Agent") + "\n  Path: " + m.agentInput.View() + "\n"
	}
	out := tuiActiveStyle.Render("Configured Agents") + "\n"
	if len(m.agents) == 0 {
		return out + "  " + tuiMutedStyle.Render("No agents configured yet. Press a to add one.") + "\n"
	}
	for i, agent := range m.agents {
		cursor := " "
		if i == m.agentIdx {
			cursor = ">"
		}
		line := fmt.Sprintf("%s %s  %s", cursor, agent.ID, agent.Path)
		if i == m.agentIdx {
			line = tuiSelectedStyle.Render(line)
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
	}
	switch m.tab {
	case tuiTabCatalog:
		return "left/right switch tabs  up/down move  q quit"
	case tuiTabRepos:
		return "a add repo  left/right switch tabs  up/down move  q quit"
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
