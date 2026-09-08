# 10-01：Manifest V2、策略快照与验证领域骨架

**What to build：** 在不改写既有 V1 Manifest 的前提下，建立严格的 ProjectManifest V2 parser、VerificationPolicySnapshot、VerificationOutcome/证据领域类型和最小持久化骨架，使后续 Verify 永远从 Change 的 BaseRevision 使用固定策略。

**Blocked by：** 顶层 Ticket 09 的真实实现与验收证据；开始前还须核对 `internal/infrastructure/manifest`、Change Execute use case、Workstore Migration 及其局部规约的当前 API。顶层 Ticket 或本子票的规划状态不解除该阻塞。

**Status：** ready-for-agent（仅文档成熟度）

## Scope

- 扩展 `internal/infrastructure/manifest` 的严格 parser，使其支持 V1 身份读取和 V2 `version`、`project_id`、`verify.commands`、可选 `commit.template` 的明确分支；拒绝未知字段、重复 key、alias/custom tag、多 document、非法 UTF-8、无效边界和隐式默认命令。
- 为 V2 实现保留命令顺序的规范化 VerificationConfigurationDigest 与独立 CommitTemplateDigest；验证 YAML 注释或格式变化不改变 digest，而任何语义变化都会改变对应 digest。
- 在适当的 `internal/governance/domain` 与 Application/Port 中定义 VerificationPolicySnapshot、VerificationOutcome、VerificationCommandResult、AcceptanceCriterionRef/Result、VerificationIntent 的最小不变量和稳定错误分类。新建 package 必须同步建立局部 `AGENTS.md` 与 `INDEX.md`。
- 在 ExecuteCommand 被接受时仅从 BaseRevision 读取并持久化 V2 策略快照；候选 Workspace 的 Manifest 不能覆盖它。V1、缺失或无效 V2 不被静默升级，后续 Verify 以可审计配置不足进入 `human_required`。
- 以实现时下一个可用 additive Migration 为策略快照及后续 VerificationIntent 的最小关联建立不可变/唯一性基础，保留既有 Event、Artifact、AgentRun、Ticket Graph、Lease 和 Migration checksum。
- 定义 WorkspaceInputRevision 与 CandidateTreeIdentity 的使用 seam：首张 Ticket 固定 BaseRevision，后续 Ticket 使用前一 KeystoneCommit after revision；本子票不实现 Git Commit。

## Acceptance

- V1 可继续被既有 Project 身份流程读取；V2 只在全部严格规则满足时被接受，Daemon/CLI/init 都不会自动改写任一版本。
- 1 至 32 条有序命令、name/argv/timeout、模板 placeholder 和所有大小上限都有确定性 unit test；重复 key、未知字段、空参数、超限、重排与非法 template 都被拒绝。
- 相同 V2 语义的不同 YAML 空白/注释得到相同 digest；命令/参数/timeout/顺序或模板语义变化得到对应不同 digest。
- 已接受 Execute 的 VerificationPolicySnapshot 可追溯到 Change/BaseRevision，候选 `.keystone/project.yaml` 的修改不会改变其命令或 template。
- V1/无效策略不能形成假的快照、Worker Assignment 或 PASS；Verify 路径可稳定进入 `human_required` 并保留安全原因。
- Migration 升级保留既有权威记录，策略快照不可被后续更新覆盖。

## Out of scope

- `worker/v1` 的新 DTO、VerificationExecutor、VerifierAgentRun、Verify HTTP/CLI、命令实际运行或 Criterion 审查（10-02）。
- Git Commit、CommitIntent、Gate 收口、下一张 Ticket 调度和 Final Verify（10-03/10-04）。
- Dashboard、真实 Codex smoke、Manifest V1 自动升级或任意配置编辑 UI。

## Verification

```bash
go test ./internal/infrastructure/manifest/...
go test ./internal/governance/...
go test ./internal/infrastructure/workstore/...
go test ./...
go vet ./...
make build
git diff --check
```
