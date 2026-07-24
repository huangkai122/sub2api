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

## 媒体生成转发（images/videos）架构速查

- 对外端点：`POST /v1/images/generations|edits`、`POST /v1/videos/generations`、`GET /v1/videos/:request_id`，在 `backend/internal/server/routes/gateway.go` 按分组平台 switch 分发。
- 每个平台一对文件：`service/<platform>_media.go` + `handler/<platform>_media.go`（openai_images/grok_media/ark_media/qwen_media）。新平台照 ark 模板抄：handler 里调度/失败切换/池模式重试/分组图片权限/内容审核/计费全通用，只需改 PlatformXxx、ForwardXxxMedia、粘性 hash 前缀。
- Qwen（2026-07-24）：走 DashScope 原生异步任务（Token Plan `{origin}/api/v1/services/aigc/...` + `X-DashScope-Async: enable`，查询 `/api/v1/tasks/{id}`）。origin 由账号 base_url 提取（剥掉 /compatible-mode/v1）。图片服务端轮询同步返回 OpenAI 格式；视频提交即返回 task_id，客户端查状态。请求体扁平 OpenAI-ish → DashScope {model,input,parameters}，含 input 对象的原生格式直接透传。
- 分组"允许生图"开关链路：字段 `Group.AllowImageGeneration`，闸口 `GroupAllowsImageGeneration()`；创建默认值在 `admin_group.go` `defaultAllowImageGenerationForPlatform`（grok/ark/qwen 默认 true）；前端显示控制在 `groupsImagePricing.ts` 的 `imagePricingPlatforms`（图片区）和 `supportsVideoPricingPlatform`（视频区）两个集合。新媒体平台要开开关就改这三处。
