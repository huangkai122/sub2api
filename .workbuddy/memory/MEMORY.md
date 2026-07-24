# MEMORY.md - sub2api 项目长期记忆

## 环境

- Go 1.26.5：`/e/Program Files/Go/bin/go.exe`（git bash PATH 里没有，用绝对路径或 `PATH="/e/Program Files/Go/bin:$PATH"`）。
- 测试：后端 `go test -tags=unit ./...`（integration 需 DB）；前端 `npx vitest run`、`npx vue-tsc --noEmit`。
- ent 重生成：`cd backend/ent && go generate .`。
- 凯哥自己频繁 commit，看到未提交改动被收走是常态；我做完留工作区即可，不代 commit。

## 已知历史遗留失败（与改动无关，别慌）

- `backend/internal/config`：`TestLoadForBootstrapAllowsMissingJWTSecret`、`TestLoadDefaultDatabaseSSLMode` 在基线提交就失败（2026-07-24 用 worktree 验证过）。

## 渠道监控（channel monitor）架构速查

- provider 唯一来源 = `backend/internal/service/channel_monitor_checker.go` 的 `providerAdapters` 表（adapter = buildPath/buildBody/buildHeaders/textPath/pathSuffix）。
- 加 provider 全链路：ent schema 两表枚举（channel_monitor.go + channel_monitor_request_template.go）→ `go generate ./ent` → 迁移重建 CHECK 约束（下一个迁移号看 backend/migrations/ 最大值，175 已用）→ checker 注册 adapter + merge 黑名单 → const 常量 + 错误文案 → admin 两个 handler 的 oneof → 前端 Provider 类型/constants/useChannelMonitorFormat/ProviderIcon/表单/过滤栏/模板弹窗/i18n。
- endpoint 语义（2026-07-24 起）：只填 origin 用平台默认路径；自带 base path 只追加协议后缀（/chat/completions 等）；已带完整后缀则原样用。禁 query/fragment，强制 https + SSRF 校验。
- qwen/mimo/ark 均为 OpenAI 兼容协议，adapter 用 `openAICompatibleChatAdapter(defaultPath)` 复用。默认路径：qwen=/compatible-mode/v1，mimo=/v1，ark=/api/v3。
- 前端平台配色约定：openai=emerald, anthropic=orange, gemini=sky, qwen=violet, mimo=rose, ark=cyan。

## 账号池要点（2026-07-23）

- 界面"池模式(pool_mode)" ≠ 账号池，它是 OpenAI/APIKey 账号的上游错误重试开关。
- 调度 = priority + load_factor + concurrency + schedulable + 全局开关 + 分组 model_routing + 粘性会话，无单一下拉。
- 手册在 `docs/ACCOUNT_POOL_ADMIN_GUIDE_CN.md`。
