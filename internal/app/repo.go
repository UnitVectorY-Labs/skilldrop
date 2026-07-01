package skilldrop

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type RepoAddRequest struct {
	URL    string
	ID     string
	Branch string
}

type RepoAddResult struct {
	Status string  `json:"status"`
	Repo   string  `json:"repo"`
	URL    string  `json:"url"`
	Branch string  `json:"branch"`
	Skills []Skill `json:"skills"`
}

type DuplicateSkillError struct {
	Name string
}

func (e *DuplicateSkillError) Error() string {
	return fmt.Sprintf("skill name already exists: %s", e.Name)
}

func RepoAdd(p paths, req RepoAddRequest) (RepoAddResult, error) {
	repo, err := DiscoverRepo(p, req)
	if err != nil {
		return RepoAddResult{}, err
	}
	return RegisterRepo(p, repo)
}

func DiscoverRepo(p paths, req RepoAddRequest) (repoConfig, error) {
	if req.URL == "" {
		return repoConfig{}, &ExitError{Code: ExitInvalidUsage, Err: errors.New("repo add requires a git URL")}
	}
	if req.Branch == "" {
		req.Branch = "main"
	}
	if req.ID == "" {
		req.ID = deriveRepoID(req.URL)
	}
	if !validRepoID(req.ID) {
		return repoConfig{}, &ExitError{Code: ExitInvalidUsage, Err: fmt.Errorf("invalid repo id: %s", req.ID)}
	}

	cloneDir := filepath.Join(p.cacheDir, "catalogs", req.ID)
	if err := syncRepo(req.URL, req.Branch, cloneDir); err != nil {
		return repoConfig{}, &ExitError{Code: ExitGitSyncFailure, Err: err}
	}
	discovered, err := scanSkills(cloneDir)
	if err != nil {
		return repoConfig{}, err
	}
	if len(discovered) == 0 {
		return repoConfig{}, &ExitError{Code: ExitFrontMatterParseFailure, Err: errors.New("no skills found")}
	}
	repo := repoConfig{
		Version: 1,
		ID:      req.ID,
		Name:    req.ID,
		Skills:  discovered,
	}
	repo.Git.URL = req.URL
	repo.Git.Branch = req.Branch
	return repo, nil
}

func RegisterRepo(p paths, repo repoConfig) (RepoAddResult, error) {
	existing, err := loadCatalogAllowMissing(p)
	if err != nil {
		return RepoAddResult{}, err
	}
	existingNames := map[string]bool{}
	for _, skill := range existing.Skills {
		if skill.Repo != repo.ID {
			existingNames[skill.Name] = true
		}
	}
	seen := map[string]bool{}
	for _, skill := range repo.Skills {
		if !skill.Enabled {
			continue
		}
		if seen[skill.Name] || existingNames[skill.Name] {
			return RepoAddResult{}, &DuplicateSkillError{Name: skill.Name}
		}
		seen[skill.Name] = true
	}
	if err := writeRepoConfig(p, repo); err != nil {
		return RepoAddResult{}, err
	}
	for i := range repo.Skills {
		repo.Skills[i].Repo = repo.ID
	}
	return RepoAddResult{
		Status: "repo_added",
		Repo:   repo.ID,
		URL:    repo.Git.URL,
		Branch: repo.Git.Branch,
		Skills: repo.Skills,
	}, nil
}

func SyncRepo(p paths, repoID string) (repoConfig, error) {
	repo, err := loadRepoConfigByID(p, repoID)
	if err != nil {
		return repoConfig{}, err
	}
	cloneDir := filepath.Join(p.cacheDir, "catalogs", repo.ID)
	if err := syncRepo(repo.Git.URL, repo.Git.Branch, cloneDir); err != nil {
		return repoConfig{}, &ExitError{Code: ExitGitSyncFailure, Err: err}
	}
	discovered, err := scanSkills(cloneDir)
	if err != nil {
		return repoConfig{}, err
	}
	enabledByName := map[string]bool{}
	for _, skill := range repo.Skills {
		enabledByName[skill.Name] = skill.Enabled
	}
	for i := range discovered {
		if enabled, ok := enabledByName[discovered[i].Name]; ok {
			discovered[i].Enabled = enabled
		} else {
			discovered[i].Enabled = false
		}
	}
	repo.Skills = discovered
	if err := writeRepoConfig(p, repo); err != nil {
		return repoConfig{}, err
	}
	return loadRepoConfigByID(p, repo.ID)
}

func syncRepo(repoURL string, branch string, dest string) error {
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		if err := runGit(dest, "fetch", "origin", branch); err != nil {
			return err
		}
		if err := runGit(dest, "checkout", branch); err != nil {
			return err
		}
		return runGit(dest, "pull", "--ff-only", "origin", branch)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", "--branch", branch, repoURL, dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func scanSkills(root string) ([]Skill, error) {
	skills := []Skill{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "SKILL.md" {
			return nil
		}
		name, err := readSkillName(path)
		if err != nil {
			return err
		}
		sourceDir := filepath.Dir(path)
		rel, err := filepath.Rel(root, sourceDir)
		if err != nil {
			return err
		}
		skills = append(skills, Skill{
			Name:       name,
			SourcePath: filepath.ToSlash(rel),
			Enabled:    true,
		})
		return nil
	})
	if err != nil {
		return nil, &ExitError{Code: ExitFrontMatterParseFailure, Err: err}
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	return skills, nil
}

func readSkillName(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return "", fmt.Errorf("%s missing YAML front matter", path)
	}
	rest := strings.TrimPrefix(text, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", fmt.Errorf("%s missing closing YAML front matter marker", path)
	}
	var frontMatter struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &frontMatter); err != nil {
		return "", fmt.Errorf("parse front matter %s: %w", path, err)
	}
	if strings.TrimSpace(frontMatter.Name) == "" {
		return "", fmt.Errorf("%s front matter missing name", path)
	}
	return strings.TrimSpace(frontMatter.Name), nil
}

func deriveRepoID(repoURL string) string {
	if parsed, err := url.Parse(repoURL); err == nil && parsed.Path != "" {
		repoURL = parsed.Path
	}
	repoURL = strings.TrimSuffix(repoURL, ".git")
	repoURL = strings.TrimRight(repoURL, "/")
	if idx := strings.LastIndex(repoURL, "/"); idx >= 0 {
		repoURL = repoURL[idx+1:]
	}
	if idx := strings.LastIndex(repoURL, ":"); idx >= 0 {
		repoURL = repoURL[idx+1:]
	}
	id := strings.ToLower(repoURL)
	id = regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(id, "-")
	id = strings.Trim(id, ".-_")
	if id == "" {
		return "repo"
	}
	return id
}

func validRepoID(id string) bool {
	return regexp.MustCompile(`^[A-Za-z0-9._-]+$`).MatchString(id) && id != "." && id != ".."
}
