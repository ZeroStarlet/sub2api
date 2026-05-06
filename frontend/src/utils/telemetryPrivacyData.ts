import type { AdminDataAccount, AdminDataPayload } from '@/types'

const TELEMETRY_PRIVACY_EXPORT_KEYS = [
  'telemetry_privacy_enabled',
  'telemetry_privacy_protected_count',
  'telemetry_privacy_cli_version'
] as const

// 判断账号迁移数据是否属于遥测隐私保护的目标范围。
// 参数 account 只读取 platform/type 两个字段，不读取 credentials、extra 或任何密钥内容；
// 目标范围严格限定为 Anthropic 平台 OAuth / Setup Token 账号，避免导入、导出流程误改
// API Key、Bedrock、Vertex 或其他平台账号的行为。返回 true 只表示该账号具备写入保护开关的资格，
// 不代表账号当前已经开启保护，调用方仍需根据用户界面开关决定是否实际写入 extra 字段。
export const isTelemetryPrivacyDataAccount = (
  account: Pick<AdminDataAccount, 'platform' | 'type'>
) => account.platform === 'anthropic' && (account.type === 'oauth' || account.type === 'setup-token')

const asRecord = (value: unknown): Record<string, unknown> | null => {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>
  }
  return null
}

// 为导入数据中的目标账号补写遥测隐私保护开关。
// 参数 payload 来自用户上传的 JSON，函数会先确认 accounts 是数组再处理；如果上传内容结构异常，
// 会原样返回给后端校验，避免前端把格式错误静默修正成另一份数据。函数只复制被改写的账号与 extra，
// 不修改原始 JSON 对象，便于导入失败后重试，也避免调用方复用同一对象时产生隐藏副作用。
export const withTelemetryPrivacyEnabledForImportedAccounts = (payload: AdminDataPayload) => {
  const source = payload as AdminDataPayload & { accounts?: unknown }
  if (!Array.isArray(source.accounts)) {
    return { payload, count: 0 }
  }

  let count = 0
  const accounts = source.accounts.map((account) => {
    if (!isTelemetryPrivacyDataAccount(account)) {
      return account
    }
    count++
    return {
      ...account,
      extra: {
        ...(asRecord(account.extra) || {}),
        telemetry_privacy_enabled: true
      }
    }
  })

  return {
    payload: {
      ...payload,
      accounts
    },
    count
  }
}

// 从导出数据中移除遥测隐私保护开关与累计保护次数。
// 该函数只处理 extra 中的保护元数据，不触碰 credentials、代理信息、模型映射或其他业务配置；
// 当管理员取消导出隐私保护设置时，迁移文件仍可用于账号迁移，但不会把“是否开启保护”和“已保护次数”
// 带到另一套环境。若某个账号移除后 extra 为空，则删除 extra 字段，保持导出 JSON 简洁。
export const withoutTelemetryPrivacyExportFields = (payload: AdminDataPayload): AdminDataPayload => {
  const source = payload as AdminDataPayload & { accounts?: unknown }
  if (!Array.isArray(source.accounts)) {
    return payload
  }

  const accounts = source.accounts.map((account) => {
    const extra = asRecord(account.extra)
    if (!extra) {
      return account
    }

    const nextExtra = { ...extra }
    for (const key of TELEMETRY_PRIVACY_EXPORT_KEYS) {
      delete nextExtra[key]
    }

    if (Object.keys(nextExtra).length === 0) {
      const nextAccount = { ...account }
      delete nextAccount.extra
      return nextAccount
    }

    return {
      ...account,
      extra: nextExtra
    }
  })

  return {
    ...payload,
    accounts
  }
}
