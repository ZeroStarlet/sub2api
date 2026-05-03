import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import TelemetryPrivacyStatsModal from '../TelemetryPrivacyStatsModal.vue'
import type { Account } from '@/types'
import type { OpsTelemetryPrivacyStatsResponse } from '@/api/admin/ops'

const { mockGetTelemetryPrivacyStats } = vi.hoisted(() => ({
  mockGetTelemetryPrivacyStats: vi.fn()
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getTelemetryPrivacyStats: (...args: any[]) => mockGetTelemetryPrivacyStats(...args)
  }
}))

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: { type: Boolean, default: false },
    title: { type: String, default: '' },
    width: { type: String, default: '' }
  },
  emits: ['close'],
  template: '<div v-if="show" data-testid="dialog"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>'
})

const LoadingSpinnerStub = defineComponent({
  name: 'LoadingSpinner',
  template: '<div data-testid="loading">加载中</div>'
})

const IconStub = defineComponent({
  name: 'Icon',
  props: {
    name: { type: String, default: '' }
  },
  template: '<span>{{ name }}</span>'
})

const makeAccount = (overrides: Partial<Account> = {}): Account => ({
  id: 4,
  name: 'Anthropic OAuth',
  platform: 'anthropic',
  type: 'oauth',
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: true,
  created_at: '2026-05-04T00:00:00Z',
  updated_at: '2026-05-04T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null,
  telemetry_privacy_protected_count: 13,
  ...overrides
})

const makeStats = (overrides: Partial<OpsTelemetryPrivacyStatsResponse> = {}): OpsTelemetryPrivacyStatsResponse => ({
  account_id: 4,
  start_time: '2026-05-03T00:00:00Z',
  end_time: '2026-05-04T00:00:00Z',
  total: 13,
  success_count: 12,
  failure_count: 1,
  body_protected_count: 13,
  body_pass_count: 12,
  body_rewritten_count: 12,
  metadata_present_count: 13,
  metadata_absent_safe_count: 0,
  header_protected_count: 13,
  header_pass_count: 12,
  header_fingerprint_default_count: 13,
  user_agent_default_count: 13,
  x_stainless_default_count: 13,
  x_app_default_count: 13,
  direct_browser_access_default_count: 13,
  tls_pass_count: 13,
  tls_default_count: 13,
  client_request_id_reset_count: 13,
  session_header_protected_count: 13,
  raw_values_logged_count: 13,
  derived_values_logged_count: 13,
  authorization_value_logged_count: 0,
  token_value_logged_count: 0,
  model_value_logged_count: 0,
  request_body_logged_count: 0,
  unique_raw_device_id_count: 9,
  unique_raw_session_id_count: 9,
  unique_raw_client_request_id_count: 13,
  unique_derived_device_id_count: 1,
  unique_derived_session_id_count: 1,
  unique_derived_client_request_id_count: 13,
  endpoint_breakdown: [{ key: 'messages', label: '消息创建', count: 13 }],
  result_breakdown: [{ key: 'metadata.user_id 已替换为账号级匿名遥测身份', label: 'metadata.user_id 已替换为账号级匿名遥测身份', count: 12 }],
  ...overrides
})

const mountModal = (props: { show?: boolean; account?: Account | null } = {}) => mount(TelemetryPrivacyStatsModal, {
  props: {
    show: props.show ?? true,
    account: props.account === undefined ? makeAccount() : props.account
  },
  global: {
    stubs: {
      BaseDialog: BaseDialogStub,
      LoadingSpinner: LoadingSpinnerStub,
      Icon: IconStub
    }
  }
})

describe('TelemetryPrivacyStatsModal', () => {
  beforeEach(() => {
    mockGetTelemetryPrivacyStats.mockReset()
  })

  it('默认按 24h 加载账号遥测隐私统计并展示关键审计摘要', async () => {
    mockGetTelemetryPrivacyStats.mockResolvedValue(makeStats())

    const wrapper = mountModal()
    await flushPromises()

    expect(mockGetTelemetryPrivacyStats).toHaveBeenCalledWith({ account_id: 4, time_range: '24h' })
    expect(wrapper.text()).toContain('遥测隐私统计')
    expect(wrapper.text()).toContain('统计窗口总数')
    expect(wrapper.text()).toContain('总体校验通过')
    expect(wrapper.text()).toContain('累计保护次数')
    expect(wrapper.text()).toContain('消息创建')
    expect(wrapper.text()).toContain('原始 device_id 去重')
    expect(wrapper.text()).not.toContain('raw-device-001')
    expect(wrapper.text()).not.toContain('derived-device-004')
  })

  it('切换时间窗口时使用同一账号重新请求统计', async () => {
    mockGetTelemetryPrivacyStats.mockResolvedValue(makeStats())

    const wrapper = mountModal()
    await flushPromises()

    const sevenDayButton = wrapper.findAll('button').find(button => button.text() === '7d')
    expect(sevenDayButton).toBeTruthy()
    await sevenDayButton!.trigger('click')
    await flushPromises()

    expect(mockGetTelemetryPrivacyStats).toHaveBeenLastCalledWith({ account_id: 4, time_range: '7d' })
  })

  it('接口拒绝异常参数时展示后端错误信息', async () => {
    mockGetTelemetryPrivacyStats.mockRejectedValue({
      response: { data: { detail: '账号 ID 无效' } }
    })

    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.text()).toContain('账号 ID 无效')
    expect(wrapper.text()).not.toContain('统计窗口总数')
  })

  it('关闭弹窗后会丢弃尚未返回的统计请求', async () => {
    let resolveStats!: (value: OpsTelemetryPrivacyStatsResponse) => void
    const pendingStats = new Promise<OpsTelemetryPrivacyStatsResponse>(resolve => {
      resolveStats = resolve
    })
    mockGetTelemetryPrivacyStats.mockReturnValueOnce(pendingStats)

    const wrapper = mountModal()
    await flushPromises()

    // 关闭弹窗等同于释放当前查看上下文；旧请求返回后不得把过期账号或时间窗口的数据重新写回。
    await wrapper.setProps({ show: false, account: null })
    resolveStats(makeStats({ total: 99 }))
    await flushPromises()

    expect(wrapper.text()).not.toContain('统计窗口总数')
    expect(wrapper.text()).not.toContain('99')
  })
})
