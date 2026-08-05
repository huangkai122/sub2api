<template>
  <AppLayout>
    <div class="mx-auto w-full max-w-6xl space-y-6 pb-8">
      <section class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="border-b border-gray-100 bg-gradient-to-r from-primary-50 to-white px-6 py-6 dark:border-dark-700 dark:from-primary-950/30 dark:to-dark-800 md:px-8">
          <p class="mb-2 text-sm font-medium text-primary-600 dark:text-primary-400">SharkSay API</p>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">大模型 API 快速调用</h1>
          <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-600 dark:text-dark-300">
            平台兼容 OpenAI 调用格式。准备一个对应平台分组的 API 密钥，选择模型，复制示例即可调用。
          </p>
        </div>

        <div class="grid gap-3 px-6 py-5 md:grid-cols-3 md:px-8">
          <div class="quick-step">
            <span class="step-number">1</span>
            <div>
              <p class="step-title">创建 API 密钥</p>
              <router-link to="/keys" class="step-link">前往 API 密钥</router-link>
            </div>
          </div>
          <div class="quick-step">
            <span class="step-number">2</span>
            <div>
              <p class="step-title">确认模型名称</p>
              <router-link to="/available-channels" class="step-link">查看可用渠道</router-link>
            </div>
          </div>
          <div class="quick-step">
            <span class="step-number">3</span>
            <div>
              <p class="step-title">替换示例变量</p>
              <p class="text-xs text-gray-500 dark:text-dark-400">替换 YOUR_API_KEY 和模型名称</p>
            </div>
          </div>
        </div>
      </section>

      <section class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800 md:p-6">
        <div class="flex flex-wrap gap-2" role="tablist" aria-label="模型平台">
          <button
            v-for="platform in platforms"
            :key="platform.id"
            type="button"
            class="platform-tab"
            :class="activePlatformId === platform.id ? platform.activeClass : ''"
            :aria-selected="activePlatformId === platform.id"
            role="tab"
            @click="selectPlatform(platform.id)"
          >
            <span class="h-2.5 w-2.5 rounded-full" :class="platform.dotClass"></span>
            {{ platform.name }}
          </button>
        </div>

        <div class="mt-6 border-t border-gray-100 pt-6 dark:border-dark-700">
          <div class="flex flex-col gap-1 md:flex-row md:items-end md:justify-between">
            <div>
              <p class="text-sm font-medium" :class="activePlatform.textClass">{{ activePlatform.vendor }}</p>
              <h2 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">{{ activePlatform.name }}</h2>
            </div>
            <p class="text-xs text-gray-500 dark:text-dark-400">API 地址：{{ apiBase }}</p>
          </div>

          <div class="mt-5 flex gap-2 overflow-x-auto border-b border-gray-200 dark:border-dark-600" role="tablist" aria-label="调用能力">
            <button
              v-for="capability in activePlatform.capabilities"
              :key="capability.id"
              type="button"
              class="capability-tab"
              :class="activeCapabilityId === capability.id ? 'capability-tab-active' : ''"
              :aria-selected="activeCapabilityId === capability.id"
              role="tab"
              @click="activeCapabilityId = capability.id"
            >
              {{ capability.name }}
            </button>
          </div>

          <div class="mt-5 grid gap-5 lg:grid-cols-[minmax(0,1fr)_260px]">
            <div class="min-w-0">
              <div class="mb-3 flex items-start justify-between gap-4">
                <div>
                  <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ activeCapability.name }}</h3>
                  <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ activeCapability.description }}</p>
                </div>
                <span class="method-badge">{{ activeCapability.method }}</span>
              </div>

              <div class="code-shell">
                <div class="code-header">
                  <span>cURL</span>
                  <button type="button" class="copy-button" @click="copyCode(activeCapability.code)">
                    {{ copied ? '已复制' : '复制代码' }}
                  </button>
                </div>
                <pre><code>{{ activeCapability.code }}</code></pre>
              </div>

              <div v-if="activeCapability.statusCode" class="mt-4">
                <p class="mb-2 text-sm font-medium text-gray-700 dark:text-dark-200">查询视频结果</p>
                <div class="code-shell">
                  <div class="code-header">
                    <span>cURL</span>
                    <button type="button" class="copy-button" @click="copyCode(activeCapability.statusCode)">
                      复制代码
                    </button>
                  </div>
                  <pre><code>{{ activeCapability.statusCode }}</code></pre>
                </div>
              </div>
            </div>

            <aside class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-900/50">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">调用说明</h3>
              <dl class="mt-4 space-y-4 text-sm">
                <div>
                  <dt class="text-xs text-gray-500 dark:text-dark-400">接口</dt>
                  <dd class="mt-1 break-all font-mono text-xs text-gray-800 dark:text-dark-100">{{ activeCapability.endpoint }}</dd>
                </div>
                <div>
                  <dt class="text-xs text-gray-500 dark:text-dark-400">推荐模型</dt>
                  <dd class="mt-1 font-mono text-xs text-gray-800 dark:text-dark-100">{{ activeCapability.model }}</dd>
                </div>
                <div v-if="activeCapability.note">
                  <dt class="text-xs text-gray-500 dark:text-dark-400">注意</dt>
                  <dd class="mt-1 leading-5 text-gray-700 dark:text-dark-200">{{ activeCapability.note }}</dd>
                </div>
              </dl>
            </aside>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'

type PlatformId = 'qwen' | 'mimo' | 'ark'
type CapabilityId = 'text' | 'image' | 'text-video' | 'image-video'

interface Capability {
  id: CapabilityId
  name: string
  description: string
  method: 'POST'
  endpoint: string
  model: string
  note?: string
  code: string
  statusCode?: string
}

interface PlatformDocs {
  id: PlatformId
  name: string
  vendor: string
  activeClass: string
  dotClass: string
  textClass: string
  capabilities: Capability[]
}

const appStore = useAppStore()
const activePlatformId = ref<PlatformId>('qwen')
const activeCapabilityId = ref<CapabilityId>('text')
const copied = ref(false)
let copiedTimer: ReturnType<typeof setTimeout> | undefined

const apiBase = computed(() => {
  const configured = appStore.apiBaseUrl.trim().replace(/\/$/, '')
  if (configured) return configured
  return typeof window !== 'undefined' ? window.location.origin : ''
})

function curl(endpoint: string, body: Record<string, unknown>): string {
  const json = JSON.stringify(body, null, 2)
    .split('\n')
    .map((line) => `  ${line}`)
    .join('\n')
  return `curl ${apiBase.value}${endpoint} \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '${json.trimStart()}'`
}

function videoStatus(): string {
  return `curl ${apiBase.value}/v1/videos/REQUEST_ID \\\n  -H "Authorization: Bearer YOUR_API_KEY"`
}

const platforms = computed<PlatformDocs[]>(() => [
  {
    id: 'qwen',
    name: '百炼 Qwen',
    vendor: '阿里云百炼',
    activeClass: 'border-violet-500 bg-violet-50 text-violet-700 dark:border-violet-500 dark:bg-violet-950/30 dark:text-violet-300',
    dotClass: 'bg-violet-500',
    textClass: 'text-violet-600 dark:text-violet-400',
    capabilities: [
      {
        id: 'text',
        name: '文本生成',
        description: '发送对话消息，获取模型回复。',
        method: 'POST',
        endpoint: '/v1/chat/completions',
        model: 'qwen-plus',
        code: curl('/v1/chat/completions', {
          model: 'qwen-plus',
          messages: [{ role: 'user', content: '用一句话介绍太原' }],
        }),
      },
      {
        id: 'image',
        name: '图片生成',
        description: '根据文字描述生成图片，成功后直接返回图片地址。',
        method: 'POST',
        endpoint: '/v1/images/generations',
        model: 'wan2.7-image',
        note: 'size 支持 1K、2K、4K 或具体像素尺寸。',
        code: curl('/v1/images/generations', {
          model: 'wan2.7-image',
          prompt: '一只戴墨镜的柴犬，电影海报风格',
          size: '2K',
        }),
      },
      {
        id: 'text-video',
        name: '文生视频',
        description: '根据文字创建视频任务，再用返回的 request_id 查询结果。',
        method: 'POST',
        endpoint: '/v1/videos/generations',
        model: 'happyhorse-1.1-t2v',
        note: '提交后会返回 request_id；视频为异步任务。',
        code: curl('/v1/videos/generations', {
          model: 'happyhorse-1.1-t2v',
          prompt: '一只小狗在雪地奔跑，电影感镜头',
          resolution: '720P',
          ratio: '16:9',
          duration: 5,
        }),
        statusCode: videoStatus(),
      },
      {
        id: 'image-video',
        name: '图生视频',
        description: '提供可公开访问的图片地址，让静态画面动起来。',
        method: 'POST',
        endpoint: '/v1/videos/generations',
        model: 'wanx2.1-i2v-turbo',
        note: 'img_url 必须是平台上游可访问的 HTTPS 图片地址。',
        code: curl('/v1/videos/generations', {
          model: 'wanx2.1-i2v-turbo',
          prompt: '镜头缓慢推进，人物自然眨眼',
          img_url: 'https://example.com/input.jpg',
          resolution: '720P',
        }),
        statusCode: videoStatus(),
      },
    ],
  },
  {
    id: 'mimo',
    name: '小米 MIMO',
    vendor: '小米大模型',
    activeClass: 'border-rose-500 bg-rose-50 text-rose-700 dark:border-rose-500 dark:bg-rose-950/30 dark:text-rose-300',
    dotClass: 'bg-rose-500',
    textClass: 'text-rose-600 dark:text-rose-400',
    capabilities: [
      {
        id: 'text',
        name: '文本生成',
        description: '使用 OpenAI 兼容格式调用 MIMO 文本模型。',
        method: 'POST',
        endpoint: '/v1/chat/completions',
        model: 'mimo-v2.5-pro',
        code: curl('/v1/chat/completions', {
          model: 'mimo-v2.5-pro',
          messages: [{ role: 'user', content: '解释一下什么是大语言模型' }],
        }),
      },
    ],
  },
  {
    id: 'ark',
    name: '火山方舟 Ark',
    vendor: '火山引擎方舟',
    activeClass: 'border-cyan-500 bg-cyan-50 text-cyan-700 dark:border-cyan-500 dark:bg-cyan-950/30 dark:text-cyan-300',
    dotClass: 'bg-cyan-500',
    textClass: 'text-cyan-600 dark:text-cyan-400',
    capabilities: [
      {
        id: 'text',
        name: '文本生成',
        description: '发送对话消息，调用方舟上的文本模型。',
        method: 'POST',
        endpoint: '/v1/chat/completions',
        model: 'doubao-seed-2.0-pro',
        code: curl('/v1/chat/completions', {
          model: 'doubao-seed-2.0-pro',
          messages: [{ role: 'user', content: '写一句夏日旅行文案' }],
        }),
      },
      {
        id: 'image',
        name: '图片生成',
        description: '调用方舟图片模型，根据提示词生成图片。',
        method: 'POST',
        endpoint: '/v1/images/generations',
        model: '以“可用渠道”页面为准',
        note: '方舟图片模型名称会随渠道配置变化，请复制“可用渠道”中显示的模型名称。',
        code: curl('/v1/images/generations', {
          model: 'YOUR_ARK_IMAGE_MODEL',
          prompt: '山西古建屋檐，清晨薄雾，国风摄影',
          size: '1024x1024',
        }),
      },
      {
        id: 'text-video',
        name: '文生视频',
        description: '根据文字创建 Seedance 视频任务。',
        method: 'POST',
        endpoint: '/v1/videos/generations',
        model: 'doubao-seedance-2.0-fast',
        note: '提交后会返回 request_id；视频为异步任务。',
        code: curl('/v1/videos/generations', {
          model: 'doubao-seedance-2.0-fast',
          prompt: '一只猫在草地上奔跑，阳光明媚，慢动作',
          ratio: '16:9',
          duration: 5,
        }),
        statusCode: videoStatus(),
      },
      {
        id: 'image-video',
        name: '图生视频',
        description: '以图片作为首帧创建 Seedance 视频任务。',
        method: 'POST',
        endpoint: '/v1/videos/generations',
        model: 'doubao-seedance-2.0-fast',
        note: 'img_url 必须是平台上游可访问的 HTTPS 图片地址。',
        code: curl('/v1/videos/generations', {
          model: 'doubao-seedance-2.0-fast',
          prompt: '镜头环绕主体，云层缓慢移动',
          img_url: 'https://example.com/input.jpg',
          ratio: '16:9',
          duration: 5,
        }),
        statusCode: videoStatus(),
      },
    ],
  },
])

const activePlatform = computed(() => platforms.value.find((item) => item.id === activePlatformId.value) ?? platforms.value[0])
const activeCapability = computed(() => activePlatform.value.capabilities.find((item) => item.id === activeCapabilityId.value) ?? activePlatform.value.capabilities[0])

function selectPlatform(id: PlatformId) {
  activePlatformId.value = id
  activeCapabilityId.value = 'text'
  copied.value = false
}

async function copyCode(code: string) {
  try {
    await navigator.clipboard.writeText(code)
    copied.value = true
    if (copiedTimer) clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => {
      copied.value = false
    }, 1600)
  } catch {
    copied.value = false
  }
}
</script>

<style scoped>
.quick-step {
  @apply flex items-center gap-3 rounded-xl border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-900/50;
}
.step-number {
  @apply flex h-8 w-8 flex-none items-center justify-center rounded-full bg-primary-100 text-sm font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300;
}
.step-title {
  @apply text-sm font-semibold text-gray-900 dark:text-white;
}
.step-link {
  @apply text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300;
}
.platform-tab {
  @apply inline-flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700;
}
.capability-tab {
  @apply flex-none border-b-2 border-transparent px-4 py-2.5 text-sm font-medium text-gray-500 transition-colors hover:text-gray-900 dark:text-dark-400 dark:hover:text-white;
}
.capability-tab-active {
  @apply border-primary-500 text-primary-600 dark:text-primary-400;
}
.method-badge {
  @apply rounded-md bg-emerald-100 px-2.5 py-1 font-mono text-xs font-bold text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300;
}
.code-shell {
  @apply overflow-hidden rounded-xl border border-gray-800 bg-gray-950 shadow-sm dark:border-dark-600;
}
.code-header {
  @apply flex items-center justify-between border-b border-gray-800 bg-gray-900 px-4 py-2 text-xs text-gray-400;
}
.copy-button {
  @apply rounded px-2 py-1 text-gray-300 transition-colors hover:bg-white/10 hover:text-white;
}
pre {
  @apply max-h-[430px] overflow-auto p-4 text-sm leading-6 text-gray-100;
}
code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  white-space: pre;
}
</style>
