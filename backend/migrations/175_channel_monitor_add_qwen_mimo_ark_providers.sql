-- Migration: 175_channel_monitor_add_qwen_mimo_ark_providers
-- 渠道监控 provider 扩展：openai/anthropic/gemini 之外新增 qwen / mimo / ark。
-- 三个平台均为 OpenAI 兼容协议（Bearer + choices.0.message.content），
-- 仅默认请求路径不同（见 service 层 providerAdapters）。
--
-- 实现方式：重建两张表的 provider CHECK 约束，纳入新枚举值。
-- DROP IF EXISTS + ADD 的组合可重复执行，天然幂等。
-- 既有行不受约束变更影响（原有枚举值全部保留）。

ALTER TABLE channel_monitors
    DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;
ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'qwen', 'mimo', 'ark'));

ALTER TABLE channel_monitor_request_templates
    DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;
ALTER TABLE channel_monitor_request_templates
    ADD CONSTRAINT channel_monitor_request_templates_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'qwen', 'mimo', 'ark'));
