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
- Qwen（2026-07-24）：走 DashScope 原生异步任务（Token Plan `{origin}/api/v1/services/aigc/...` + `X-DashScope-Async: enable`，查询 `/api/v1/tasks/{id}`）。origin 由账号 base_url 提取（剥掉 /compatible-mode/v1）。图片服务端轮询同步返回 OpenAI 格式；视频提交即返回 task_id，客户端查状态。请求体扁平 OpenAI-ish → DashScope {model,input,parameters}，含 input 对象的原生格式直接透传。**注意凯哥已把图片端点改为 `multimodal-generation/generation`（支持 wan2.7-image、size 别名 1K/2K/4K→像素）。**
- Ark（2026-07-30）：视频 `/contents/generations/tasks` 需 `content` 数组。`buildArkVideoTaskBody` 把扁平 {prompt,ratio,duration,...} 转成 content 数组（prompt→text 项，img_url/image_url→image_url 项），其余顶层字段原样保留（火山新方式就是顶层参数）；已有非空 content 透传。火山参数语义：顶层 ratio/duration/resolution/seed/watermark/camera_fixed/generate_audio，旧方式为 text 后缀 `--rs/--rt/--dur`。
- 分组"允许生图"开关链路：字段 `Group.AllowImageGeneration`，闸口 `GroupAllowsImageGeneration()`；创建默认值在 `admin_group.go` `defaultAllowImageGenerationForPlatform`（grok/ark/qwen 默认 true）；前端显示控制在 `groupsImagePricing.ts` 的 `imagePricingPlatforms`（图片区）和 `supportsVideoPricingPlatform`（视频区）两个集合。新媒体平台要开开关就改这三处。

## 部署打包标准流程（代码改完 → 上线）

- 前端 build：`pnpm --dir frontend run build` → 输出 `backend/internal/web/dist`。
- 后端编译**必须带** `-tags=embed`（否则 `embed_off.go` 的 `Middleware()` 直接对所有前端路由返回 `404 Frontend not embedded`）：`cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags=embed -ldflags="-s -w" -trimpath -o bin/server-linux-amd64 ./cmd/server`。
- 上线路径：scp 覆盖生产二进制 → `systemctl restart sub2api`（服务名 `sub2api`，env 在 `/etc/sub2api/sub2api.env`，数据目录 `/opt/sub2api/data`，二进制位置按实际部署目录）。
- 仅改后端 Go（前端 `dist` 未动）：跳过前端 build，直接重编后端即可。
- 改了 ent schema：`cd backend/ent && go generate .`；改了 wire 注入：`cd backend && go generate ./cmd/server`，再编译。
- 新增迁移 SQL 放 `backend/migrations/`，部署后应用启动自动跑，无需手动。
- ⚠️ 生产 `/etc/sub2api/sub2api.env` 的 `TOTP_ENCRYPTION_KEY` 必须保持与导出库时的本地原值一致，否则库里加密的 API key/TOTP 全解不开（2026-07-29 踩过）。重新部署**不要动 env 文件**。
- 验证：`curl -sS -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/health`（8080 是明文 HTTP，外层 HTTPS 由 openresty 提供，用 http 测）返回 200 即活。
