<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.telemetryPrivacyStats.title')"
    width="full"
    @close="handleClose"
  >
    <div class="space-y-5">
      <!-- 账号信息栏 + 时间范围选择器 -->
      <div
        v-if="account"
        class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-emerald-200/60 bg-gradient-to-r from-emerald-50 to-teal-50 p-4 shadow-sm dark:border-emerald-800/30 dark:from-emerald-900/20 dark:to-teal-900/10"
      >
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-emerald-600 text-white shadow-sm shadow-emerald-600/30">
            <Icon name="shield" size="sm" :stroke-width="2" />
          </div>
          <div>
            <div class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ account.name }}</div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.telemetryPrivacyStats.accountInfo') }} · ID {{ account.id }}
              &nbsp;·&nbsp;{{ t('admin.accounts.telemetryPrivacyStats.protectedCountLabel') }}: {{ formatCount(getTelemetryPrivacyProtectedCount(account)) }}
            </div>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button
            v-for="item in timeRangeOptions"
            :key="item.value"
            type="button"
            :class="[
              'rounded-lg px-3 py-1.5 text-xs font-medium transition-all duration-150',
              timeRange === item.value
                ? 'bg-emerald-600 text-white shadow-sm shadow-emerald-600/25'
                : 'bg-white text-gray-600 hover:bg-gray-100 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600'
            ]"
            @click="setTimeRange(item.value)"
          >
            {{ item.label }}
          </button>
          <span class="mx-1 text-gray-300 dark:text-gray-600">|</span>
          <input
            type="date"
            :value="customStartDate"
            class="rounded-lg border border-gray-200 bg-white px-2 py-1.5 text-xs text-gray-600 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-300"
            :title="'开始日期'"
            @change="onCustomStartChange"
          />
          <span class="text-xs text-gray-400">-</span>
          <input
            type="date"
            :value="customEndDate"
            class="rounded-lg border border-gray-200 bg-white px-2 py-1.5 text-xs text-gray-600 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-300"
            :title="'结束日期'"
            @change="onCustomEndChange"
          />
          <button
            v-if="isCustomRange"
            type="button"
            class="rounded-lg bg-blue-600 px-2 py-1.5 text-xs font-medium text-white hover:bg-blue-700"
            @click="applyCustomRange"
          >
            {{ t('admin.accounts.telemetryPrivacyStats.customRange') }}
          </button>
        </div>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-16">
        <LoadingSpinner />
        <span class="ml-3 text-sm text-gray-500">{{ t('admin.accounts.telemetryPrivacyStats.loading') }}</span>
      </div>

      <div v-else-if="error" class="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800/30 dark:bg-red-900/20 dark:text-red-300">
        {{ error }}
      </div>

      <template v-else-if="stats && stats.total > 0">
        <!-- Tab 导航栏 -->
        <div class="flex gap-1 rounded-xl bg-gray-100 p-1 dark:bg-dark-700">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            type="button"
            :class="[
              'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all duration-150',
              activeTab === tab.key
                ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-gray-100'
                : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
            ]"
            @click="activeTab = tab.key"
          >
            {{ tab.label }}
          </button>
        </div>

        <!-- ==================== 概览 Tab ==================== -->
        <div v-if="activeTab === 'overview'" class="space-y-4">
          <!-- 4 张大号统计卡片 -->
          <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
            <div class="rounded-xl border border-blue-200/60 bg-gradient-to-br from-blue-50 to-indigo-50 p-5 shadow-sm dark:border-blue-800/30 dark:from-blue-900/20 dark:to-indigo-900/10">
              <div class="mb-1 text-xs font-medium text-blue-600 dark:text-blue-400">{{ t('admin.accounts.telemetryPrivacyStats.protectionTotal') }}</div>
              <div class="text-3xl font-bold text-blue-700 dark:text-blue-300">{{ formatCount(stats.total) }}</div>
              <div class="mt-1 text-xs text-blue-500/70 dark:text-blue-400/60">{{ formatTimeRange(stats.start_time, stats.end_time) }}</div>
            </div>
            <div :class="[
              'rounded-xl border p-5 shadow-sm',
              successRate >= 95
                ? 'border-emerald-200/60 bg-gradient-to-br from-emerald-50 to-green-50 dark:border-emerald-800/30 dark:from-emerald-900/20 dark:to-green-900/10'
                : successRate >= 80
                  ? 'border-amber-200/60 bg-gradient-to-br from-amber-50 to-yellow-50 dark:border-amber-800/30 dark:from-amber-900/20 dark:to-yellow-900/10'
                  : 'border-red-200/60 bg-gradient-to-br from-red-50 to-rose-50 dark:border-red-800/30 dark:from-red-900/20 dark:to-rose-900/10'
            ]">
              <div class="mb-1 text-xs font-medium" :class="successRate >= 95 ? 'text-emerald-600 dark:text-emerald-400' : successRate >= 80 ? 'text-amber-600 dark:text-amber-400' : 'text-red-600 dark:text-red-400'">{{ t('admin.accounts.telemetryPrivacyStats.protectionSuccessRate') }}</div>
              <div class="text-3xl font-bold" :class="successRate >= 95 ? 'text-emerald-700 dark:text-emerald-300' : successRate >= 80 ? 'text-amber-700 dark:text-amber-300' : 'text-red-700 dark:text-red-300'">{{ successRate.toFixed(1) }}%</div>
              <div class="mt-1 flex items-center gap-1 text-xs text-gray-500">
                <span class="font-medium text-emerald-600 dark:text-emerald-400">{{ formatCount(stats.success_count) }}</span> {{ t('admin.accounts.telemetryPrivacyStats.pass') }}
                <span v-if="stats.failure_count > 0" class="text-red-500">· {{ formatCount(stats.failure_count) }} {{ t('admin.accounts.telemetryPrivacyStats.fail') }}</span>
              </div>
            </div>
            <div class="rounded-xl border border-purple-200/60 bg-gradient-to-br from-purple-50 to-violet-50 p-5 shadow-sm dark:border-purple-800/30 dark:from-purple-900/20 dark:to-violet-900/10">
              <div class="mb-1 text-xs font-medium text-purple-600 dark:text-purple-400">{{ t('admin.accounts.telemetryPrivacyStats.identityConvergence') }}</div>
              <div class="flex items-baseline gap-2">
                <span class="text-3xl font-bold text-purple-700 dark:text-purple-300">{{ stats.unique_raw_device_id_count || 0 }}</span>
                <span class="text-lg text-purple-400">→</span>
                <span class="text-3xl font-bold text-purple-700 dark:text-purple-300">{{ stats.unique_derived_device_id_count || 0 }}</span>
              </div>
              <div class="mt-1 text-xs text-purple-500/70 dark:text-purple-400/60">Device ID 多端收敛</div>
            </div>
            <div class="rounded-xl border border-teal-200/60 bg-gradient-to-br from-teal-50 to-cyan-50 p-5 shadow-sm dark:border-teal-800/30 dark:from-teal-900/20 dark:to-cyan-900/10">
              <div class="mb-1 text-xs font-medium text-teal-600 dark:text-teal-400">{{ t('admin.accounts.telemetryPrivacyStats.activeWindow') }}</div>
              <div class="text-lg font-bold text-teal-700 dark:text-teal-300">{{ formatTimeRange(stats.start_time, stats.end_time) }}</div>
              <div class="mt-1 flex items-center gap-2 text-xs text-teal-500/70">
                <span class="inline-flex items-center gap-1">
                  <span class="h-1.5 w-1.5 rounded-full bg-teal-500"></span>
                  端点 {{ stats.endpoint_breakdown?.length || 0 }} 类
                </span>
              </div>
            </div>
          </div>

          <!-- 保护量趋势迷你折线图 -->
          <div v-if="stats.time_series?.length" class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.accounts.telemetryPrivacyStats.protectionVolumeTrend') }}</h4>
            <div class="h-56">
              <Line v-if="timeSeriesChartData" :data="timeSeriesChartData" :options="sparklineOptions" />
              <div v-else class="flex h-full items-center justify-center text-sm text-gray-400">暂无图表数据</div>
            </div>
          </div>
        </div>

        <!-- ==================== 保护流水线 Tab ==================== -->
        <div v-else-if="activeTab === 'pipeline'" class="space-y-5">
          <!-- 流水线可视化 -->
          <div class="rounded-xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <h4 class="mb-5 text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.accounts.telemetryPrivacyStats.pipelineTitle') }}</h4>
            <div class="flex flex-wrap items-center gap-3 lg:gap-4">
              <PipelineStep
                :label="t('admin.accounts.telemetryPrivacyStats.pipelineStepInbound')"
                :count="stats.total"
                :total="stats.total"
                tone="slate"
                icon="arrowRight"
              />
              <div class="hidden text-gray-300 dark:text-gray-600 lg:block">
                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
              </div>
              <PipelineStep
                :label="t('admin.accounts.telemetryPrivacyStats.pipelineStepBody')"
                :count="stats.body_pass_count"
                :total="stats.body_protected_count || stats.total"
                :sub="stats.body_rewritten_count"
                sub-label="已改写"
                tone="blue"
                icon="terminal"
              />
              <div class="hidden text-gray-300 dark:text-gray-600 lg:block">
                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
              </div>
              <PipelineStep
                :label="t('admin.accounts.telemetryPrivacyStats.pipelineStepHeader')"
                :count="stats.header_pass_count"
                :total="stats.header_protected_count || stats.total"
                :sub="stats.header_fingerprint_default_count"
                sub-label="指纹默认"
                tone="indigo"
                icon="globe"
              />
              <div class="hidden text-gray-300 dark:text-gray-600 lg:block">
                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
              </div>
              <PipelineStep
                :label="t('admin.accounts.telemetryPrivacyStats.pipelineStepTLS')"
                :count="stats.tls_pass_count"
                :total="stats.total"
                :sub="stats.tls_default_count"
                sub-label="默认指纹"
                tone="teal"
                icon="lock"
              />
              <div class="hidden text-gray-300 dark:text-gray-600 lg:block">
                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
              </div>
              <PipelineStep
                :label="t('admin.accounts.telemetryPrivacyStats.pipelineStepUpstream')"
                :count="stats.success_count"
                :total="stats.total"
                tone="emerald"
                icon="check"
              />
            </div>
            <!-- 整体成功率条 -->
            <div class="mt-5">
              <div class="mb-1 flex justify-between text-xs text-gray-500 dark:text-gray-400">
                <span>端到端通过率</span>
                <span class="font-semibold" :class="successRate >= 95 ? 'text-emerald-600' : 'text-amber-600'">{{ successRate.toFixed(1) }}%</span>
              </div>
              <div class="h-2.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div
                  class="h-full rounded-full transition-all duration-500"
                  :class="successRate >= 95 ? 'bg-emerald-500' : successRate >= 80 ? 'bg-amber-500' : 'bg-red-500'"
                  :style="{ width: `${Math.min(100, Math.max(0, successRate))}%` }"
                />
              </div>
            </div>
          </div>

          <!-- 详细校验指标 -->
          <div class="grid gap-4 lg:grid-cols-3">
            <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <h5 class="mb-3 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.sectionBodyDetail') }}</h5>
              <div class="space-y-2.5">
                <MetricBar label="隐私校验通过" :value="stats.body_pass_count" :total="stats.total" />
                <MetricBar label="metadata.user_id 已改写" :value="stats.body_rewritten_count" :total="stats.total" />
                <MetricBar label="metadata 存在" :value="stats.metadata_present_count" :total="stats.total" tone="neutral" />
                <MetricBar label="metadata 安全缺失" :value="stats.metadata_absent_safe_count" :total="stats.total" tone="neutral" />
              </div>
            </div>
            <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <h5 class="mb-3 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.sectionHeaderDetail') }}</h5>
              <div class="space-y-2.5">
                <MetricBar label="Header 校验通过" :value="stats.header_pass_count" :total="stats.total" />
                <MetricBar label="指纹已收敛为默认" :value="stats.header_fingerprint_default_count" :total="stats.total" />
                <MetricBar label="User-Agent 默认" :value="stats.user_agent_default_count" :total="stats.total" />
                <MetricBar label="X-Stainless 默认" :value="stats.x_stainless_default_count" :total="stats.total" />
                <MetricBar label="X-App 默认" :value="stats.x_app_default_count" :total="stats.total" />
                <MetricBar label="会话头已保护" :value="stats.session_header_protected_count" :total="stats.total" />
              </div>
            </div>
            <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <h5 class="mb-3 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.sectionTLSDetail') }}</h5>
              <div class="space-y-2.5">
                <MetricBar label="TLS 校验通过" :value="stats.tls_pass_count" :total="stats.total" />
                <MetricBar label="TLS 指纹默认" :value="stats.tls_default_count" :total="stats.total" />
                <MetricBar label="请求 ID 已重置" :value="stats.client_request_id_reset_count" :total="stats.total" />
                <MetricBar label="Direct-Browser-Access 默认" :value="stats.direct_browser_access_default_count" :total="stats.total" tone="neutral" />
              </div>
            </div>
          </div>

          <!-- 敏感字段审计边界 -->
          <div class="rounded-xl border border-amber-200/60 bg-amber-50/50 p-4 shadow-sm dark:border-amber-800/30 dark:bg-amber-900/10">
            <h5 class="mb-3 text-xs font-semibold uppercase tracking-wide text-amber-700 dark:text-amber-400">{{ t('admin.accounts.telemetryPrivacyStats.sectionSensitiveAudit') }}</h5>
            <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
              <div class="flex items-center justify-between rounded-lg bg-white/70 p-2.5 dark:bg-dark-800/70">
                <span class="text-xs text-gray-600 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.metricAuthValue') }}</span>
                <span :class="stats.authorization_value_logged_count > 0 ? 'font-bold text-red-600' : 'font-bold text-emerald-600'">
                  {{ stats.authorization_value_logged_count > 0 ? '⚠ ' : '✓ ' }}{{ formatCount(stats.authorization_value_logged_count) }}
                </span>
              </div>
              <div class="flex items-center justify-between rounded-lg bg-white/70 p-2.5 dark:bg-dark-800/70">
                <span class="text-xs text-gray-600 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.metricToken') }}</span>
                <span :class="stats.token_value_logged_count > 0 ? 'font-bold text-red-600' : 'font-bold text-emerald-600'">
                  {{ stats.token_value_logged_count > 0 ? '⚠ ' : '✓ ' }}{{ formatCount(stats.token_value_logged_count) }}
                </span>
              </div>
              <div class="flex items-center justify-between rounded-lg bg-white/70 p-2.5 dark:bg-dark-800/70">
                <span class="text-xs text-gray-600 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.metricModel') }}</span>
                <span :class="stats.model_value_logged_count > 0 ? 'font-bold text-red-600' : 'font-bold text-emerald-600'">
                  {{ stats.model_value_logged_count > 0 ? '⚠ ' : '✓ ' }}{{ formatCount(stats.model_value_logged_count) }}
                </span>
              </div>
              <div class="flex items-center justify-between rounded-lg bg-white/70 p-2.5 dark:bg-dark-800/70">
                <span class="text-xs text-gray-600 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.metricRequestBody') }}</span>
                <span :class="stats.request_body_logged_count > 0 ? 'font-bold text-red-600' : 'font-bold text-emerald-600'">
                  {{ stats.request_body_logged_count > 0 ? '⚠ ' : '✓ ' }}{{ formatCount(stats.request_body_logged_count) }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <!-- ==================== 身份收敛 Tab ==================== -->
        <div v-else-if="activeTab === 'identity'" class="space-y-4">
          <div class="grid gap-4 lg:grid-cols-3">
            <!-- Device ID 收敛 -->
            <div class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <h5 class="mb-4 text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.accounts.telemetryPrivacyStats.identityDeviceID') }}</h5>
              <div class="flex items-stretch gap-3">
                <div class="flex-1 rounded-lg bg-red-50 p-3 text-center dark:bg-red-900/15">
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.identityRawLabel') }}</div>
                  <div class="mt-1.5 text-2xl font-bold text-red-600 dark:text-red-400">{{ stats.unique_raw_device_id_count || 0 }}</div>
                  <div class="mt-0.5 text-[10px] text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.uniqueValues', { count: stats.unique_raw_device_id_count || 0 }) }}</div>
                </div>
                <div class="flex items-center justify-center">
                  <svg class="h-6 w-6 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
                </div>
                <div class="flex-1 rounded-lg bg-emerald-50 p-3 text-center dark:bg-emerald-900/15">
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.identityDerivedLabel') }}</div>
                  <div class="mt-1.5 text-2xl font-bold text-emerald-600 dark:text-emerald-400">{{ stats.unique_derived_device_id_count || 0 }}</div>
                  <div class="mt-0.5 text-[10px] text-emerald-500">{{ stats.unique_derived_device_id_count <= 1 ? t('admin.accounts.telemetryPrivacyStats.convergedToUnified') : t('admin.accounts.telemetryPrivacyStats.uniqueValues', { count: stats.unique_derived_device_id_count || 0 }) }}</div>
                </div>
              </div>
              <div v-if="stats.unique_raw_device_id_count > 0 && stats.unique_derived_device_id_count > 0" class="mt-3">
                <div class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                  <span>收敛率</span>
                  <span class="font-semibold text-purple-600 dark:text-purple-400">
                    {{ ((1 - stats.unique_derived_device_id_count / Math.max(1, stats.unique_raw_device_id_count)) * 100).toFixed(1) }}%
                  </span>
                </div>
                <div class="mt-1 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                  <div class="h-full rounded-full bg-purple-500" :style="{ width: `${Math.min(100, (1 - stats.unique_derived_device_id_count / Math.max(1, stats.unique_raw_device_id_count)) * 100)}%` }" />
                </div>
              </div>
            </div>

            <!-- Session ID 收敛 -->
            <div class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <h5 class="mb-4 text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.accounts.telemetryPrivacyStats.identitySessionID') }}</h5>
              <div class="flex items-stretch gap-3">
                <div class="flex-1 rounded-lg bg-red-50 p-3 text-center dark:bg-red-900/15">
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.identityRawLabel') }}</div>
                  <div class="mt-1.5 text-2xl font-bold text-red-600 dark:text-red-400">{{ stats.unique_raw_session_id_count || 0 }}</div>
                  <div class="mt-0.5 text-[10px] text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.uniqueValues', { count: stats.unique_raw_session_id_count || 0 }) }}</div>
                </div>
                <div class="flex items-center justify-center">
                  <svg class="h-6 w-6 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
                </div>
                <div class="flex-1 rounded-lg bg-emerald-50 p-3 text-center dark:bg-emerald-900/15">
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.identityDerivedLabel') }}</div>
                  <div class="mt-1.5 text-2xl font-bold text-emerald-600 dark:text-emerald-400">{{ stats.unique_derived_session_id_count || 0 }}</div>
                  <div class="mt-0.5 text-[10px] text-emerald-500">{{ stats.unique_derived_session_id_count <= 1 ? t('admin.accounts.telemetryPrivacyStats.convergedToUnified') : t('admin.accounts.telemetryPrivacyStats.uniqueValues', { count: stats.unique_derived_session_id_count || 0 }) }}</div>
                </div>
              </div>
              <div v-if="stats.unique_raw_session_id_count > 0 && stats.unique_derived_session_id_count > 0" class="mt-3">
                <div class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                  <span>收敛率</span>
                  <span class="font-semibold text-purple-600 dark:text-purple-400">
                    {{ ((1 - stats.unique_derived_session_id_count / Math.max(1, stats.unique_raw_session_id_count)) * 100).toFixed(1) }}%
                  </span>
                </div>
                <div class="mt-1 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                  <div class="h-full rounded-full bg-purple-500" :style="{ width: `${Math.min(100, (1 - stats.unique_derived_session_id_count / Math.max(1, stats.unique_raw_session_id_count)) * 100)}%` }" />
                </div>
              </div>
            </div>

            <!-- Request ID -->
            <div class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <h5 class="mb-4 text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.accounts.telemetryPrivacyStats.identityRequestID') }}</h5>
              <div class="flex items-stretch gap-3">
                <div class="flex-1 rounded-lg bg-blue-50 p-3 text-center dark:bg-blue-900/15">
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.identityRawLabel') }}</div>
                  <div class="mt-1.5 text-2xl font-bold text-blue-600 dark:text-blue-400">{{ stats.unique_raw_client_request_id_count || 0 }}</div>
                  <div class="mt-0.5 text-[10px] text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.uniqueValues', { count: stats.unique_raw_client_request_id_count || 0 }) }}</div>
                </div>
                <div class="flex items-center justify-center">
                  <svg class="h-6 w-6 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
                </div>
                <div class="flex-1 rounded-lg bg-blue-50 p-3 text-center dark:bg-blue-900/15">
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.identityDerivedLabel') }}</div>
                  <div class="mt-1.5 text-2xl font-bold text-blue-600 dark:text-blue-400">{{ stats.unique_derived_client_request_id_count || 0 }}</div>
                  <div class="mt-0.5 text-[10px] text-blue-500">{{ t('admin.accounts.telemetryPrivacyStats.regeneratedPerRequest') }}</div>
                </div>
              </div>
              <div class="mt-3 rounded-lg bg-gray-50 p-2.5 text-center text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                {{ t('admin.accounts.telemetryPrivacyStats.requestIDNoConvergence') }}
              </div>
            </div>
          </div>
        </div>

        <!-- ==================== 趋势分析 Tab ==================== -->
        <div v-else-if="activeTab === 'trends'" class="space-y-4">
          <div v-if="stats.time_series?.length" class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <h4 class="mb-4 text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.accounts.telemetryPrivacyStats.trendsTitle') }}</h4>
            <div class="h-72">
              <Line v-if="timeSeriesChartData" :data="timeSeriesChartData" :options="fullChartOptions" />
              <div v-else class="flex h-full items-center justify-center text-sm text-gray-400">暂无趋势数据</div>
            </div>
          </div>

          <div v-else class="rounded-xl border border-gray-200 bg-white p-8 text-center shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="text-sm text-gray-500">{{ t('admin.accounts.telemetryPrivacyStats.noData') }}</div>
          </div>

          <div class="grid gap-4 lg:grid-cols-2">
            <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <h5 class="mb-3 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.endpointDist') }}</h5>
              <div class="space-y-2">
                <template v-if="stats.endpoint_breakdown?.length">
                  <div v-for="item in stats.endpoint_breakdown" :key="item.key" class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-700">
                    <span class="text-sm text-gray-700 dark:text-gray-200">{{ item.label || item.key || '-' }}</span>
                    <div class="flex items-center gap-2">
                      <div class="h-1.5 w-16 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                        <div class="h-full rounded-full bg-blue-500" :style="{ width: `${stats.total > 0 ? (item.count / stats.total) * 100 : 0}%` }" />
                      </div>
                      <span class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ formatCount(item.count) }}</span>
                    </div>
                  </div>
                </template>
                <div v-else class="text-xs text-gray-400">暂无数据</div>
              </div>
            </div>
            <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <h5 class="mb-3 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.resultDist') }}</h5>
              <div class="space-y-2">
                <template v-if="stats.result_breakdown?.length">
                  <div v-for="item in stats.result_breakdown" :key="item.key" class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-700">
                    <span class="max-w-[280px] truncate text-sm text-gray-700 dark:text-gray-200">{{ item.label || item.key || '-' }}</span>
                    <div class="flex items-center gap-2">
                      <div class="h-1.5 w-16 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                        <div class="h-full rounded-full bg-emerald-500" :style="{ width: `${stats.total > 0 ? (item.count / stats.total) * 100 : 0}%` }" />
                      </div>
                      <span class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ formatCount(item.count) }}</span>
                    </div>
                  </div>
                </template>
                <div v-else class="text-xs text-gray-400">暂无数据</div>
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- 空数据状态 -->
      <div v-else-if="stats && stats.total === 0" class="flex flex-col items-center justify-center py-16 text-center">
        <div class="mb-3 flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700">
          <Icon name="shield" size="lg" :stroke-width="1.5" class="text-gray-400" />
        </div>
        <div class="text-sm font-medium text-gray-600 dark:text-gray-300">{{ t('admin.accounts.telemetryPrivacyStats.noData') }}</div>
        <div class="mt-1 text-xs text-gray-400">{{ formatTimeRange(stats.start_time, stats.end_time) }}</div>
      </div>
    </div>

    <template #footer>
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-2">
          <a
            v-if="account"
            :href="getTelemetryPrivacyLogURL(account)"
            class="btn btn-secondary text-xs"
          >
            <Icon name="document" size="xs" :stroke-width="2" />
            {{ t('admin.accounts.telemetryPrivacyStats.viewLogs') }}
          </a>
          <button
            v-if="stats && stats.total > 0"
            type="button"
            class="btn btn-secondary text-xs"
            @click="exportStats"
          >
            {{ t('admin.accounts.telemetryPrivacyStats.exportStatsBtn') }}
          </button>
        </div>
        <div class="flex items-center gap-2">
          <span v-if="!stats" class="text-xs text-gray-400">{{ t('admin.accounts.telemetryPrivacyStats.selectAccountPrompt') }}</span>
          <button type="button" class="btn btn-primary" @click="handleClose">{{ t('admin.accounts.telemetryPrivacyStats.closeBtn') }}</button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, CategoryScale, Filler, Legend, LineElement, LinearScale, PointElement, Title, Tooltip } from 'chart.js'
import { Line } from 'vue-chartjs'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import { opsAPI, type OpsTelemetryPrivacyStatsResponse } from '@/api/admin/ops'
import type { Account } from '@/types'

ChartJS.register(Title, Tooltip, Legend, LineElement, LinearScale, PointElement, CategoryScale, Filler)

type TimeRange = '1h' | '6h' | '24h' | '7d' | '30d' | 'custom'
type TabKey = 'overview' | 'pipeline' | 'identity' | 'trends'

const { t } = useI18n()

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const timeRange = ref<TimeRange>('24h')
const activeTab = ref<TabKey>('overview')
const loading = ref(false)
const error = ref('')
const stats = ref<OpsTelemetryPrivacyStatsResponse | null>(null)
const statsRequestSeq = ref(0)

// 自定义日期范围
const customStartDate = ref('')
const customEndDate = ref('')
const customStartTime = ref('')
const customEndTime = ref('')
const isCustomRange = computed(() => timeRange.value === 'custom')

const timeRangeOptions: Array<{ value: TimeRange; label: string }> = [
  { value: '1h', label: '1h' },
  { value: '6h', label: '6h' },
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' },
  { value: 'custom', label: t('admin.accounts.telemetryPrivacyStats.customRange') }
]

const tabs: Array<{ key: TabKey; label: string }> = [
  { key: 'overview', label: t('admin.accounts.telemetryPrivacyStats.tabOverview') },
  { key: 'pipeline', label: t('admin.accounts.telemetryPrivacyStats.tabPipeline') },
  { key: 'identity', label: t('admin.accounts.telemetryPrivacyStats.tabIdentity') },
  { key: 'trends', label: t('admin.accounts.telemetryPrivacyStats.tabTrends') }
]

// === 基础工具函数 ===

const formatCount = (value: number | null | undefined): string => {
  const n = Number(value ?? 0)
  if (!Number.isFinite(n)) return '0'
  const v = Math.trunc(n)
  if (v >= 10000) return `${(v / 10000).toFixed(1)}${t('admin.accounts.telemetryPrivacyStats.protectedCountUnit')}`
  if (v >= 1000) return `${(v / 1000).toFixed(1)}${t('admin.accounts.telemetryPrivacyStats.protectedCountUnitK')}`
  return v.toLocaleString()
}

const formatTimeRange = (start?: string, end?: string) => {
  if (!start || !end) return ''
  return `${new Date(start).toLocaleDateString()} ${new Date(start).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} - ${new Date(end).toLocaleDateString()} ${new Date(end).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`
}

const getTelemetryPrivacyProtectedCount = (account: Account | null): number => {
  const count = Number(account?.telemetry_privacy_protected_count ?? 0)
  if (!Number.isFinite(count) || count < 0) return 0
  return Math.trunc(count)
}

const getTelemetryPrivacyLogURL = (account: Account): string => {
  const params = new URLSearchParams({
    platform: 'anthropic',
    system_log_time_range: timeRange.value === 'custom' ? '24h' : timeRange.value,
    system_log_component: 'service.gateway.audit.telemetry_privacy',
    system_log_account_id: String(account.id),
    system_log_q: '遥测隐私保护'
  })
  return `/admin/ops?${params.toString()}`
}

// === 成功率计算 ===

const successRate = computed(() => {
  if (!stats.value || !stats.value.total) return 0
  return (stats.value.success_count / stats.value.total) * 100
})

// === 自定义日期范围 ===

function onCustomStartChange(e: Event) {
  const input = e.target as HTMLInputElement
  customStartDate.value = input.value
  if (input.value) {
    timeRange.value = 'custom'
  }
}

function onCustomEndChange(e: Event) {
  const input = e.target as HTMLInputElement
  customEndDate.value = input.value
  if (input.value) {
    timeRange.value = 'custom'
  }
}

function applyCustomRange() {
  if (!customStartDate.value || !customEndDate.value) return
  customStartTime.value = new Date(customStartDate.value).toISOString()
  customEndTime.value = new Date(new Date(customEndDate.value).getTime() + 86400000 - 1).toISOString()
  timeRange.value = 'custom'
  loadStats()
}

// === 暗色模式 ===

const isDarkMode = computed(() => {
  if (typeof document !== 'undefined') {
    return document.documentElement.classList.contains('dark')
  }
  return false
})

const chartColors = computed(() => ({
  blue: '#3b82f6',
  blueAlpha: 'rgba(59,130,246,0.15)',
  emerald: '#10b981',
  emeraldAlpha: 'rgba(16,185,129,0.15)',
  red: '#ef4444',
  redAlpha: 'rgba(239,68,68,0.12)',
  grid: isDarkMode.value ? '#374151' : '#f3f4f6',
  text: isDarkMode.value ? '#9ca3af' : '#6b7280'
}))

// === Chart.js 时序数据 ===

const timeSeriesChartData = computed(() => {
  if (!stats.value?.time_series?.length) return null
  const points = stats.value.time_series
  return {
    labels: points.map(p => {
      const d = new Date(p.bucket_start)
      const windowHours = stats.value ? (new Date(stats.value.end_time).getTime() - new Date(stats.value.start_time).getTime()) / 3600000 : 24
      if (windowHours <= 24) {
        return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
      }
      return d.toLocaleDateString([], { month: 'short', day: 'numeric' })
    }),
    datasets: [
      {
        label: t('admin.accounts.telemetryPrivacyStats.trendsVolume'),
        data: points.map(p => p.total),
        borderColor: chartColors.value.blue,
        backgroundColor: chartColors.value.blueAlpha,
        fill: true,
        tension: 0.4,
        pointRadius: 0,
        pointHitRadius: 10
      },
      {
        label: t('admin.accounts.telemetryPrivacyStats.trendsSuccess'),
        data: points.map(p => p.success_count),
        borderColor: chartColors.value.emerald,
        backgroundColor: chartColors.value.emeraldAlpha,
        fill: true,
        tension: 0.4,
        pointRadius: 0,
        pointHitRadius: 10
      },
      {
        label: t('admin.accounts.telemetryPrivacyStats.trendsFailure'),
        data: points.map(p => p.failure_count),
        borderColor: chartColors.value.red,
        backgroundColor: chartColors.value.redAlpha,
        fill: true,
        tension: 0.4,
        pointRadius: 0,
        pointHitRadius: 10,
        hidden: true
      }
    ]
  }
})

const baseChartOptions = computed(() => {
  const c = chartColors.value
  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { intersect: false, mode: 'index' as const },
    plugins: {
      legend: {
        position: 'top' as const,
        align: 'end' as const,
        labels: { color: c.text, usePointStyle: true, boxWidth: 6, font: { size: 10 } }
      },
      tooltip: {
        backgroundColor: isDarkMode.value ? '#1f2937' : '#ffffff',
        titleColor: isDarkMode.value ? '#f3f4f6' : '#111827',
        bodyColor: isDarkMode.value ? '#d1d5db' : '#4b5563',
        borderColor: c.grid,
        borderWidth: 1,
        padding: 10
      }
    },
    scales: {
      x: {
        type: 'category' as const,
        grid: { display: false },
        ticks: { color: c.text, font: { size: 10 }, maxTicksLimit: 8, autoSkip: true, autoSkipPadding: 10 }
      },
      y: {
        type: 'linear' as const,
        display: true,
        position: 'left' as const,
        grid: { color: c.grid, borderDash: [4, 4] },
        ticks: { color: c.text, font: { size: 10 } },
        beginAtZero: true
      }
    }
  }
})

const sparklineOptions = computed(() => ({
  ...baseChartOptions.value,
  plugins: {
    ...baseChartOptions.value.plugins,
    legend: { ...baseChartOptions.value.plugins.legend, display: true }
  }
}))

const fullChartOptions = computed(() => ({
  ...baseChartOptions.value,
  plugins: {
    ...baseChartOptions.value.plugins,
    legend: { ...baseChartOptions.value.plugins.legend, display: true }
  }
}))

// === 数据加载 ===

// 统计弹窗允许管理员快速切换账号与时间窗口；每次请求记录递增序号，只有最新序号可以回写界面状态。
const loadStats = async () => {
  if (!props.show || !props.account) return
  const requestSeq = statsRequestSeq.value + 1
  statsRequestSeq.value = requestSeq
  loading.value = true
  error.value = ''
  const accountID = props.account.id
  try {
    const query: Record<string, unknown> = { account_id: accountID }
    if (timeRange.value === 'custom' && customStartTime.value && customEndTime.value) {
      query.start_time = customStartTime.value
      query.end_time = customEndTime.value
    } else {
      query.time_range = timeRange.value
    }
    const nextStats = await opsAPI.getTelemetryPrivacyStats(query as any)
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
  if (value !== 'custom') {
    customStartDate.value = ''
    customEndDate.value = ''
    customStartTime.value = ''
    customEndTime.value = ''
    loadStats()
  }
  // 切换到自定义模式时不自动加载，等待用户选择日期后通过 applyCustomRange 加载
}

const handleClose = () => {
  emit('close')
}

// 导出统计为 JSON 文件
const exportStats = () => {
  if (!stats.value) return
  const data = JSON.stringify(stats.value, null, 2)
  const blob = new Blob([data], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `telemetry-privacy-stats-${stats.value.account_id}-${new Date().toISOString().slice(0, 10)}.json`
  a.click()
  URL.revokeObjectURL(url)
}

watch(
  () => [props.show, props.account?.id],
  () => {
    if (props.show) {
      timeRange.value = '24h'
      customStartDate.value = ''
      customEndDate.value = ''
      customStartTime.value = ''
      customEndTime.value = ''
      activeTab.value = 'overview'
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

// === 内联子组件 ===

const pipelineToneClasses: Record<string, string> = {
  slate: 'border-slate-200 bg-slate-50 text-slate-700 dark:border-slate-700 dark:bg-slate-900/30 dark:text-slate-300',
  blue: 'border-blue-200 bg-blue-50 text-blue-700 dark:border-blue-800/30 dark:bg-blue-900/20 dark:text-blue-300',
  indigo: 'border-indigo-200 bg-indigo-50 text-indigo-700 dark:border-indigo-800/30 dark:bg-indigo-900/20 dark:text-indigo-300',
  teal: 'border-teal-200 bg-teal-50 text-teal-700 dark:border-teal-800/30 dark:bg-teal-900/20 dark:text-teal-300',
  emerald: 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-800/30 dark:bg-emerald-900/20 dark:text-emerald-300'
}

const PipelineStep = defineComponent({
  props: {
    label: { type: String, required: true },
    count: { type: Number, required: true },
    total: { type: Number, required: true },
    sub: { type: Number, default: 0 },
    subLabel: { type: String, default: '' },
    tone: { type: String, default: 'slate' },
    icon: { type: String, default: '' }
  },
  setup(props) {
    return () => {
      const rate = props.total > 0 ? Math.min(100, Math.max(0, (props.count / props.total) * 100)) : 0
      const toneClass = pipelineToneClasses[props.tone] || pipelineToneClasses.slate
      return h('div', {
        class: ['flex min-w-[140px] flex-1 flex-col rounded-xl border p-4 shadow-sm', toneClass]
      }, [
        h('div', { class: 'mb-2 flex items-center gap-1.5' }, [
          props.icon ? h(Icon, { name: props.icon as any, size: 'xs', 'stroke-width': 2, class: 'opacity-70' }) : null,
          h('span', { class: 'text-xs font-semibold' }, props.label)
        ]),
        h('div', { class: 'flex items-baseline gap-1' }, [
          h('span', { class: 'text-2xl font-bold' }, formatCount(props.count)),
          h('span', { class: 'text-xs opacity-60' }, `/ ${formatCount(props.total)}`)
        ]),
        h('div', { class: 'mt-2 h-1.5 overflow-hidden rounded-full bg-black/10 dark:bg-white/10' }, [
          h('div', {
            class: rate >= 95 ? 'h-full rounded-full bg-emerald-500' : rate >= 80 ? 'h-full rounded-full bg-amber-500' : 'h-full rounded-full bg-red-500',
            style: { width: `${rate}%` }
          })
        ]),
        props.sub > 0 && props.subLabel
          ? h('div', { class: 'mt-1.5 text-[10px] opacity-70' }, `${props.subLabel}: ${formatCount(props.sub)}`)
          : null
      ])
    }
  }
})

const MetricBar = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: Number, required: true },
    total: { type: Number, required: true },
    tone: { type: String, default: 'emerald' }
  },
  setup(props) {
    return () => {
      const rate = props.total > 0 ? Math.min(100, Math.max(0, (props.value / props.total) * 100)) : 0
      const barColor = props.tone === 'neutral' ? 'bg-blue-400' : 'bg-emerald-500'
      return h('div', { class: 'space-y-1' }, [
        h('div', { class: 'flex items-center justify-between gap-2 text-xs' }, [
          h('span', { class: 'text-gray-600 dark:text-gray-300' }, props.label),
          h('span', { class: 'font-semibold text-gray-900 dark:text-gray-100' }, `${formatCount(props.value)} / ${formatCount(props.total)}`)
        ]),
        h('div', { class: 'h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700' }, [
          h('div', { class: `h-full rounded-full ${barColor}`, style: { width: `${rate}%` } })
        ])
      ])
    }
  }
})

</script>
