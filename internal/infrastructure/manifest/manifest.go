// Package manifest 提供 ProjectManifest V1 的严格文件适配器。
package manifest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

const manifestContent = "version: 1\nproject_id: %s\n"

// Store 负责 Repository 内 `.keystone/project.yaml` 的读写。
type Store struct{}

// Ensure 严格读取现有 Manifest，或使用 candidate 创建缺失 Manifest。
func (Store) Ensure(ctx context.Context, binding domain.RepositoryBinding, candidate domain.ProjectID) (domain.ProjectManifest, error) {
	if ctx == nil {
		return domain.ProjectManifest{}, fmt.Errorf("ensure manifest: %w", domain.ErrInvalidRequest)
	}
	if err := binding.Validate(); err != nil {
		return domain.ProjectManifest{}, fmt.Errorf("ensure manifest binding: %w", err)
	}
	if err := candidate.Validate(); err != nil {
		return domain.ProjectManifest{}, err
	}
	if err := prepareDirectory(filepath.Dir(binding.ManifestPath)); err != nil {
		return domain.ProjectManifest{}, err
	}
	manifestBytes, err := readOrCreate(ctx, binding.ManifestPath, candidate)
	if err != nil {
		return domain.ProjectManifest{}, err
	}
	manifest, err := parse(manifestBytes)
	if err != nil {
		return domain.ProjectManifest{}, err
	}
	return manifest, nil
}

func prepareDirectory(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("create manifest directory: %w", domain.ErrManifestUnavailable)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("inspect manifest directory: %w", domain.ErrManifestUnavailable)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("manifest directory topology is invalid: %w", domain.ErrManifestInvalid)
	}
	return nil
}

func readOrCreate(ctx context.Context, path string, candidate domain.ProjectID) ([]byte, error) {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("manifest file topology is invalid: %w", domain.ErrManifestInvalid)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read project manifest: %w", domain.ErrManifestUnavailable)
		}
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect project manifest: %w", domain.ErrManifestUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create project manifest: %w", err)
	}
	data := []byte(fmt.Sprintf(manifestContent, candidate))
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return readOrCreate(ctx, path, candidate)
		}
		return nil, fmt.Errorf("create project manifest: %w", domain.ErrManifestUnavailable)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	if written, err := file.Write(data); err != nil || written != len(data) {
		if err == nil {
			err = io.ErrShortWrite
		}
		return nil, fmt.Errorf("write project manifest: %w", domain.ErrManifestUnavailable)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("sync project manifest: %w", domain.ErrManifestUnavailable)
	}
	if err := file.Close(); err != nil {
		closed = true
		return nil, fmt.Errorf("close project manifest: %w", domain.ErrManifestUnavailable)
	}
	closed = true
	return data, nil
}

func parse(data []byte) (domain.ProjectManifest, error) {
	text := string(data)
	if text == "" || !strings.HasSuffix(text, "\n") {
		return domain.ProjectManifest{}, fmt.Errorf("parse project manifest: %w", domain.ErrManifestInvalid)
	}
	if strings.Contains(text, "\r") {
		if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\r") {
			return domain.ProjectManifest{}, fmt.Errorf("parse project manifest: %w", domain.ErrManifestInvalid)
		}
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	if strings.Contains(text, "---") || strings.Contains(text, "...") || strings.ContainsAny(text, "&*!<>") {
		return domain.ProjectManifest{}, fmt.Errorf("parse project manifest: %w", domain.ErrManifestInvalid)
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) != 2 {
		return domain.ProjectManifest{}, fmt.Errorf("parse project manifest: %w", domain.ErrManifestInvalid)
	}
	fields := make(map[string]string, 2)
	for _, line := range lines {
		if strings.TrimSpace(line) != line || strings.Count(line, ":") != 1 {
			return domain.ProjectManifest{}, fmt.Errorf("parse project manifest: %w", domain.ErrManifestInvalid)
		}
		parts := strings.SplitN(line, ":", 2)
		key, value := parts[0], strings.TrimSpace(parts[1])
		if key != "version" && key != "project_id" || value == "" || strings.Contains(value, "#") {
			return domain.ProjectManifest{}, fmt.Errorf("parse project manifest: %w", domain.ErrManifestInvalid)
		}
		if _, exists := fields[key]; exists {
			return domain.ProjectManifest{}, fmt.Errorf("parse project manifest: %w", domain.ErrManifestInvalid)
		}
		fields[key] = value
	}
	if len(fields) != 2 {
		return domain.ProjectManifest{}, fmt.Errorf("parse project manifest: %w", domain.ErrManifestInvalid)
	}
	version, err := strconv.Atoi(fields["version"])
	if err != nil || fields["version"] != "1" {
		return domain.ProjectManifest{}, fmt.Errorf("parse project manifest: %w", domain.ErrManifestInvalid)
	}
	manifest := domain.ProjectManifest{Version: version, ProjectID: domain.ProjectID(fields["project_id"])}
	if err := manifest.Validate(); err != nil {
		return domain.ProjectManifest{}, err
	}
	return manifest, nil
}
