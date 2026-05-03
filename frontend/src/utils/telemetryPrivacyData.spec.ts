import { describe, expect, it } from 'vitest'
import {
  withoutTelemetryPrivacyExportFields,
  withTelemetryPrivacyEnabledForImportedAccounts
} from './telemetryPrivacyData'
import type { AdminDataPayload } from '@/types'

const basePayload = (): AdminDataPayload => ({
  exported_at: '2026-05-03T00:00:00Z',
  proxies: [],
  accounts: [
    {
      name: 'anthropic-oauth',
      platform: 'anthropic',
      type: 'oauth',
      credentials: { access_token: 'a' },
      extra: { telemetry_privacy_protected_count: 7, custom: 'keep' },
      concurrency: 1,
      priority: 1
    },
    {
      name: 'anthropic-setup-token',
      platform: 'anthropic',
      type: 'setup-token',
      credentials: { refresh_token: 'b' },
      concurrency: 1,
      priority: 1
    },
    {
      name: 'anthropic-apikey',
      platform: 'anthropic',
      type: 'apikey',
      credentials: { api_key: 'c' },
      extra: { custom: 'keep' },
      concurrency: 1,
      priority: 1
    },
    {
      name: 'openai-oauth',
      platform: 'openai',
      type: 'oauth',
      credentials: { refresh_token: 'd' },
      extra: { custom: 'keep' },
      concurrency: 1,
      priority: 1
    }
  ]
})

describe('遥测隐私保护迁移数据处理', () => {
  it('导入时只为 Anthropic OAuth / Setup Token 账号补写保护开关', () => {
    const payload = basePayload()

    const result = withTelemetryPrivacyEnabledForImportedAccounts(payload)

    expect(result.count).toBe(2)
    expect(result.payload.accounts[0].extra).toEqual({
      telemetry_privacy_protected_count: 7,
      telemetry_privacy_enabled: true,
      custom: 'keep'
    })
    expect(result.payload.accounts[1].extra).toEqual({
      telemetry_privacy_enabled: true
    })
    expect(result.payload.accounts[2].extra).toEqual({ custom: 'keep' })
    expect(result.payload.accounts[3].extra).toEqual({ custom: 'keep' })
    expect(payload.accounts[1].extra).toBeUndefined()
  })

  it('导出取消时只移除保护开关和累计次数', () => {
    const payload = basePayload()
    payload.accounts[0].extra = {
      telemetry_privacy_enabled: true,
      telemetry_privacy_protected_count: 9,
      custom: 'keep'
    }
    payload.accounts[1].extra = {
      telemetry_privacy_enabled: true,
      telemetry_privacy_protected_count: 3
    }

    const result = withoutTelemetryPrivacyExportFields(payload)

    expect(result.accounts[0].extra).toEqual({ custom: 'keep' })
    expect(result.accounts[1].extra).toBeUndefined()
    expect(result.accounts[2].extra).toEqual({ custom: 'keep' })
    expect(result.accounts[3].extra).toEqual({ custom: 'keep' })
    expect(payload.accounts[0].extra).toEqual({
      telemetry_privacy_enabled: true,
      telemetry_privacy_protected_count: 9,
      custom: 'keep'
    })
  })
})
