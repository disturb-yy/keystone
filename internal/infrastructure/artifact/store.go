// Package artifact 提供 Change Artifact 内容的本机原子存储。
package artifact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/disturb-yy/keystone/internal/work/domain"
)

// Store 将 Artifact 内容保存到调用方提供的 LocalStateRoot 子目录。
type Store struct {
	root string
}

// New 创建 Artifact 内容适配器并确保其目录存在。
func New(root string) (*Store, error) {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, fmt.Errorf("create artifact store: %w", domain.ErrInvalidRequest)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fmt.Errorf("create artifact directory: %w", domain.ErrUnavailable)
	}
	if err := os.Chmod(root, 0700); err != nil {
		return nil, fmt.Errorf("set artifact directory mode: %w", domain.ErrUnavailable)
	}
	return &Store{root: root}, nil
}

// Put 原子写入内容；同一摘要和长度的已存在内容会被校验后复用。
func (s *Store) Put(ctx context.Context, content []byte) (domain.ArtifactIdentity, error) {
	if err := validateStoreContext(ctx, s); err != nil {
		return domain.ArtifactIdentity{}, err
	}
	identity := domain.NewArtifactIdentity(content)
	path := s.path(identity)
	if existing, err := os.ReadFile(path); err == nil {
		if domain.NewArtifactIdentity(existing) != identity {
			return domain.ArtifactIdentity{}, fmt.Errorf("existing artifact does not match identity: %w", domain.ErrArtifactUnavailable)
		}
		return identity, nil
	} else if !os.IsNotExist(err) {
		return domain.ArtifactIdentity{}, fmt.Errorf("read existing artifact: %w", domain.ErrUnavailable)
	}

	temporary, err := os.CreateTemp(s.root, ".artifact-*.tmp")
	if err != nil {
		return domain.ArtifactIdentity{}, fmt.Errorf("create artifact temporary file: %w", domain.ErrUnavailable)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0600); err != nil {
		return domain.ArtifactIdentity{}, fmt.Errorf("set artifact mode: %w", domain.ErrUnavailable)
	}
	if written, err := temporary.Write(content); err != nil {
		return domain.ArtifactIdentity{}, fmt.Errorf("write artifact: %w", domain.ErrUnavailable)
	} else if written != len(content) {
		return domain.ArtifactIdentity{}, fmt.Errorf("write artifact: %w", domain.ErrUnavailable)
	}
	if err := temporary.Sync(); err != nil {
		return domain.ArtifactIdentity{}, fmt.Errorf("sync artifact: %w", domain.ErrUnavailable)
	}
	if err := temporary.Close(); err != nil {
		closed = true
		return domain.ArtifactIdentity{}, fmt.Errorf("close artifact: %w", domain.ErrUnavailable)
	}
	closed = true
	if err := os.Rename(temporaryPath, path); err != nil {
		if existing, readErr := os.ReadFile(path); readErr == nil && domain.NewArtifactIdentity(existing) == identity {
			return identity, nil
		}
		return domain.ArtifactIdentity{}, fmt.Errorf("publish artifact: %w", domain.ErrUnavailable)
	}
	return identity, nil
}

// Read 读取内容并重新计算摘要和长度。
func (s *Store) Read(ctx context.Context, identity domain.ArtifactIdentity) ([]byte, error) {
	if err := validateStoreContext(ctx, s); err != nil {
		return nil, err
	}
	if err := identity.Validate(); err != nil {
		return nil, fmt.Errorf("read artifact identity: %w", domain.ErrArtifactUnavailable)
	}
	content, err := os.ReadFile(s.path(identity))
	if err != nil {
		return nil, fmt.Errorf("read artifact content: %w", domain.ErrArtifactUnavailable)
	}
	if domain.NewArtifactIdentity(content) != identity {
		return nil, fmt.Errorf("artifact content checksum mismatch: %w", domain.ErrArtifactUnavailable)
	}
	return content, nil
}

func (s *Store) path(identity domain.ArtifactIdentity) string {
	return filepath.Join(s.root, identity.SHA256)
}

func validateStoreContext(ctx context.Context, store *Store) error {
	if ctx == nil || store == nil || store.root == "" {
		return fmt.Errorf("artifact store: %w", domain.ErrInvalidRequest)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("artifact store: %w", err)
	}
	return nil
}
