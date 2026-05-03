<template>
  <BaseDialog
    :show="show"
    title="遥测隐私统计"
    width="extra-wide"
    @close="handleClose"
  >
    <div class="space-y-4">
      <div
        v-if="account"
        class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-emerald-200 bg-emerald-50 p-3 dark:border-emerald-800/40 dark:bg-emerald-900/15"
      >
        <div class="flex items-center gap-3">
          <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-emerald-600 text-white">
            <Icon name="shield" size="sm" :stroke-width="2" />
          </div>
          <div>
            <div class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ account.name }}</div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              账号 {{ account.id }} · {{ formatTimeRange(stats?.start_time, stats?.end_time) }}
            </div>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button
            v-for="item in timeRangeOptions"
            :key="item.value"
            type="button"
            :class="[
              'rounded px-2 py-1 text-xs font-medium transition-colors',
              timeRange === item.value
                ? 'bg-emerald-600 text-white'
                : 'bg-white text-gray-700 hover:bg-gray-100 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600'
            ]"
            @click="setTimeRange(item.value)"
          >
            {{ item.label }}
          </button>
        </div>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <div v-else-if="error" class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800/40 dark:bg-red-900/20 dark:text-red-300">
        {{ error }}
      </div>

      <template v-else-if="stats">
        <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
          <StatCard title="统计窗口总数" :value="formatNumber(stats.total)" tone="blue" />
          <StatCard title="总体校验通过" :value="formatNumber(stats.success_count)" :sub="formatRate(stats.success_count, stats.total)" tone="emerald" />
          <StatCard title="异常或未通过" :value="formatNumber(stats.failure_count)" :sub="formatRate(stats.failure_count, stats.total)" tone="red" />
          <StatCard title="累计保护次数" :value="formatNumber(getTelemetryPrivacyProtectedCount(account))" tone="gray" />
        </div>

        <div class="grid gap-4 lg:grid-cols-3">
          <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-gray-100">核心保护校验</h4>
            <div class="mt-3 space-y-2">
              <MetricRow label="正文隐私校验通过" :value="stats.body_pass_count" :total="stats.total" />
              <MetricRow label="metadata.user_id 已改写" :value="stats.body_rewritten_count" :total="stats.total" />
              <MetricRow label="header 校验通过" :value="stats.header_pass_count" :total="stats.total" />
              <MetricRow label="TLS 指纹校验通过" :value="stats.tls_pass_count" :total="stats.total" />
              <MetricRow label="请求 ID 已重置" :value="stats.client_request_id_reset_count" :total="stats.total" />
              <MetricRow label="会话头已保护" :value="stats.session_header_protected_count" :total="stats.total" />
            </div>
          </section>

          <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-gray-100">默认指纹收敛</h4>
            <div class="mt-3 space-y-2">
              <MetricRow label="header 指纹默认" :value="stats.header_fingerprint_default_count" :total="stats.total" />
              <MetricRow label="User-Agent 默认" :value="stats.user_agent_default_count" :total="stats.total" />
              <MetricRow label="X-Stainless 默认" :value="stats.x_stainless_default_count" :total="stats.total" />
              <MetricRow label="X-App 默认" :value="stats.x_app_default_count" :total="stats.total" />
              <MetricRow label="Direct-Browser-Access 默认" :value="stats.direct_browser_access_default_count" :total="stats.total" />
              <MetricRow label="TLS 默认" :value="stats.tls_default_count" :total="stats.total" />
            </div>
          </section>

          <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-gray-100">敏感字段审计边界</h4>
            <div class="mt-3 space-y-2">
              <MetricRow label="原始遥测字段已记录" :value="stats.raw_values_logged_count" :total="stats.total" />
              <MetricRow label="派生遥测字段已记录" :value="stats.derived_values_logged_count" :total="stats.total" />
              <MetricRow label="认证值被记录" :value="stats.authorization_value_logged_count" :total="stats.total" danger />
              <MetricRow label="token 被记录" :value="stats.token_value_logged_count" :total="stats.total" danger />
              <MetricRow label="模型名被记录" :value="stats.model_value_logged_count" :total="stats.total" danger />
              <MetricRow label="请求正文被记录" :value="stats.request_body_logged_count" :total="stats.total" danger />
            </div>
          </section>
        </div>

        <div class="grid gap-4 lg:grid-cols-2">
          <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-gray-100">身份收敛分析</h4>
            <div class="mt-3 grid grid-cols-2 gap-3">
              <MiniStat label="原始 device_id 去重" :value="stats.unique_raw_device_id_count" />
              <MiniStat label="派生 device_id 去重" :value="stats.unique_derived_device_id_count" />
              <MiniStat label="原始 session_id 去重" :value="stats.unique_raw_session_id_count" />
              <MiniStat label="派生 session_id 去重" :value="stats.unique_derived_session_id_count" />
              <MiniStat label="原始请求 ID 去重" :value="stats.unique_raw_client_request_id_count" />
              <MiniStat label="派生请求 ID 去重" :value="stats.unique_derived_client_request_id_count" />
            </div>
          </section>

          <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-gray-100">端点与处理结果</h4>
            <div class="mt-3 grid gap-3 md:grid-cols-2">
              <BreakdownList title="端点分布" :items="stats.endpoint_breakdown" />
              <BreakdownList title="正文结果" :items="stats.result_breakdown" />
            </div>
          </section>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex items-center justify-end gap-2">
        <a
          v-if="account"
          :href="getTelemetryPrivacyLogURL(account)"
          class="btn btn-secondary"
        >
          查看日志
        </a>
        <button type="button" class="btn btn-primary" @click="handleClose">关闭</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { defineComponent, h, ref, watch, type PropType } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import { opsAPI, type OpsTelemetryPrivacyStatsBreakdownItem, type OpsTelemetryPrivacyStatsResponse } from '@/api/admin/ops'
import type { Account } from '@/types'

type TimeRange = '5m' | '30m' | '1h' | '6h' | '24h' | '7d' | '30d'

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const timeRange = ref<TimeRange>('24h')
const loading = ref(false)
const error = ref('')
const stats = ref<OpsTelemetryPrivacyStatsResponse | null>(null)
const statsRequestSeq = ref(0)

const timeRangeOptions: Array<{ value: TimeRange; label: string }> = [
  { value: '1h', label: '1h' },
  { value: '6h', label: '6h' },
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' }
]

const formatNumber = (value: number | null | undefined) => {
  const n = Number(value ?? 0)
  if (!Number.isFinite(n)) return '0'
  return Math.trunc(n).toLocaleString()
}

const formatRate = (value: number, total: number) => {
  if (!total) return '0%'
  return `${((value / total) * 100).toFixed(1)}%`
}

const formatTimeRange = (start?: string, end?: string) => {
  if (!start || !end) return '最近 24 小时'
  return `${new Date(start).toLocaleString()} - ${new Date(end).toLocaleString()}`
}

const getTelemetryPrivacyProtectedCount = (account: Account | null): number => {
  const count = Number(account?.telemetry_privacy_protected_count ?? 0)
  if (!Number.isFinite(count) || count < 0) return 0
  return Math.trunc(count)
}

const getTelemetryPrivacyLogURL = (account: Account): string => {
  const params = new URLSearchParams({
    platform: 'anthropic',
    system_log_time_range: timeRange.value,
    system_log_component: 'service.gateway.audit.telemetry_privacy',
    system_log_account_id: String(account.id),
    system_log_q: '遥测隐私保护'
  })
  return `/admin/ops?${params.toString()}`
}

// 统计弹窗允许管理员快速切换账号与时间窗口；每次请求记录递增序号，只有最新序号可以回写界面状态。
// 这样可以避免旧请求后返回时覆盖新窗口数据，同时保留 finally 中的 loading 收尾，确保关闭弹窗或切换账号不会遗留加载态。
const loadStats = async () => {
  if (!props.show || !props.account) return
  const requestSeq = statsRequestSeq.value + 1
  statsRequestSeq.value = requestSeq
  loading.value = true
  error.value = ''
  const accountID = props.account.id
  try {
    const nextStats = await opsAPI.getTelemetryPrivacyStats({
      account_id: accountID,
      time_range: timeRange.value
    })
    if (statsRequestSeq.value !== requestSeq) return
    stats.value = nextStats
  } catch (err: any) {
    if (statsRequestSeq.value !== requestSeq) return
    stats.value = null
    error.value = err?.response?.data?.detail || err?.message || '遥测隐私统计加载失败'
  } finally {
    if (statsRequestSeq.value === requestSeq) {
      loading.value = false
    }
  }
}

const setTimeRange = (value: TimeRange) => {
  if (timeRange.value === value) return
  timeRange.value = value
  loadStats()
}

const handleClose = () => {
  emit('close')
}

watch(
  () => [props.show, props.account?.id],
  () => {
    if (props.show) {
      timeRange.value = '24h'
      loadStats()
    } else {
      statsRequestSeq.value += 1
      stats.value = null
      error.value = ''
      loading.value = false
    }
  },
  { immediate: true }
)

const toneClasses = {
  blue: 'border-blue-200 bg-blue-50 text-blue-700 dark:border-blue-800/30 dark:bg-blue-900/15 dark:text-blue-300',
  emerald: 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-800/30 dark:bg-emerald-900/15 dark:text-emerald-300',
  red: 'border-red-200 bg-red-50 text-red-700 dark:border-red-800/30 dark:bg-red-900/15 dark:text-red-300',
  gray: 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-700 dark:bg-dark-700 dark:text-gray-200'
}

const StatCard = defineComponent({
  props: {
    title: { type: String, required: true },
    value: { type: String, required: true },
    sub: { type: String, default: '' },
    tone: { type: String, default: 'gray' }
  },
  setup(componentProps) {
    return () => h('div', {
      class: ['rounded-lg border p-4', toneClasses[componentProps.tone as keyof typeof toneClasses] || toneClasses.gray]
    }, [
      h('div', { class: 'text-xs font-medium opacity-80' }, componentProps.title),
      h('div', { class: 'mt-2 text-2xl font-bold' }, componentProps.value),
      componentProps.sub ? h('div', { class: 'mt-1 text-xs opacity-75' }, componentProps.sub) : null
    ])
  }
})

const MetricRow = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: Number, required: true },
    total: { type: Number, required: true },
    danger: { type: Boolean, default: false }
  },
  setup(componentProps) {
    return () => {
      const rate = componentProps.total ? Math.min(100, Math.max(0, (componentProps.value / componentProps.total) * 100)) : 0
      return h('div', { class: 'space-y-1' }, [
        h('div', { class: 'flex items-center justify-between gap-3 text-xs' }, [
          h('span', { class: 'text-gray-600 dark:text-gray-300' }, componentProps.label),
          h('span', { class: componentProps.danger && componentProps.value > 0 ? 'font-semibold text-red-600 dark:text-red-300' : 'font-semibold text-gray-900 dark:text-gray-100' }, `${formatNumber(componentProps.value)} / ${formatNumber(componentProps.total)}`)
        ]),
        h('div', { class: 'h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700' }, [
          h('div', {
            class: componentProps.danger && componentProps.value > 0 ? 'h-full bg-red-500' : 'h-full bg-emerald-500',
            style: { width: `${rate}%` }
          })
        ])
      ])
    }
  }
})

const MiniStat = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: Number, required: true }
  },
  setup(componentProps) {
    return () => h('div', { class: 'rounded-lg bg-gray-50 p-3 dark:bg-dark-700' }, [
      h('div', { class: 'text-xs text-gray-500 dark:text-gray-400' }, componentProps.label),
      h('div', { class: 'mt-1 text-lg font-semibold text-gray-900 dark:text-gray-100' }, formatNumber(componentProps.value))
    ])
  }
})

const BreakdownList = defineComponent({
  props: {
    title: { type: String, required: true },
    items: { type: Array as PropType<OpsTelemetryPrivacyStatsBreakdownItem[]>, required: true }
  },
  setup(componentProps) {
    return () => h('div', [
      h('div', { class: 'text-xs font-semibold text-gray-600 dark:text-gray-300' }, componentProps.title),
      componentProps.items.length
        ? h('div', { class: 'mt-2 space-y-2' }, componentProps.items.map(item => h('div', { class: 'flex items-start justify-between gap-3 rounded bg-gray-50 px-2 py-1.5 text-xs dark:bg-dark-700' }, [
          h('span', { class: 'break-all text-gray-700 dark:text-gray-200' }, item.label || item.key || '-'),
          h('span', { class: 'font-semibold text-gray-900 dark:text-gray-100' }, formatNumber(item.count))
        ])))
        : h('div', { class: 'mt-2 text-xs text-gray-500 dark:text-gray-400' }, '暂无数据')
    ])
  }
})

</script>
