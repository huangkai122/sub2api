import type { UserSupportedModel } from '@/api/channels'
import {
  BILLING_MODE_TOKEN,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_IMAGE
} from '@/constants/channel'

/**
 * 支持模型的展示分类：与后台渠道定价（模型定价）的计费模式一一对应——
 * token ⇒ 文本生成、图片（按次）⇒ 图片生成、按次 ⇒ 视频生成；
 * 未配置定价的模型归入 other。
 */
export type ModelCategoryKey = 'text' | 'image' | 'video' | 'other'

export interface ModelCategory {
  key: ModelCategoryKey
  models: UserSupportedModel[]
}

/** 分类展示顺序：文本 → 图片 → 视频 → 其他。 */
const CATEGORY_ORDER: readonly ModelCategoryKey[] = ['text', 'image', 'video', 'other']

export function modelCategoryOf(model: UserSupportedModel): ModelCategoryKey {
  switch (model.pricing?.billing_mode) {
    case BILLING_MODE_TOKEN:
      return 'text'
    case BILLING_MODE_IMAGE:
      return 'image'
    case BILLING_MODE_PER_REQUEST:
      return 'video'
    default:
      return 'other'
  }
}

/**
 * 把平台 section 下的支持模型按计费模式分桶，保持分类固定顺序、
 * 桶内保持原始顺序；空分类不出现。
 */
export function categorizeSupportedModels(models: UserSupportedModel[]): ModelCategory[] {
  const buckets = new Map<ModelCategoryKey, UserSupportedModel[]>()
  for (const m of models) {
    const key = modelCategoryOf(m)
    const list = buckets.get(key)
    if (list) {
      list.push(m)
    } else {
      buckets.set(key, [m])
    }
  }
  return CATEGORY_ORDER.filter((key) => buckets.has(key)).map((key) => ({
    key,
    models: buckets.get(key) as UserSupportedModel[]
  }))
}
