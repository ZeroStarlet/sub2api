<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { opsAPI, type OpsRuntimeLogConfig, type OpsSystemLog, type OpsSystemLogSinkHealth } from '@/api/admin/ops'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import { useAppStore } from '@/stores'

const appStore = useAppStore()
const route = useRoute()

const props = withDefaults(defineProps<{
  platformFilter?: string
  refreshToken?: number
}>(), {
  platformFilter: '',
  refreshToken: 0
})

const loading = ref(false)
const logs = ref<OpsSystemLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const health = ref<OpsSystemLogSinkHealth>({
  queue_depth: 0,
  queue_capacity: 0,
  dropped_count: 0,
  write_failed_count: 0,
  written_count: 0,
  avg_write_delay_ms: 0
})

const runtimeLoading = ref(false)
const runtimeSaving = ref(false)
const runtimeConfig = reactive<OpsRuntimeLogConfig>({
  level: 'info',
  enable_sampling: false,
  sampling_initial: 100,
  sampling_thereafter: 100,
  caller: true,
  stacktrace_level: 'error',
  retention_days: 30
})

const filters = reactive({
  time_range: '1h' as '5m' | '30m' | '1h' | '6h' | '24h' | '7d' | '30d',
  start_time: '',
  end_time: '',
  level: '',
  component: '',
  request_id: '',
  client_request_id: '',
  user_id: '',
  account_id: '',
  platform: '',
  model: '',
  q: ''
})
const routeLogFiltersActive = ref(false)

const runtimeLevelOptions = [
  { value: 'debug', label: 'debug' },
  { value: 'info', label: 'info' },
  { value: 'warn', label: 'warn' },
  { value: 'error', label: 'error' }
]

const stacktraceLevelOptions = [
  { value: 'none', label: 'none' },
  { value: 'error', label: 'error' },
  { value: 'fatal', label: 'fatal' }
]

const timeRangeOptions = [
  { value: '5m', label: '5m' },
  { value: '30m', label: '30m' },
  { value: '1h', label: '1h' },
  { value: '6h', label: '6h' },
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' }
]
const timeRangeValues = new Set(timeRangeOptions.map(item => item.value))

const filterLevelOptions = [
  { value: '', label: '全部' },
  { value: 'debug', label: 'debug' },
  { value: 'info', label: 'info' },
  { value: 'warn', label: 'warn' },
  { value: 'error', label: 'error' }
]

const levelBadgeClass = (level: string) => {
  const v = String(level || '').toLowerCase()
  if (v === 'error' || v === 'fatal') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (v === 'warn' || v === 'warning') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (v === 'debug') return 'bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-300'
  return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
}

const formatTime = (value: string) => {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString()
}

const getExtraString = (extra: Record<string, any> | undefined, key: string) => {
  if (!extra) return ''
  const v = extra[key]
  if (v == null) return ''
  if (typeof v === 'string') return v.trim()
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  return ''
}

const getExtraBoolLabel = (extra: Record<string, any> | undefined, key: string) => {
  if (!extra) return ''
  const v = extra[key]
  if (v === true || v === 'true') return '是'
  if (v === false || v === 'false') return '否'
  return ''
}

const getExtraPassLabel = (extra: Record<string, any> | undefined, key: string) => {
  if (!extra) return ''
  const v = extra[key]
  if (v === true || v === 'true') return '通过'
  if (v === false || v === 'false') return '未通过'
  return ''
}

const formatTelemetryPrivacyEndpoint = (endpoint: string) => {
  if (endpoint === 'messages') return '消息创建'
  if (endpoint === 'count_tokens') return '令牌计数'
  return endpoint || '-'
}

const formatTelemetryPrivacyCheck = (
  extra: Record<string, any> | undefined,
  passKey: string,
  fallbackKey?: string
) => {
  const pass = getExtraPassLabel(extra, passKey)
  if (pass) return pass
  if (!fallbackKey) return '-'
  return getExtraBoolLabel(extra, fallbackKey) || '-'
}

const formatTelemetryPrivacyBoundary = (extra: Record<string, any> | undefined) => {
  const values = [
    `认证=${getExtraBoolLabel(extra, 'authorization_value_logged') || '否'}`,
    `token=${getExtraBoolLabel(extra, 'token_value_logged') || '否'}`,
    `模型=${getExtraBoolLabel(extra, 'model_value_logged') || '否'}`,
    `正文=${getExtraBoolLabel(extra, 'request_body_logged') || '否'}`
  ]
  return values.join('/')
}

// 遥测隐私审计日志的 extra 保留完整字段用于统计弹窗和管理员深查；列表行只展示判定摘要、
// 关键处理结论和敏感边界，避免高频日志把上百个 key=value 全部铺开导致无法快速判断成败。
const formatTelemetryPrivacyLogSummary = (row: OpsSystemLog) => {
  const extra = row.extra || {}
  const bodyResult = getExtraString(extra, 'body_result') || '-'
  const summaryParts = [
    '遥测隐私=已处理',
    `端点=${formatTelemetryPrivacyEndpoint(getExtraString(extra, 'endpoint'))}`,
    `path=${getExtraString(extra, 'path') || '-'}`,
    `总体=${getExtraPassLabel(extra, 'protection_success') || '-'}`,
    `正文=${formatTelemetryPrivacyCheck(extra, 'body_privacy_protection_pass', 'body_protected')}`,
    `改写=${getExtraBoolLabel(extra, 'body_rewritten') || '否'}`,
    `header=${formatTelemetryPrivacyCheck(extra, 'header_privacy_protection_pass', 'header_protected')}`,
    `TLS=${formatTelemetryPrivacyCheck(extra, 'tls_privacy_protection_pass', 'tls_fingerprint_final_default')}`,
    `请求ID=${getExtraBoolLabel(extra, 'x_client_request_id_regenerated') || '否'}`,
    `会话头=${getExtraBoolLabel(extra, 'x_claude_code_session_final_protected') || '否'}`,
    `metadata存在=${getExtraBoolLabel(extra, 'metadata_user_id_present') || '否'}`,
    `metadata结果=${bodyResult}`,
    `原始字段=${getExtraBoolLabel(extra, 'raw_values_logged') || '否'}`,
    `派生字段=${getExtraBoolLabel(extra, 'derived_values_logged') || '否'}`,
    `敏感边界=${formatTelemetryPrivacyBoundary(extra)}`
  ]
  if (row.account_id != null) summaryParts.push(`acc=${row.account_id}`)
  if (row.platform) summaryParts.push(`platform=${row.platform}`)
  return summaryParts.join(' ')
}

const formatSystemLogDetail = (row: OpsSystemLog) => {
  const parts: string[] = []
  const msg = String(row.message || '').trim()
  if (msg) parts.push(msg)

  const extra = row.extra || {}
  const statusCode = getExtraString(extra, 'status_code')
  const latencyMs = getExtraString(extra, 'latency_ms')
  const method = getExtraString(extra, 'method')
  const path = getExtraString(extra, 'path')
  const clientIP = getExtraString(extra, 'client_ip')
  const protocol = getExtraString(extra, 'protocol')

  const accessParts: string[] = []
  if (statusCode) accessParts.push(`status=${statusCode}`)
  if (latencyMs) accessParts.push(`latency_ms=${latencyMs}`)
  if (method) accessParts.push(`method=${method}`)
  if (path) accessParts.push(`path=${path}`)
  if (clientIP) accessParts.push(`ip=${clientIP}`)
  if (protocol) accessParts.push(`proto=${protocol}`)
  if (accessParts.length > 0) parts.push(accessParts.join(' '))

  const isTelemetryPrivacyLog = extra.telemetry_privacy === true || extra.telemetry_privacy === 'true'
  if (isTelemetryPrivacyLog) {
    parts.push(formatTelemetryPrivacyLogSummary(row))
    return parts.join('  ')
  }
  const corrParts: string[] = []
  if (row.request_id && !isTelemetryPrivacyLog) corrParts.push(`req=${row.request_id}`)
  if (row.client_request_id && !isTelemetryPrivacyLog) corrParts.push(`client_req=${row.client_request_id}`)
  if (row.user_id != null && !isTelemetryPrivacyLog) corrParts.push(`user=${row.user_id}`)
  if (row.account_id != null) corrParts.push(`acc=${row.account_id}`)
  if (row.platform) corrParts.push(`platform=${row.platform}`)
  if (row.model && !isTelemetryPrivacyLog) corrParts.push(`model=${row.model}`)
  if (corrParts.length > 0) parts.push(corrParts.join(' '))

  const errors = getExtraString(extra, 'errors')
  if (errors) parts.push(`errors=${errors}`)
  const err = getExtraString(extra, 'err') || getExtraString(extra, 'error')
  if (err) parts.push(`error=${err}`)

  // 用空格拼接，交给 CSS 自动换行，尽量“填满再换行”。
  return parts.join('  ')
}

const toRFC3339 = (value: string) => {
  if (!value) return undefined
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return undefined
  return d.toISOString()
}

const readRouteString = (key: string): string => {
  const value = route.query[key]
  if (typeof value === 'string') return value
  if (Array.isArray(value) && typeof value[0] === 'string') return value[0]
  return ''
}

// 账号列表的“日志”入口使用 system_log_* 查询参数深链到这里，避免覆盖运维看板自身的筛选状态。
const applyRouteLogFilters = (): boolean => {
  let changed = false
  const applyString = (key: 'component' | 'account_id' | 'q', value: string) => {
    if (filters[key] !== value) {
      filters[key] = value
      changed = true
    }
  }

  const timeRange = readRouteString('system_log_time_range')
  const accountID = readRouteString('system_log_account_id')
  const component = readRouteString('system_log_component')
  const query = readRouteString('system_log_q')
  const hasRouteFilters = Boolean(timeRange || accountID || component || query)

  if (!hasRouteFilters) {
    if (!routeLogFiltersActive.value) return false
    routeLogFiltersActive.value = false
    if (filters.time_range !== '1h') {
      filters.time_range = '1h'
      changed = true
    }
    applyString('account_id', '')
    applyString('component', '')
    applyString('q', '')
    return changed
  }

  routeLogFiltersActive.value = true
  const nextTimeRange = timeRangeValues.has(timeRange) ? timeRange : '1h'
  if (filters.time_range !== nextTimeRange) {
    filters.time_range = nextTimeRange as typeof filters.time_range
    changed = true
  }

  applyString('account_id', accountID)
  applyString('component', component)
  applyString('q', query)
  return changed
}

const buildQuery = () => {
  const query: Record<string, any> = {
    page: page.value,
    page_size: pageSize.value,
    time_range: filters.time_range
  }

  if (filters.time_range === '30d') {
    query.time_range = '30d'
  }
  if (filters.start_time) query.start_time = toRFC3339(filters.start_time)
  if (filters.end_time) query.end_time = toRFC3339(filters.end_time)
  if (filters.level.trim()) query.level = filters.level.trim()
  if (filters.component.trim()) query.component = filters.component.trim()
  if (filters.request_id.trim()) query.request_id = filters.request_id.trim()
  if (filters.client_request_id.trim()) query.client_request_id = filters.client_request_id.trim()
  if (filters.user_id.trim()) {
    const v = Number.parseInt(filters.user_id.trim(), 10)
    if (Number.isFinite(v) && v > 0) query.user_id = v
  }
  if (filters.account_id.trim()) {
    const v = Number.parseInt(filters.account_id.trim(), 10)
    if (Number.isFinite(v) && v > 0) query.account_id = v
  }
  if (filters.platform.trim()) query.platform = filters.platform.trim()
  if (filters.model.trim()) query.model = filters.model.trim()
  if (filters.q.trim()) query.q = filters.q.trim()
  return query
}

const fetchLogs = async () => {
  loading.value = true
  try {
    const res = await opsAPI.listSystemLogs(buildQuery())
    logs.value = res.items || []
    total.value = res.total || 0
  } catch (err: any) {
    console.error('[OpsSystemLogTable] Failed to fetch logs', err)
    appStore.showError(err?.response?.data?.detail || '系统日志加载失败')
  } finally {
    loading.value = false
  }
}

const fetchHealth = async () => {
  try {
    health.value = await opsAPI.getSystemLogSinkHealth()
  } catch {
    // 忽略健康数据读取失败，不影响主流程。
  }
}

const loadRuntimeConfig = async () => {
  runtimeLoading.value = true
  try {
    const cfg = await opsAPI.getRuntimeLogConfig()
    runtimeConfig.level = cfg.level
    runtimeConfig.enable_sampling = cfg.enable_sampling
    runtimeConfig.sampling_initial = cfg.sampling_initial
    runtimeConfig.sampling_thereafter = cfg.sampling_thereafter
    runtimeConfig.caller = cfg.caller
    runtimeConfig.stacktrace_level = cfg.stacktrace_level
    runtimeConfig.retention_days = cfg.retention_days
  } catch (err: any) {
    console.error('[OpsSystemLogTable] Failed to load runtime log config', err)
  } finally {
    runtimeLoading.value = false
  }
}

const saveRuntimeConfig = async () => {
  runtimeSaving.value = true
  try {
    const saved = await opsAPI.updateRuntimeLogConfig({ ...runtimeConfig })
    runtimeConfig.level = saved.level
    runtimeConfig.enable_sampling = saved.enable_sampling
    runtimeConfig.sampling_initial = saved.sampling_initial
    runtimeConfig.sampling_thereafter = saved.sampling_thereafter
    runtimeConfig.caller = saved.caller
    runtimeConfig.stacktrace_level = saved.stacktrace_level
    runtimeConfig.retention_days = saved.retention_days
    appStore.showSuccess('日志运行时配置已生效')
  } catch (err: any) {
    console.error('[OpsSystemLogTable] Failed to save runtime log config', err)
    appStore.showError(err?.response?.data?.detail || '保存日志配置失败')
  } finally {
    runtimeSaving.value = false
  }
}

const resetRuntimeConfig = async () => {
  const ok = window.confirm('确认回滚为启动配置（env/yaml）并立即生效？')
  if (!ok) return

  runtimeSaving.value = true
  try {
    const saved = await opsAPI.resetRuntimeLogConfig()
    runtimeConfig.level = saved.level
    runtimeConfig.enable_sampling = saved.enable_sampling
    runtimeConfig.sampling_initial = saved.sampling_initial
    runtimeConfig.sampling_thereafter = saved.sampling_thereafter
    runtimeConfig.caller = saved.caller
    runtimeConfig.stacktrace_level = saved.stacktrace_level
    runtimeConfig.retention_days = saved.retention_days
    appStore.showSuccess('已回滚到启动日志配置')
    await fetchHealth()
  } catch (err: any) {
    console.error('[OpsSystemLogTable] Failed to reset runtime log config', err)
    appStore.showError(err?.response?.data?.detail || '回滚日志配置失败')
  } finally {
    runtimeSaving.value = false
  }
}

const cleanupCurrentFilter = async () => {
  const ok = window.confirm('确认按当前筛选条件清理系统日志？该操作不可撤销。')
  if (!ok) return
  try {
    const payload = {
      start_time: toRFC3339(filters.start_time),
      end_time: toRFC3339(filters.end_time),
      level: filters.level.trim() || undefined,
      component: filters.component.trim() || undefined,
      request_id: filters.request_id.trim() || undefined,
      client_request_id: filters.client_request_id.trim() || undefined,
      user_id: filters.user_id.trim() ? Number.parseInt(filters.user_id.trim(), 10) : undefined,
      account_id: filters.account_id.trim() ? Number.parseInt(filters.account_id.trim(), 10) : undefined,
      platform: filters.platform.trim() || undefined,
      model: filters.model.trim() || undefined,
      q: filters.q.trim() || undefined
    }
    const res = await opsAPI.cleanupSystemLogs(payload)
    appStore.showSuccess(`清理完成，删除 ${res.deleted || 0} 条日志`)
    page.value = 1
    await Promise.all([fetchLogs(), fetchHealth()])
  } catch (err: any) {
    console.error('[OpsSystemLogTable] Failed to cleanup logs', err)
    appStore.showError(err?.response?.data?.detail || '清理系统日志失败')
  }
}

const resetFilters = () => {
  filters.time_range = '1h'
  filters.start_time = ''
  filters.end_time = ''
  filters.level = ''
  filters.component = ''
  filters.request_id = ''
  filters.client_request_id = ''
  filters.user_id = ''
  filters.account_id = ''
  filters.platform = props.platformFilter || ''
  filters.model = ''
  filters.q = ''
  page.value = 1
  fetchLogs()
}

watch(() => props.platformFilter, (v) => {
  const nextPlatform = v || ''
  if (filters.platform === nextPlatform) return
  filters.platform = nextPlatform
  page.value = 1
  fetchLogs()
})

watch(() => props.refreshToken, () => {
  fetchLogs()
  fetchHealth()
})

watch(() => route.query, () => {
  if (applyRouteLogFilters()) {
    page.value = 1
    fetchLogs()
  }
})

const onPageChange = (next: number) => {
  page.value = next
  fetchLogs()
}

const onPageSizeChange = (next: number) => {
  pageSize.value = next
  page.value = 1
  fetchLogs()
}

const applyFilters = () => {
  page.value = 1
  fetchLogs()
}

const hasData = computed(() => logs.value.length > 0)

onMounted(async () => {
  if (props.platformFilter) {
    filters.platform = props.platformFilter
  }
  applyRouteLogFilters()
  await Promise.all([fetchLogs(), fetchHealth(), loadRuntimeConfig()])
})
</script>

<template>
  <section class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900/60">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="text-sm font-bold text-gray-900 dark:text-white">系统日志</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">默认按最新时间倒序，支持筛选搜索与按条件清理。</p>
      </div>
      <div class="flex flex-wrap items-center gap-2 text-xs">
        <span class="rounded-md bg-gray-100 px-2 py-1 text-gray-700 dark:bg-dark-700 dark:text-gray-200">队列 {{ health.queue_depth }}/{{ health.queue_capacity }}</span>
        <span class="rounded-md bg-gray-100 px-2 py-1 text-gray-700 dark:bg-dark-700 dark:text-gray-200">写入 {{ health.written_count }}</span>
        <span class="rounded-md bg-amber-100 px-2 py-1 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">丢弃 {{ health.dropped_count }}</span>
        <span class="rounded-md bg-red-100 px-2 py-1 text-red-700 dark:bg-red-900/30 dark:text-red-300">失败 {{ health.write_failed_count }}</span>
      </div>
    </div>

    <div class="mb-4 rounded-xl border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800/70">
      <div class="mb-2 flex items-center justify-between">
        <div class="text-xs font-semibold text-gray-700 dark:text-gray-200">运行时日志配置（实时生效）</div>
        <span v-if="runtimeLoading" class="text-xs text-gray-500">加载中...</span>
      </div>
      <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-6">
        <label class="text-xs text-gray-600 dark:text-gray-300">
          级别
          <Select v-model="runtimeConfig.level" class="mt-1" :options="runtimeLevelOptions" />
        </label>
        <label class="text-xs text-gray-600 dark:text-gray-300">
          堆栈阈值
          <Select v-model="runtimeConfig.stacktrace_level" class="mt-1" :options="stacktraceLevelOptions" />
        </label>
        <label class="text-xs text-gray-600 dark:text-gray-300">
          采样初始
          <input v-model.number="runtimeConfig.sampling_initial" type="number" min="1" class="input mt-1" />
        </label>
        <label class="text-xs text-gray-600 dark:text-gray-300">
          采样后续
          <input v-model.number="runtimeConfig.sampling_thereafter" type="number" min="1" class="input mt-1" />
        </label>
        <label class="text-xs text-gray-600 dark:text-gray-300">
          保留天数
          <input v-model.number="runtimeConfig.retention_days" type="number" min="1" max="3650" class="input mt-1" />
        </label>
        <div class="md:col-span-2 xl:col-span-6">
          <div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
            <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
              <label class="inline-flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300">
                <input v-model="runtimeConfig.caller" type="checkbox" />
                caller
              </label>
              <label class="inline-flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300">
                <input v-model="runtimeConfig.enable_sampling" type="checkbox" />
                sampling
              </label>
            </div>
            <div class="flex flex-wrap items-center gap-2 lg:justify-end">
              <button type="button" class="btn btn-primary btn-sm" :disabled="runtimeSaving" @click="saveRuntimeConfig">
                {{ runtimeSaving ? '保存中...' : '保存并生效' }}
              </button>
              <button type="button" class="btn btn-secondary btn-sm" :disabled="runtimeSaving" @click="resetRuntimeConfig">
                回滚默认值
              </button>
            </div>
          </div>
        </div>
      </div>
      <p v-if="health.last_error" class="mt-2 text-xs text-red-600 dark:text-red-400">最近写入错误：{{ health.last_error }}</p>
    </div>

    <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-5">
      <label class="text-xs text-gray-600 dark:text-gray-300">
        时间范围
        <Select v-model="filters.time_range" class="mt-1" :options="timeRangeOptions" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        开始时间（可选）
        <input v-model="filters.start_time" type="datetime-local" class="input mt-1" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        结束时间（可选）
        <input v-model="filters.end_time" type="datetime-local" class="input mt-1" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        级别
        <Select v-model="filters.level" class="mt-1" :options="filterLevelOptions" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        组件
        <input v-model="filters.component" type="text" class="input mt-1" placeholder="如 http.access" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        request_id
        <input v-model="filters.request_id" type="text" class="input mt-1" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        client_request_id
        <input v-model="filters.client_request_id" type="text" class="input mt-1" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        user_id
        <input v-model="filters.user_id" type="text" class="input mt-1" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        account_id
        <input v-model="filters.account_id" type="text" class="input mt-1" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        平台
        <input v-model="filters.platform" type="text" class="input mt-1" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        模型
        <input v-model="filters.model" type="text" class="input mt-1" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        关键词
        <input v-model="filters.q" type="text" class="input mt-1" placeholder="消息/request_id" />
      </label>
    </div>

    <div class="mb-3 flex flex-wrap gap-2">
      <button type="button" class="btn btn-primary btn-sm" @click="applyFilters">查询</button>
      <button type="button" class="btn btn-secondary btn-sm" @click="resetFilters">重置</button>
      <button type="button" class="btn btn-danger btn-sm" @click="cleanupCurrentFilter">按当前筛选清理</button>
      <button type="button" class="btn btn-secondary btn-sm" @click="fetchHealth">刷新健康指标</button>
    </div>

    <div class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
      <div v-if="loading" class="px-4 py-8 text-center text-sm text-gray-500">加载中...</div>
      <div v-else-if="!hasData" class="px-4 py-8 text-center text-sm text-gray-500">暂无系统日志</div>
      <div v-else class="overflow-auto">
        <table class="min-w-full table-fixed divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-900">
            <tr>
              <th class="w-[170px] px-3 py-2 text-left text-[11px] font-semibold text-gray-500">时间</th>
              <th class="w-[80px] px-3 py-2 text-left text-[11px] font-semibold text-gray-500">级别</th>
              <th class="px-3 py-2 text-left text-[11px] font-semibold text-gray-500">日志详细信息</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
            <tr v-for="row in logs" :key="row.id" class="align-top">
              <td class="px-3 py-2 text-xs text-gray-700 dark:text-gray-300">{{ formatTime(row.created_at) }}</td>
              <td class="px-3 py-2 text-xs">
                <span class="inline-flex rounded-full px-2 py-0.5 font-semibold" :class="levelBadgeClass(row.level)">
                  {{ row.level }}
                </span>
              </td>
              <td class="px-3 py-2 text-xs text-gray-700 dark:text-gray-300 whitespace-normal break-all">
                {{ formatSystemLogDetail(row) }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <Pagination
        :total="total"
        :page="page"
        :page-size="pageSize"
        @update:page="onPageChange"
        @update:page-size="onPageSizeChange"
      />
    </div>
  </section>
</template>
