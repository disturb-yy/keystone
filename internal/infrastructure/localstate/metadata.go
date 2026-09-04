package localstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// Metadata 是供诊断使用的单实例运行记录，不是锁权威。
type Metadata struct {
	PID        int    `json:"pid"`
	Endpoint   string `json:"endpoint"`
	InstanceID string `json:"instance_id"`
	StartedAt  string `json:"started_at"`
}

// PublishMetadata 原子发布单个运行元数据记录。
//
// 调用方应在成功获取 p 对应的 InstanceLock 后调用本方法；元数据只用于
// 诊断，不参与锁所有权判断。
func PublishMetadata(p Paths, metadata Metadata) (publishErr error) {
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode instance metadata: %w", err)
	}
	temporary, err := os.CreateTemp(p.RuntimeDir, ".instance-*.tmp")
	if err != nil {
		return fmt.Errorf("create instance metadata temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	temporaryClosed := false
	defer func() {
		var cleanupErr error
		if !temporaryClosed {
			if err := temporary.Close(); err != nil {
				cleanupErr = fmt.Errorf("close instance metadata temporary file during cleanup: %w", err)
			}
		}
		if err := os.Remove(temporaryPath); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove instance metadata temporary file during cleanup: %w", err))
		}
		if cleanupErr != nil {
			publishErr = errors.Join(publishErr, cleanupErr)
		}
	}()
	if err := temporary.Chmod(0600); err != nil {
		return fmt.Errorf("set instance metadata mode: %w", err)
	}
	payload := append(data, '\n')
	if written, err := temporary.Write(payload); err != nil {
		return fmt.Errorf("write instance metadata: %w", err)
	} else if written != len(payload) {
		return fmt.Errorf("write instance metadata: %w", io.ErrShortWrite)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync instance metadata: %w", err)
	}
	if err := temporary.Close(); err != nil {
		temporaryClosed = true
		return fmt.Errorf("close instance metadata temporary file: %w", err)
	}
	temporaryClosed = true
	if err := replaceFile(temporaryPath, p.MetadataPath); err != nil {
		return fmt.Errorf("publish instance metadata: %w", err)
	}
	return nil
}

// ClearMetadata 仅清理 instanceID 对应的当前记录。
func ClearMetadata(p Paths, instanceID string) error {
	data, err := os.ReadFile(p.MetadataPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read instance metadata: %w", err)
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("decode instance metadata: %w", err)
	}
	if metadata.InstanceID != instanceID {
		return nil
	}
	if err := os.Remove(p.MetadataPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove instance metadata: %w", err)
	}
	return nil
}
