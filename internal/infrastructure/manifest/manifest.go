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
	"sync/atomic"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

const manifestContent = "version: 1\nproject_id: %s\n"

type manifestFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

type openManifestFile func(string, int, os.FileMode) (manifestFile, error)

// Store 负责 Repository 内 `.keystone/project.yaml` 的读写。
type Store struct {
	openFile openManifestFile
}

var temporaryManifestSequence uint64

// Ensure 严格读取现有 Manifest，或使用 candidate 创建缺失 Manifest。
func (s Store) Ensure(ctx context.Context, binding domain.RepositoryBinding, candidate domain.ProjectID) (domain.ProjectManifest, error) {
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
	manifestBytes, err := s.readOrCreate(ctx, binding.ManifestPath, candidate)
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

func (s Store) readOrCreate(ctx context.Context, path string, candidate domain.ProjectID) (data []byte, err error) {
	info, err := os.Lstat(path)
	if err == nil {
		return readExisting(path, info)
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect project manifest: %w", domain.ErrManifestUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create project manifest: %w", err)
	}
	data = []byte(fmt.Sprintf(manifestContent, candidate))
	file, temporaryPath, err := s.createTemporaryFile(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("create project manifest: %w", domain.ErrManifestUnavailable)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		if temporaryPath != "" {
			if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove temporary project manifest: %w", removeErr))
			}
		}
	}()
	if written, writeErr := file.Write(data); writeErr != nil || written != len(data) {
		if writeErr == nil {
			writeErr = io.ErrShortWrite
		}
		return nil, fmt.Errorf("write project manifest: %w", errors.Join(domain.ErrManifestUnavailable, writeErr))
	}
	if syncErr := file.Sync(); syncErr != nil {
		return nil, fmt.Errorf("sync project manifest: %w", errors.Join(domain.ErrManifestUnavailable, syncErr))
	}
	closeErr := file.Close()
	closed = true
	if closeErr != nil {
		return nil, fmt.Errorf("close project manifest: %w", errors.Join(domain.ErrManifestUnavailable, closeErr))
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return readExisting(path, nil)
		}
		return nil, fmt.Errorf("publish project manifest: %w", errors.Join(domain.ErrManifestUnavailable, err))
	}
	if err := os.Remove(temporaryPath); err != nil {
		return nil, fmt.Errorf("remove temporary project manifest: %w", errors.Join(domain.ErrManifestUnavailable, err))
	}
	temporaryPath = ""
	return readExisting(path, nil)
}

func (s Store) createTemporaryFile(dir string) (manifestFile, string, error) {
	openFile := s.openFile
	if openFile == nil {
		openFile = func(path string, flags int, mode os.FileMode) (manifestFile, error) {
			return os.OpenFile(path, flags, mode)
		}
	}
	for range 100 {
		sequence := atomic.AddUint64(&temporaryManifestSequence, 1)
		path := filepath.Join(dir, fmt.Sprintf(".project.yaml.tmp-%d-%d", os.Getpid(), sequence))
		file, err := openFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			return file, path, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("allocate temporary project manifest: %w", os.ErrExist)
}

func readExisting(path string, info os.FileInfo) ([]byte, error) {
	if info == nil {
		var err error
		info, err = os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("read project manifest: %w", domain.ErrManifestUnavailable)
		}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("manifest file topology is invalid: %w", domain.ErrManifestInvalid)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read project manifest: %w", domain.ErrManifestUnavailable)
	}
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
