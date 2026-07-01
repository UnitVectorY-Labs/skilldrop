package skilldrop

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type DropRequest struct {
	Skill     Skill
	Agent     Agent
	CacheDir  string
	Workspace string
	Force     bool
	DryRun    bool
}

type DropResult struct {
	Status           string     `json:"status"`
	Skill            string     `json:"skill"`
	Agent            string     `json:"agent"`
	Source           string     `json:"source"`
	Destination      string     `json:"destination"`
	FilesWritten     int        `json:"files_written"`
	FilesUnchanged   int        `json:"files_unchanged"`
	FilesOverwritten int        `json:"files_overwritten"`
	Files            []DropFile `json:"-"`
}

type DropFile struct {
	Path   string
	Action string
}

type ConflictError struct {
	Path string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("would overwrite %s", e.Path)
}

type copyPlan struct {
	source string
	dest   string
	rel    string
	action copyAction
	mode   fs.FileMode
}

type copyAction int

const (
	copyWrite copyAction = iota
	copyUnchanged
	copyOverwrite
)

func (a copyAction) Label() string {
	switch a {
	case copyWrite:
		return "add"
	case copyUnchanged:
		return "same"
	case copyOverwrite:
		return "updated"
	default:
		return "change"
	}
}

func Drop(req DropRequest) (DropResult, error) {
	source := filepath.Join(req.CacheDir, "catalogs", req.Skill.Repo, filepath.FromSlash(req.Skill.SourcePath))
	dest := filepath.Join(req.Workspace, filepath.FromSlash(req.Agent.Path), req.Skill.Name)
	result := DropResult{
		Status:      "dropped",
		Skill:       req.Skill.Name,
		Agent:       req.Agent.ID,
		Source:      filepath.ToSlash(source),
		Destination: filepath.ToSlash(dest),
	}
	if req.DryRun {
		result.Status = "dry_run"
	}

	plans, err := planCopy(source, dest, req.Force)
	if err != nil {
		return DropResult{}, err
	}
	for _, plan := range plans {
		result.Files = append(result.Files, DropFile{
			Path:   filepath.ToSlash(plan.rel),
			Action: plan.action.Label(),
		})
		switch plan.action {
		case copyWrite:
			result.FilesWritten++
		case copyUnchanged:
			result.FilesUnchanged++
		case copyOverwrite:
			result.FilesOverwritten++
		}
	}
	if req.DryRun {
		return result, nil
	}
	for _, plan := range plans {
		if plan.action == copyUnchanged {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(plan.dest), 0o755); err != nil {
			return DropResult{}, &ExitError{Code: ExitGeneral, Err: err}
		}
		if err := copyFile(plan.source, plan.dest, plan.mode); err != nil {
			return DropResult{}, &ExitError{Code: ExitGeneral, Err: err}
		}
	}
	return result, nil
}

func planCopy(source string, dest string, force bool) ([]copyPlan, error) {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &ExitError{Code: ExitMissingConfiguration, Err: fmt.Errorf("missing cached skill source: %s", source)}
		}
		return nil, &ExitError{Code: ExitGeneral, Err: err}
	}
	if !sourceInfo.IsDir() {
		return nil, &ExitError{Code: ExitGeneral, Err: fmt.Errorf("skill source is not a directory: %s", source)}
	}

	var plans []copyPlan
	var conflicts []string
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		action, err := classifyCopy(path, target, force)
		if err != nil {
			var conflict *ConflictError
			if errors.As(err, &conflict) {
				conflicts = append(conflicts, conflict.Path)
				return nil
			}
			return err
		}
		plans = append(plans, copyPlan{source: path, dest: target, rel: rel, action: action, mode: info.Mode()})
		return nil
	})
	if err != nil {
		return nil, &ExitError{Code: ExitGeneral, Err: err}
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return nil, &ConflictError{Path: filepath.ToSlash(conflicts[0])}
	}
	return plans, nil
}

func classifyCopy(source string, dest string, force bool) (copyAction, error) {
	destInfo, err := os.Stat(dest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return copyWrite, nil
		}
		return copyWrite, err
	}
	if destInfo.IsDir() {
		return copyWrite, &ConflictError{Path: dest}
	}
	same, err := sameFileContent(source, dest)
	if err != nil {
		return copyWrite, err
	}
	if same {
		return copyUnchanged, nil
	}
	if !force {
		return copyWrite, &ConflictError{Path: dest}
	}
	return copyOverwrite, nil
}

func sameFileContent(a string, b string) (bool, error) {
	left, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	right, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(left, right), nil
}

func copyFile(source string, dest string, mode fs.FileMode) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, mode.Perm())
}
