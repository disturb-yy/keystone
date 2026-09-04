# 本机 Daemon 的发现与控制边界

M1 由 `keystone` 启动独立的 `keystone-daemon`，并以经归一化的 LocalStateRoot 作为实例边界。Daemon 在 `127.0.0.1:0` 监听；Booting 时 `GET /healthz` 返回 `503 {"ready":false}`，只有完成 InstanceLock、持久状态可用和 Migration 后才发布 RuntimeMetadata，并以 `200 {"ready":true}` 进入 DaemonReadiness。这样避免固定端口冲突，也不把 PID 或 metadata 误当成锁权威。

Client 只可通过 DaemonEndpoint 的 HTTP/JSON Query 确认状态与 SchemaMigrationVersion，不直接访问 SQLite：`GET /v1/daemon/status` 返回权威状态，`POST /v1/daemon/stop` 以 instance ID 防止陈旧 metadata 误停实例，并异步优雅关停。M1 采用本机 loopback 信任域，instance ID 不作为安全凭据，绝不按 PID 杀进程；跨用户隔离需另行引入 control credential。

`keystone daemon start` 是唯一可 ensure 实例的命令，必须在有界等待内确认目标实例已健康后才成功返回；`status` 和 `stop` 从不隐式启动实例。metadata 缺失、endpoint 不可达或 instance ID 不匹配均为明确失败且不触碰现有本机状态；同一实例已接受的重复 stop 可再次返回已接受，不要求 `Idempotency-Key`。

DaemonReadiness 表达当前可服务性而非一次性的启动里程碑。持久状态探测失败时，DaemonReadinessEndpoint 降为 not ready，status 返回服务不可用，但已存在实例的 stop 通道仍可用于优雅退出。
