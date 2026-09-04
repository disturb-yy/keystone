package main

import (
	"context"
	"errors"

	"github.com/disturb-yy/keystone/contracts/controlplane"
	"github.com/disturb-yy/keystone/internal/infrastructure/localstate"
)

func (c *cli) verifyReady(ctx context.Context, metadata localstate.Metadata) (controlplane.DaemonStatusResponse, error) {
	health, err := c.client().health(ctx, metadata.Endpoint)
	if err != nil {
		return controlplane.DaemonStatusResponse{}, err
	}
	if !health.Ready {
		return controlplane.DaemonStatusResponse{}, newCLIError(ErrorHealthNotReady, "Daemon 健康检查未 ready", nil)
	}
	status, err := c.client().status(ctx, metadata.Endpoint)
	if err != nil {
		return controlplane.DaemonStatusResponse{}, err
	}
	if err := validateReadyStatus(metadata, status); err != nil {
		return controlplane.DaemonStatusResponse{}, err
	}
	return status, nil
}

func validateReadyStatus(metadata localstate.Metadata, status controlplane.DaemonStatusResponse) error {
	if !status.DaemonReadiness {
		return newCLIError(ErrorHealthNotReady, "Daemon status 表明当前未 ready", nil)
	}
	if status.DaemonInstanceID == "" || status.DaemonInstanceID != metadata.InstanceID {
		return newCLIError(ErrorInstanceMismatch, "Daemon status 的 DaemonInstanceID 与 metadata 不一致", nil)
	}
	if status.DatabasePath == "" || status.SchemaMigrationVersion < 0 {
		return newCLIError(ErrorInvalidResponse, "Daemon status 字段无效", errors.New("status contains invalid database or migration fields"))
	}
	return nil
}
