package skilldrop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type paths struct {
	configDir string
	cacheDir  string
}

func defaultPaths() (paths, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		var err error
		configHome, err = os.UserConfigDir()
		if err != nil {
			return paths{}, err
		}
	}
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		var err error
		cacheHome, err = os.UserCacheDir()
		if err != nil {
			return paths{}, err
		}
	}
	return paths{
		configDir: filepath.Join(configHome, "skilldrop"),
		cacheDir:  filepath.Join(cacheHome, "skilldrop"),
	}, nil
}

func ensureStorage(p paths) error {
	dirs := []string{
		p.configDir,
		filepath.Join(p.configDir, "repos"),
		p.cacheDir,
		filepath.Join(p.cacheDir, "catalogs"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return &ExitError{Code: ExitMissingConfiguration, Err: err}
		}
	}
	agentsPath := filepath.Join(p.configDir, "agents.yaml")
	if _, err := os.Stat(agentsPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return &ExitError{Code: ExitMissingConfiguration, Err: err}
	}
	data := []byte("version: 1\n\nagents: []\n")
	if err := os.WriteFile(agentsPath, data, 0o644); err != nil {
		return &ExitError{Code: ExitMissingConfiguration, Err: err}
	}
	return nil
}

type repoConfig struct {
	Version int `yaml:"version"`
	ID      string
	Name    string
	Git     struct {
		URL    string `yaml:"url"`
		Branch string `yaml:"branch"`
	} `yaml:"git"`
	Skills []Skill `yaml:"skills"`
}

type Skill struct {
	Name       string `json:"name" yaml:"name"`
	Repo       string `json:"repo" yaml:"-"`
	SourcePath string `json:"source_path" yaml:"source_path"`
	Enabled    bool   `json:"enabled" yaml:"enabled"`
}

type Catalog struct {
	Skills []Skill
}

func (c Catalog) Find(name string) (Skill, bool) {
	for _, skill := range c.Skills {
		if skill.Name == name && skill.Enabled {
			return skill, true
		}
	}
	return Skill{}, false
}

func loadCatalog(p paths) (Catalog, error) {
	reposDir := filepath.Join(p.configDir, "repos")
	entries, err := os.ReadDir(reposDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Catalog{}, &ExitError{Code: ExitMissingConfiguration, Err: fmt.Errorf("missing repo config directory: %s", reposDir)}
		}
		return Catalog{}, &ExitError{Code: ExitMissingConfiguration, Err: err}
	}

	catalog := Catalog{Skills: []Skill{}}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		repo, err := loadRepoConfig(filepath.Join(reposDir, entry.Name()))
		if err != nil {
			return Catalog{}, err
		}
		for _, skill := range repo.Skills {
			if skill.Name == "" || skill.SourcePath == "" || !skill.Enabled {
				continue
			}
			skill.Repo = repo.ID
			catalog.Skills = append(catalog.Skills, skill)
		}
	}
	sort.Slice(catalog.Skills, func(i, j int) bool {
		return catalog.Skills[i].Name < catalog.Skills[j].Name
	})
	return catalog, nil
}

func loadRepoConfigs(p paths) ([]repoConfig, error) {
	reposDir := filepath.Join(p.configDir, "repos")
	entries, err := os.ReadDir(reposDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &ExitError{Code: ExitMissingConfiguration, Err: fmt.Errorf("missing repo config directory: %s", reposDir)}
		}
		return nil, &ExitError{Code: ExitMissingConfiguration, Err: err}
	}
	repos := []repoConfig{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		repo, err := loadRepoConfig(filepath.Join(reposDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].ID < repos[j].ID
	})
	return repos, nil
}

func loadRepoConfigsAllowMissing(p paths) ([]repoConfig, error) {
	repos, err := loadRepoConfigs(p)
	if err == nil {
		return repos, nil
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) && exitErr.Code == ExitMissingConfiguration {
		return nil, nil
	}
	return nil, err
}

func loadCatalogAllowMissing(p paths) (Catalog, error) {
	catalog, err := loadCatalog(p)
	if err == nil {
		return catalog, nil
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) && exitErr.Code == ExitMissingConfiguration {
		return Catalog{}, nil
	}
	return Catalog{}, err
}

func loadRepoConfig(path string) (repoConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return repoConfig{}, &ExitError{Code: ExitMissingConfiguration, Err: err}
	}
	var repo repoConfig
	if err := yaml.Unmarshal(data, &repo); err != nil {
		return repoConfig{}, &ExitError{Code: ExitMissingConfiguration, Err: fmt.Errorf("parse %s: %w", path, err)}
	}
	if repo.ID == "" {
		repo.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return repo, nil
}

func loadRepoConfigByID(p paths, repoID string) (repoConfig, error) {
	return loadRepoConfig(filepath.Join(p.configDir, "repos", repoID+".yaml"))
}

func writeRepoConfig(p paths, repo repoConfig) error {
	path := filepath.Join(p.configDir, "repos", repo.ID+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return &ExitError{Code: ExitMissingConfiguration, Err: err}
	}
	data, err := yaml.Marshal(repo)
	if err != nil {
		return &ExitError{Code: ExitMissingConfiguration, Err: err}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return &ExitError{Code: ExitMissingConfiguration, Err: err}
	}
	return nil
}

func setRepoSkillEnabled(p paths, repoID string, skillIndex int, enabled bool) (repoConfig, error) {
	repo, err := loadRepoConfigByID(p, repoID)
	if err != nil {
		return repoConfig{}, err
	}
	if skillIndex < 0 || skillIndex >= len(repo.Skills) {
		return repoConfig{}, &ExitError{Code: ExitInvalidUsage, Err: errors.New("skill selection is out of range")}
	}
	repo.Skills[skillIndex].Enabled = enabled
	if err := writeRepoConfig(p, repo); err != nil {
		return repoConfig{}, err
	}
	return loadRepoConfigByID(p, repoID)
}

func disableCatalogSkill(p paths, skill Skill) error {
	repo, err := loadRepoConfigByID(p, skill.Repo)
	if err != nil {
		return err
	}
	for i := range repo.Skills {
		if repo.Skills[i].Name == skill.Name {
			repo.Skills[i].Enabled = false
			return writeRepoConfig(p, repo)
		}
	}
	return &ExitError{Code: ExitSkillNotFound, Err: fmt.Errorf("skill not found: %s", skill.Name)}
}

type Agent struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

func loadAgents(p paths) ([]Agent, error) {
	path := filepath.Join(p.configDir, "agents.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &ExitError{Code: ExitMissingConfiguration, Err: fmt.Errorf("missing agent config: %s", path)}
		}
		return nil, &ExitError{Code: ExitMissingConfiguration, Err: err}
	}
	var raw struct {
		Version int         `yaml:"version"`
		Agents  []yaml.Node `yaml:"agents"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, &ExitError{Code: ExitMissingConfiguration, Err: fmt.Errorf("parse %s: %w", path, err)}
	}
	agents := []Agent{}
	for i := range raw.Agents {
		agent, err := parseAgentNode(&raw.Agents[i])
		if err != nil {
			return nil, &ExitError{Code: ExitMissingConfiguration, Err: fmt.Errorf("parse %s: %w", path, err)}
		}
		if agent.Path == "" {
			continue
		}
		if agent.ID == "" {
			agent.ID = deriveAgentID(agent.Path)
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

func loadAgentsAllowMissing(p paths) ([]Agent, error) {
	agents, err := loadAgents(p)
	if err == nil {
		return agents, nil
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) && exitErr.Code == ExitMissingConfiguration {
		return nil, nil
	}
	return nil, err
}

func writeAgents(p paths, agents []Agent) error {
	path := filepath.Join(p.configDir, "agents.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return &ExitError{Code: ExitMissingConfiguration, Err: err}
	}
	paths := make([]string, 0, len(agents))
	seen := map[string]bool{}
	for _, agent := range agents {
		cleanPath := strings.TrimSpace(agent.Path)
		if cleanPath == "" || seen[cleanPath] {
			continue
		}
		seen[cleanPath] = true
		paths = append(paths, cleanPath)
	}
	data, err := yaml.Marshal(struct {
		Version int      `yaml:"version"`
		Agents  []string `yaml:"agents"`
	}{
		Version: 1,
		Agents:  paths,
	})
	if err != nil {
		return &ExitError{Code: ExitMissingConfiguration, Err: err}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return &ExitError{Code: ExitMissingConfiguration, Err: err}
	}
	return nil
}

func addAgent(p paths, agentPath string) ([]Agent, error) {
	agentPath = strings.TrimSpace(agentPath)
	if agentPath == "" {
		return nil, &ExitError{Code: ExitInvalidUsage, Err: errors.New("agent path is required")}
	}
	agents, err := loadAgentsAllowMissing(p)
	if err != nil {
		return nil, err
	}
	for _, agent := range agents {
		if agent.Path == agentPath {
			return agents, nil
		}
	}
	agents = append(agents, Agent{ID: deriveAgentID(agentPath), Path: agentPath})
	if err := writeAgents(p, agents); err != nil {
		return nil, err
	}
	return loadAgentsAllowMissing(p)
}

func removeAgent(p paths, index int) ([]Agent, error) {
	agents, err := loadAgentsAllowMissing(p)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(agents) {
		return agents, &ExitError{Code: ExitInvalidUsage, Err: errors.New("agent selection is out of range")}
	}
	agents = append(agents[:index], agents[index+1:]...)
	if err := writeAgents(p, agents); err != nil {
		return nil, err
	}
	return loadAgentsAllowMissing(p)
}

func parseAgentNode(node *yaml.Node) (Agent, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		return Agent{Path: node.Value}, nil
	case yaml.MappingNode:
		var agent Agent
		var raw struct {
			ID   string `yaml:"id"`
			Name string `yaml:"name"`
			Path string `yaml:"path"`
		}
		if err := node.Decode(&raw); err != nil {
			return Agent{}, err
		}
		agent.ID = raw.ID
		if agent.ID == "" {
			agent.ID = raw.Name
		}
		agent.Path = raw.Path
		return agent, nil
	default:
		return Agent{}, errors.New("agent must be a path string or mapping")
	}
}

func deriveAgentID(path string) string {
	first := strings.Split(filepath.ToSlash(strings.Trim(path, "/")), "/")[0]
	first = strings.TrimPrefix(first, ".")
	if first == "" {
		return path
	}
	return first
}

func findAgent(agents []Agent, id string) (Agent, bool) {
	for _, agent := range agents {
		if agent.ID == id || agent.Path == id {
			return agent, true
		}
	}
	return Agent{}, false
}
