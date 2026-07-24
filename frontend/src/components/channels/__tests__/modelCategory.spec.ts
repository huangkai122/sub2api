import { describe, expect, it } from 'vitest'

import type { UserSupportedModel } from '@/api/channels'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
  type BillingMode
} from '@/constants/channel'
import { categorizeSupportedModels, modelCategoryOf } from '../modelCategory'

function model(name: string, billingMode?: BillingMode): UserSupportedModel {
  return {
    name,
    platform: 'qwen',
    pricing:
      billingMode == null
        ? null
        : {
            billing_mode: billingMode,
            input_price: null,
            output_price: null,
            cache_write_price: null,
            cache_read_price: null,
            image_output_price: null,
            per_request_price: null,
            intervals: []
          }
  }
}

describe('modelCategoryOf', () => {
  it('maps billing modes to display categories', () => {
    expect(modelCategoryOf(model('qwen-plus', BILLING_MODE_TOKEN))).toBe('text')
    expect(modelCategoryOf(model('wanx-t2i', BILLING_MODE_IMAGE))).toBe('image')
    expect(modelCategoryOf(model('happyhorse-t2v', BILLING_MODE_PER_REQUEST))).toBe('video')
  })

  it('falls back to other when pricing is missing', () => {
    expect(modelCategoryOf(model('no-pricing'))).toBe('other')
  })
})

describe('categorizeSupportedModels', () => {
  it('groups models by category in fixed order and keeps in-bucket order', () => {
    const input = [
      model('video-1', BILLING_MODE_PER_REQUEST),
      model('text-1', BILLING_MODE_TOKEN),
      model('img-1', BILLING_MODE_IMAGE),
      model('text-2', BILLING_MODE_TOKEN),
      model('unknown-1')
    ]
    const categories = categorizeSupportedModels(input)
    expect(categories.map((c) => c.key)).toEqual(['text', 'image', 'video', 'other'])
    expect(categories[0].models.map((m) => m.name)).toEqual(['text-1', 'text-2'])
    expect(categories[1].models.map((m) => m.name)).toEqual(['img-1'])
    expect(categories[2].models.map((m) => m.name)).toEqual(['video-1'])
    expect(categories[3].models.map((m) => m.name)).toEqual(['unknown-1'])
  })

  it('omits empty categories', () => {
    const categories = categorizeSupportedModels([
      model('text-1', BILLING_MODE_TOKEN),
      model('text-2', BILLING_MODE_TOKEN)
    ])
    expect(categories.map((c) => c.key)).toEqual(['text'])
  })

  it('returns empty array for empty input', () => {
    expect(categorizeSupportedModels([])).toEqual([])
  })
})
