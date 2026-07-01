package skilldrop

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type PickupRequest struct {
	Skill     Skill
	Agent     Agent
	CacheDir  string
	Workspace string
	Force     bool
	DryRun    bool
}

type PickupResult struct {
	Status       string `json:"status"`
	Skill        string `json:"skill"`
	Agent        string `json:"agent"`
	Destination  string `json:"destination"`
	FilesRemoved int    `json:"files_removed"`
}

type LocalChangeError struct {
	Path string
}

func (e *LocalChangeError) Error() string {
	return fmt.Sprintf("refusing to remove skill with local changes: %s", e.Path)
}

func Pickup(req PickupRequest) (PickupResult, error) {
	source := filepath.Join(req.CacheDir, "catalogs", req.Skill.Repo, filepath.FromSlash(req.Skill.SourcePath))
	dest := filepath.Join(req.Workspace, filepath.FromSlash(req.Agent.Path), req.Skill.Name)
	result := PickupResult{
		Status:      "picked_up",
		Skill:       req.Skill.Name,
		Agent:       req.Agent.ID,
		Destination: filepath.ToSlash(dest),
	}
	if req.DryRun {
		result.Status = "dry_run"
	}

	files, err := planPickup(source, dest, req.Force)
	if err != nil {
		return PickupResult{}, err
	}
	result.FilesRemoved = len(files)
	if req.DryRun {
		return result, nil
	}
	if err := os.RemoveAll(dest); err != nil {
		return PickupResult{}, &ExitError{Code: ExitGeneral, Err: err}
	}
	return result, nil
}

func planPickup(source string, dest string, force bool) ([]string, error) {
	destInfo, err := os.Stat(dest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &ExitError{Code: ExitMissingConfiguration, Err: fmt.Errorf("installed skill does not exist: %s", dest)}
		}
		return nil, &ExitError{Code: ExitGeneral, Err: err}
	}
	if !destInfo.IsDir() {
		return nil, &ExitError{Code: ExitGeneral, Err: fmt.Errorf("installed skill is not a directory: %s", dest)}
	}

	var files []string
	var localChanges []string
	err = filepath.WalkDir(dest, func(path string, entry fs.DirEntry, walkErr error) error {
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
		files = append(files, path)
		if force {
			return nil
		}
		rel, err := filepath.Rel(dest, path)
		if err != nil {
			return err
		}
		sourcePath := filepath.Join(source, rel)
		sourceInfo, err := os.Stat(sourcePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				localChanges = append(localChanges, path)
				return nil
			}
			return err
		}
		if sourceInfo.IsDir() {
			localChanges = append(localChanges, path)
			return nil
		}
		same, err := sameFileContent(sourcePath, path)
		if err != nil {
			return err
		}
		if !same {
			localChanges = append(localChanges, path)
		}
		return nil
	})
	if err != nil {
		return nil, &ExitError{Code: ExitGeneral, Err: err}
	}
	if len(localChanges) > 0 {
		sort.Strings(localChanges)
		return nil, &LocalChangeError{Path: filepath.ToSlash(localChanges[0])}
	}
	return files, nil
}
