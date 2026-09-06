package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/disturb-yy/keystone/contracts/controlplane"
)

func (c *cli) newChangeCommand() *cobra.Command {
	change := &cobra.Command{
		Use:   "change",
		Short: "管理 Change 生命周期",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	change.AddCommand(c.newChangeCreateCommand())
	change.AddCommand(c.newChangeListCommand())
	change.AddCommand(c.newChangeShowCommand())
	change.AddCommand(c.newChangeControlCommand("pause"))
	change.AddCommand(c.newChangeControlCommand("resume"))
	change.AddCommand(c.newChangeControlCommand("cancel"))
	change.AddCommand(c.newChangeDecisionCommand())
	return change
}

func (c *cli) newChangeCreateCommand() *cobra.Command {
	var repositoryPath, intent, key string
	command := &cobra.Command{
		Use:   "create",
		Short: "创建 Change",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.changeCreate(cmd.Context(), cmd.OutOrStdout(), repositoryPath, intent, key)
		},
	}
	command.Flags().StringVar(&repositoryPath, "repository-path", "", "已注册 Project 的 Repository 路径")
	command.Flags().StringVar(&intent, "intent", "", "Change Intent 原文")
	command.Flags().StringVar(&key, "idempotency-key", "", "幂等键")
	return command
}

func (c *cli) newChangeListCommand() *cobra.Command {
	var repositoryPath string
	command := &cobra.Command{
		Use:   "list",
		Short: "列出 Change",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.changeList(cmd.Context(), cmd.OutOrStdout(), repositoryPath)
		},
	}
	command.Flags().StringVar(&repositoryPath, "repository-path", "", "已注册 Project 的 Repository 路径")
	return command
}

func (c *cli) newChangeShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show CHANGE_ID",
		Short: "查看 Change",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.changeShow(cmd.Context(), cmd.OutOrStdout(), args[0])
		},
	}
}

func (c *cli) newChangeControlCommand(operation string) *cobra.Command {
	var key string
	var version int
	command := &cobra.Command{
		Use:   operation + " CHANGE_ID",
		Short: operation + " Change",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.changeControl(cmd.Context(), cmd.OutOrStdout(), args[0], operation, version, key)
		},
	}
	command.Flags().IntVar(&version, "expected-version", 0, "观察到的 Change version")
	command.Flags().StringVar(&key, "idempotency-key", "", "幂等键")
	return command
}

func (c *cli) newChangeDecisionCommand() *cobra.Command {
	decision := &cobra.Command{
		Use:   "decide",
		Short: "提交 HumanDecision",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	decision.AddCommand(c.newChangeDecisionLeaf("retry"), c.newChangeDecisionLeaf("cancel"))
	return decision
}

func (c *cli) newChangeDecisionLeaf(kind string) *cobra.Command {
	var key, reason string
	var version int
	command := &cobra.Command{
		Use:   kind + " CHANGE_ID",
		Short: "提交 " + kind + " HumanDecision",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.changeDecision(cmd.Context(), cmd.OutOrStdout(), args[0], kind, version, key, reason)
		},
	}
	command.Flags().IntVar(&version, "expected-version", 0, "观察到的 Change version")
	command.Flags().StringVar(&key, "idempotency-key", "", "幂等键")
	command.Flags().StringVar(&reason, "reason", "", "人工决定原因")
	return command
}

func (c *cli) changeCreate(ctx context.Context, out io.Writer, repositoryPath, intent, key string) error {
	if key == "" || strings.TrimSpace(intent) == "" {
		return newCLIError(ErrorChangeFailed, "change create 需要 intent 和 idempotency-key", nil)
	}
	path, err := absoluteCLIPath(repositoryPath)
	if err != nil {
		return newCLIError(ErrorChangeFailed, "repository-path 无效", err)
	}
	metadata, err := c.ensureDaemon(ctx)
	if err != nil {
		return err
	}
	response, err := c.client().changeCreate(ctx, metadata.Endpoint, key, controlplane.ChangeCreateRequest{RepositoryPath: path, Intent: intent})
	if err != nil {
		return err
	}
	return writeJSONOutput(out, response)
}

func (c *cli) changeList(ctx context.Context, out io.Writer, repositoryPath string) error {
	path, err := absoluteCLIPath(repositoryPath)
	if err != nil {
		return newCLIError(ErrorChangeFailed, "repository-path 无效", err)
	}
	metadata, err := readRuntimeMetadata(c.paths)
	if err != nil {
		return err
	}
	response, err := c.client().changeList(ctx, metadata.Endpoint, path)
	if err != nil {
		return err
	}
	return writeJSONOutput(out, response)
}

func (c *cli) changeShow(ctx context.Context, out io.Writer, changeID string) error {
	metadata, err := readRuntimeMetadata(c.paths)
	if err != nil {
		return err
	}
	response, err := c.client().changeShow(ctx, metadata.Endpoint, changeID)
	if err != nil {
		return err
	}
	return writeJSONOutput(out, response)
}

func (c *cli) changeControl(ctx context.Context, out io.Writer, changeID, operation string, version int, key string) error {
	if version < 1 || key == "" {
		return newCLIError(ErrorChangeFailed, "change control 需要 expected-version 和 idempotency-key", nil)
	}
	metadata, err := c.ensureDaemon(ctx)
	if err != nil {
		return err
	}
	response, err := c.client().changeCommand(ctx, metadata.Endpoint, key, changeID, controlplane.ChangeCommandRequest{Command: operation, ExpectedVersion: version})
	if err != nil {
		return err
	}
	return writeJSONOutput(out, response)
}

func (c *cli) changeDecision(ctx context.Context, out io.Writer, changeID, decision string, version int, key, reason string) error {
	if version < 1 || key == "" {
		return newCLIError(ErrorChangeFailed, "change decision 需要 expected-version 和 idempotency-key", nil)
	}
	metadata, err := c.ensureDaemon(ctx)
	if err != nil {
		return err
	}
	response, err := c.client().changeDecision(ctx, metadata.Endpoint, key, changeID, controlplane.HumanDecisionRequest{Decision: decision, ExpectedVersion: version, Reason: reason})
	if err != nil {
		return err
	}
	return writeJSONOutput(out, response)
}

func absoluteCLIPath(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("path is required")
	}
	path, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", err
	}
	return path, nil
}
