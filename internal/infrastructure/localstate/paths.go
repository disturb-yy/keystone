package localstate

import (
	"fmt"
	"os"
	"path/filepath"
)

const defaultDataDirName = ".keystone"

// Paths 描述一个 Keystone 本机数据根及其固定子路径。
type Paths struct {
	Root          string
	StateDir      string
	DatabasePath  string
	ArtifactsDir  string
	WorkspacesDir string
	RuntimeDir    string
	LockPath      string
	MetadataPath  string
}

// Resolve 只解析路径，不创建目录，也不访问 Repository Manifest。
func Resolve(dataDir string) (Paths, error) {
	root, err := resolveRoot(dataDir)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve local state root: %w", err)
	}

	stateDir := filepath.Join(root, "state")
	runtimeDir := filepath.Join(root, "runtime")
	return Paths{
		Root:          root,
		StateDir:      stateDir,
		DatabasePath:  filepath.Join(stateDir, "keystone.db"),
		ArtifactsDir:  filepath.Join(root, "artifacts"),
		WorkspacesDir: filepath.Join(root, "workspaces"),
		RuntimeDir:    runtimeDir,
		LockPath:      filepath.Join(runtimeDir, "keystone.lock"),
		MetadataPath:  filepath.Join(runtimeDir, "instance.json"),
	}, nil
}

func resolveRoot(dataDir string) (string, error) {
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		dataDir = filepath.Join(home, defaultDataDirName)
	}

	root, err := filepath.Abs(filepath.Clean(dataDir))
	if err != nil {
		return "", fmt.Errorf("make absolute data root: %w", err)
	}
	return filepath.Clean(root), nil
}

// Initialize 创建本机状态目录，并使用 owner-only 权限。
func (p Paths) Initialize() error {
	for _, dir := range []string{p.Root, p.StateDir, p.ArtifactsDir, p.WorkspacesDir, p.RuntimeDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create local state directory %s: %w", dir, err)
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return fmt.Errorf("set local state directory mode %s: %w", dir, err)
		}
	}
	return nil
}
