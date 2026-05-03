import { describe, it, expect, beforeEach, vi } from 'vitest'
import { defineComponent, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import OpsSystemLogTable from '../OpsSystemLogTable.vue'

const mockListSystemLogs = vi.fn()
const mockGetSystemLogSinkHealth = vi.fn()
const mockGetRuntimeLogConfig = vi.fn()

const mockRoute = reactive({
  query: {}
})

vi.mock('vue-router', () => ({
  useRoute: () => mockRoute
}))

vi.mock('@/api/admin/ops', () => ({
  default: {
    listSystemLogs: (...args: any[]) => mockListSystemLogs(...args),
    getSystemLogSinkHealth: (...args: any[]) => mockGetSystemLogSinkHealth(...args),
    getRuntimeLogConfig: (...args: any[]) => mockGetRuntimeLogConfig(...args),
    updateRuntimeLogConfig: vi.fn(),
    resetRuntimeLogConfig: vi.fn(),
    cleanupSystemLogs: vi.fn()
  },
  opsAPI: {
    listSystemLogs: (...args: any[]) => mockListSystemLogs(...args),
    getSystemLogSinkHealth: (...args: any[]) => mockGetSystemLogSinkHealth(...args),
    getRuntimeLogConfig: (...args: any[]) => mockGetRuntimeLogConfig(...args),
    updateRuntimeLogConfig: vi.fn(),
    resetRuntimeLogConfig: vi.fn(),
    cleanupSystemLogs: vi.fn()
  }
}))

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: { type: [String, Number, Boolean], default: '' },
    options: { type: Array, default: () => [] }
  },
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />'
})

const PaginationStub = defineComponent({
  name: 'PaginationStub',
  props: {
    total: { type: Number, default: 0 },
    page: { type: Number, default: 1 },
    pageSize: { type: Number, default: 20 }
  },
  template: '<div class="pagination-stub" />'
})

describe('OpsSystemLogTable', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockRoute.query = {}
    mockGetSystemLogSinkHealth.mockResolvedValue({
      queue_depth: 0,
      queue_capacity: 10000,
      dropped_count: 0,
      write_failed_count: 0,
      written_count: 1,
      avg_write_delay_ms: 0
    })
    mockGetRuntimeLogConfig.mockResolvedValue({
      level: 'info',
      enable_sampling: false,
      sampling_initial: 100,
      sampling_thereafter: 100,
      caller: true,
      stacktrace_level: 'error',
      retention_days: 30
    })
  })

  it('遥测隐私系统日志优先展示处理摘要并隐藏冗长审计值', async () => {
    mockListSystemLogs.mockResolvedValue({
      items: [
        {
          id: 1,
          created_at: '2026-05-04T02:40:03Z',
          level: 'info',
          component: 'service.gateway.audit.telemetry_privacy',
          message: '遥测隐私保护已处理',
          request_id: 'request-id-should-not-display',
          client_request_id: 'client-request-id-should-not-display',
          user_id: 1,
          account_id: 4,
          platform: 'anthropic',
          model: 'claude-model-should-not-display',
          extra: {
            telemetry_privacy: true,
            endpoint: 'messages',
            path: '/v1/messages',
            privacy_scope: 'Anthropic 平台 OAuth / Setup Token 账号',
            protection_success: true,
            body_protected: true,
            body_privacy_protection_pass: true,
            body_rewritten: true,
            body_result: 'metadata.user_id 已替换为账号级匿名遥测身份',
            metadata_user_id_present: true,
            metadata_user_id_absent_safe: false,
            metadata_user_id_absent_policy: '缺失时不新增遥测身份',
            metadata_user_id_check_applicable: true,
            metadata_user_id_string: true,
            metadata_user_id_parsed: true,
            metadata_user_id_format: 'json',
            header_protected: true,
            header_privacy_protection_pass: true,
            tls_privacy_protection_pass: true,
            x_client_request_id_regenerated: true,
            x_claude_code_session_final_protected: true,
            sensitive_values_logged: true,
            sensitive_values_logged_reason: '按管理员审计要求记录客户端原始遥测值和账号级伪装派生值，不记录认证值、模型或请求正文',
            raw_metadata_user_id: '{"device_id":"raw-device-001","account_uuid":"raw-account-001","session_id":"raw-session-001"}',
            raw_metadata_user_id_parsed: true,
            raw_metadata_user_id_format: 'json',
            raw_device_id: 'raw-device-001',
            raw_account_uuid: 'raw-account-001',
            raw_session_id: 'raw-session-001',
            raw_x_claude_code_session_id: 'raw-session-001',
            raw_x_client_request_id: 'raw-client-request-001',
            raw_user_agent: 'claude-cli/2.1.92 (external, cli)',
            raw_x_stainless_os: 'Linux',
            raw_x_stainless_arch: 'arm64',
            raw_values_logged: true,
            authorization: 'Bearer authorization-should-not-display',
            token_value: 'token-should-not-display',
            request_body: '{"messages":[{"role":"user","content":"body-should-not-display"}]}',
            derived_metadata_user_id: '{"device_id":"derived-device-004","account_uuid":"","session_id":"derived-session-004"}',
            derived_metadata_user_id_candidate: '{"device_id":"derived-device-004","account_uuid":"","session_id":"derived-session-004"}',
            derived_metadata_user_id_upstream: true,
            derived_device_id: 'derived-device-004',
            derived_device_id_candidate: 'derived-device-004',
            derived_account_uuid: '',
            derived_account_uuid_candidate: '',
            derived_session_id: 'derived-session-004',
            derived_session_id_candidate: 'derived-session-004',
            derived_x_claude_code_session_id: 'derived-session-004',
            derived_x_client_request_id: 'derived-client-request-001',
            derived_user_agent: 'claude-cli/2.1.92 (external, cli)',
            derived_x_stainless_os: 'MacOS',
            derived_x_stainless_arch: 'x64',
            derived_tls_fingerprint_profile: 'Built-in Default (Node.js 24.x)',
            derived_values_logged: true,
            raw_device_id_logged: true,
            raw_session_id_logged: true,
            raw_account_uuid_logged: true,
            raw_client_request_id_logged: true,
            derived_device_id_logged: true,
            derived_session_id_logged: true,
            authorization_value_logged: false,
            token_value_logged: false,
            model_value_logged: false,
            request_body_logged: false
          }
        }
      ],
      total: 1,
      page: 1,
      page_size: 20
    })

    const wrapper = mount(OpsSystemLogTable, {
      global: {
        stubs: {
          Select: SelectStub,
          Pagination: PaginationStub
        }
      }
    })
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('遥测隐私=已处理')
    expect(text).toContain('端点=消息创建')
    expect(text).toContain('总体=通过')
    expect(text).toContain('正文=通过')
    expect(text).toContain('header=通过')
    expect(text).toContain('metadata结果=metadata.user_id 已替换为账号级匿名遥测身份')
    expect(text).toContain('敏感边界=认证=否/token=否/模型=否/正文=否')
    expect(text).not.toContain('原始device_id值=raw-device-001')
    expect(text).not.toContain('派生device_id上游值=derived-device-004')
    expect(text).not.toContain('派生TLS指纹配置=Built-in Default (Node.js 24.x)')
    expect(text).not.toContain('authorization-should-not-display')
    expect(text).not.toContain('token-should-not-display')
    expect(text).not.toContain('body-should-not-display')
    expect(text).not.toContain('claude-model-should-not-display')
    expect(text).not.toContain('request-id-should-not-display')
    expect(text).not.toContain('client-request-id-should-not-display')
    expect(text).not.toContain('user=1')
  })

  it('非遥测系统日志继续展示原有关联字段', async () => {
    mockListSystemLogs.mockResolvedValue({
      items: [
        {
          id: 2,
          created_at: '2026-05-04T02:41:03Z',
          level: 'info',
          component: 'http.access',
          message: 'http request completed',
          request_id: 'normal-request-001',
          client_request_id: 'normal-client-request-001',
          user_id: 2,
          account_id: 5,
          platform: 'anthropic',
          model: 'claude-normal-model',
          extra: {
            status_code: 200,
            latency_ms: 123,
            method: 'POST',
            path: '/v1/messages',
            client_ip: '192.168.10.22',
            protocol: 'HTTP/1.1'
          }
        }
      ],
      total: 1,
      page: 1,
      page_size: 20
    })

    const wrapper = mount(OpsSystemLogTable, {
      global: {
        stubs: {
          Select: SelectStub,
          Pagination: PaginationStub
        }
      }
    })
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('status=200')
    expect(text).toContain('latency_ms=123')
    expect(text).toContain('req=normal-request-001')
    expect(text).toContain('client_req=normal-client-request-001')
    expect(text).toContain('user=2')
    expect(text).toContain('acc=5')
    expect(text).toContain('platform=anthropic')
    expect(text).toContain('model=claude-normal-model')
  })
})
