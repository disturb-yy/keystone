# 12-06：双平台 Review 与 Evidence 发布

> 状态：`implemented-unverified`（Review Packet 代码已落地，尚未形成独立审阅或 Evidence）。本次按用户授权偏离上游阻塞；父 Ticket：[12 Golden Path E2E Evidence](../../12-golden-path-e2e.md)；规格：[12-golden-path-e2e-spec.md](../spec/12-golden-path-e2e-spec.md)。
>
> 顶层硬阻塞：Ticket 11 的真实 acceptance evidence 未形成前不得开始实现；Linux 与 WSL 的独立可用环境也是本子票的外部执行前置。

**What to build：** 让每个 terminal GoldenPathRun 形成完整性可校验的脱敏 review packet，由独立审阅者给出绑定结论；只有 Linux 与 WSL 的同源真实 Codex PASS Run 均成立时，人工才可 append-only 发布可复盘的成功 Evidence。

**Blocked by：** Ticket 11 的真实 acceptance evidence；12-05 Candidate 服务与 Production Dashboard 观察；独立非 WSL Linux host/VM 与 WSL 的真实 Codex 可用环境。

**Status：** implemented-unverified（仅表示代码已落地，不表示 GoldenPathEvidence）

## Scope

- 为每个 terminal Run（包括 Codex 预检或中途失败）形成脱敏 `GoldenPathReviewPacket`，以规范 manifest 逐项固定材料 role、media type、byte length 与 SHA-256，并以 manifest SHA-256 作为 packet digest。
- 让 packet 绑定 `GoldenPathRunID`、`GoldenPathEvidenceSetID`、终态、最后 checkpoint、失败分类（如有）和 exit code；成功 packet 还固定浏览器、Dashboard、服务、命令、Query、revision、Trace 和安全摘要。
- 要求独立于 Run 执行者/Runner 的只读审阅者生成 UUIDv7 `GoldenPathReviewID`，绑定审阅角色、独立性声明、RunID、EvidenceSetID、packet digest 和 PASS/FAIL/UNVERIFIED 结论。
- Runner 永不创建或改写成功 Evidence。仅在 PASS 后由人工为该平台追加成功记录，绑定 EvidenceSetID、ReviewID、packet digest 与三条证据链；撤销或替代只能追加引用原 RunID 的新记录。
- 分别在非 WSL Linux host/VM 与 WSL 运行真实 Codex GoldenPathRun，核对二者同一 EvidenceSetID、独立 RunID/平台来源与完整成功条件，才允许宣布 V1 Thin Vertical Slice 完成。
- 更新受影响导航文档，只描述已经真实运行和审阅的事实；未运行、FAIL、UNVERIFIED、Codex 不可用或平台来源不明都不能伪装为成功。

## Authority and Safety Boundaries

- `GoldenPathReview` 不是 Runner 自检、Runtime outcome、Daemon readiness 或人工口头确认；审阅者不得修改 Daemon、fixture、candidate Repository、Workspace 或 LocalStateRoot。
- `GoldenPathEvidence` 是 PASS 后的人工发布记录，不是原始日志仓库。Markdown 只保存安全摘要、Artifact identity 与 digest，不保存 token、认证原文、绝对路径、hostname、用户名、IP、machine ID、完整环境或敏感 Prompt。
- `GoldenPathPlatformProvenance` 是可复核来源声明而非密码学远程证明。WSL 必须有 WSL marker；Linux 不得有 WSL/container marker 且必须带 host/VM 声明；任何歧义都只能得到 `platform_provenance_unverified`。
- 失败或未执行 Run 只能留下受控根中的脱敏失败摘要；独立只读复核完成前不得自动 reset、clean、amend、rebase、删除 Worktree 或清理现场。

## Acceptance Criteria

- [ ] 每个 terminal Run 都有规范、完整性可校验的 review packet；安全材料与 manifest digest 能复核，且失败 packet 不遗漏终态阶段、最后 checkpoint、失败分类和 exit code。
- [ ] 独立审阅生成不可移用的 ReviewID 绑定，并且 FAIL/UNVERIFIED 从不触发成功 Evidence 发布。
- [ ] 成功 Evidence 只能由人工 append-only 创建，包含三条证据链、平台、RunID、EvidenceSetID、ReviewID、packet digest、版本/digest、命令/API/退出结果、关键 Daemon identities/revisions 与脱敏说明；既有成功记录不被静默改写。
- [ ] Linux 与 WSL 都有独立真实 Codex PASS Run，二者 EvidenceSetID 相同、RunID 不同、平台来源符合要求；交叉编译、WSL container、单个平台或复用同一 Run 均不能宣布 V1 完成。
- [ ] 若任一平台、审阅或完整条件失败，成功 Evidence 不创建；现场仅在独立只读复核后由人工处理。

## Out of Scope

- 修改 Runner、fixture、Control Plane、Worker、Dashboard 或上游 Ticket 09–11 的业务行为来让审阅通过。
- 远程签名/远程证明、团队审批系统、自动 Markdown 发布、自动清理、merge、push、PR、deploy 或将原生 Windows 添加为 Ticket 12 第三平台。

## Verification

- 对成功与失败 terminal Run 验证 packet manifest、digest、脱敏、ReviewID 绑定与 append-only 发布规则。
- 在两个独立目标平台执行真实 Codex 完整链路并由独立审阅者复核；只记录实际 PASS/FAIL/UNVERIFIED，不能用文档、构建或 fake seam 代替。

