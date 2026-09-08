# ADR-0037：ProjectManifest V2 承载确定性验证配置

## 状态

已接受。

## 决策

Ticket 10 将 `verify.commands` 作为 ProjectManifest V2 的严格、版本化配置。V2 的可解释结构固定为：

```yaml
version: 2
project_id: <uuidv7>
verify:
  commands:
    - name: go-test
      argv: ["go", "test", "./..."]
      timeout_seconds: 900
commit:
  template: "{ticket_title}"
```

`verify.commands` 必填，按声明顺序包含 1 至 32 条名称唯一的命令；每条命令只能包含 `name`、非空 `argv` 与 1 至 1800 秒的 `timeout_seconds`。`commit` 可省略；存在时只可包含 `template`，且模板只能引用 `{change_id}`、`{ticket_id}` 与 `{ticket_title}`。V2 拒绝未知字段、重复键、空参数元素和任何隐式默认命令。参数数组不经 shell 解释，Manifest 不提供自由 cwd、环境变量或忽略失败开关。

严格 parser 分别对 `verify.commands` 与 `commit.template` 形成保留命令顺序的规范化语义 digest；YAML 空白和注释不改变 digest。前者固定验证证据和 Verify/FinalVerify 请求身份，后者只固定 CommitIntent 的消息输入，避免提交模板变化使既有验证证据失效。V1 Manifest 仍可用于既有 Project 身份协调，但不能提供自动 Verify 所需的确定性配置；Verify 遇到 V1 时进入 `human_required`，等待 Repository 手工、版本化地升级。Daemon 与 `keystone init` 不得静默改写 V1 或 V2 Manifest。

Daemon 在接受 ExecuteCommand 时必须从 BaseRevision 读取并解析 Manifest，持久化 VerificationPolicySnapshot。Ticket Verify、Commit 和 Final Verify 只使用这个快照，绝不读取候选 Workspace 当前的 Manifest；因此 Change 内对 `.keystone/project.yaml` 的修改只会影响后续从新 revision 创建的 Change。

## 理由与边界

验证命令属于 Repository 版本化知识，而不是本机 Daemon、Client 临时参数或 Runtime 自报。以 V2 显式演进保留 V1 严格边界和可审计升级路径；语义 digest 让重新格式化 YAML 不会伪造配置变更，参数数组使实际执行输入可复盘，避免未记录的 shell 拼接语义。该 ADR 定义 Ticket 10 的目标配置契约，不证明 V2 parser、升级流程或 Verify runtime 已实现。
