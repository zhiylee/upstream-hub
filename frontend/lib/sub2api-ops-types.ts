export type AccountPlatform = "anthropic" | "openai" | "gemini" | "antigravity" | "grok"
export type AccountType = "oauth" | "setup-token" | "apikey" | "upstream" | "bedrock" | "service_account"
export type AccountStatus = "active" | "inactive" | "error" | "rate_limited" | "temp_unschedulable" | "unschedulable"
export type OpsTimeRange = "5m" | "30m" | "1h" | "6h" | "24h" | "7d" | "30d"
export type OpsWidgetWidth = 4 | 6 | 8 | 12
export type OpsWidgetHeight = "compact" | "standard" | "tall"
export type OpsWidgetType = "group_metrics" | "accounts"

export interface Sub2APIOpsConfig {
  configured: boolean
  name: string
  site_url: string
  admin_key_configured: boolean
  layout?: OpsDashboardLayout
  updated_at?: string
}

export interface OpsDashboardLayout {
  version: 1
  widgets: OpsDashboardWidget[]
}

export interface OpsDashboardWidget<T = unknown> {
  id: string
  type: string
  width: OpsWidgetWidth
  height: OpsWidgetHeight
  config: T
}

export interface GroupMetricsWidgetConfig {
  title: string
  platform: AccountPlatform | ""
  groupIds: number[]
  timeRange: OpsTimeRange
  refreshSeconds: number
  slaTarget: number
  ttftP95TargetMs: number
  trendMetric: "requests" | "tokens"
  recentAccountLimit: number
}

export interface AccountsWidgetConfig {
  title: string
  platform: AccountPlatform | ""
  accountType: AccountType | ""
  statuses: AccountStatus[]
  excludedStatuses: AccountStatus[]
  groups: string[]
  pageSize: number
  refreshSeconds: number
  showUsage: boolean
  showSchedulerScore: boolean
  sortBy: "id" | "name" | "status" | "schedulable" | "priority" | "rate_multiplier" | "concurrency" | "current_concurrency" | "last_used_at" | "created_at" | "expires_at"
  sortOrder: "asc" | "desc"
}

export interface OpsRate {
  current: number
  peak: number
  avg: number
}

export interface OpsPercentiles {
  p50_ms: number | null
  p90_ms: number | null
  p95_ms: number | null
  p99_ms: number | null
  avg_ms: number | null
  max_ms: number | null
}

export interface OpsOverview {
  start_time: string
  end_time: string
  platform: string
  group_id: number | null
  health_score: number
  success_count: number
  error_count_total: number
  business_limited_count: number
  error_count_sla: number
  request_count_total: number
  request_count_sla: number
  token_consumed: number
  sla: number
  error_rate: number
  upstream_error_rate: number
  qps: OpsRate
  tps: OpsRate
  duration: OpsPercentiles
  ttft: OpsPercentiles
}

export interface OpsThroughputPoint {
  bucket_start: string
  request_count: number
  token_consumed: number
  switch_count: number
  qps: number
  tps: number
}

export interface OpsSnapshot {
  generated_at: string
  overview: OpsOverview | null
  throughput_trend: {
    bucket: string
    points: OpsThroughputPoint[] | null
  } | null
  error_trend: {
    bucket: string
    points: Array<{
      bucket_start: string
      error_count_total: number
      business_limited_count: number
      error_count_sla: number
    }> | null
  } | null
  aggregation?: {
    group_ids: number[]
    percentile_mode: "worst_group"
  }
}

export interface OpsGroup {
  id: number
  name: string
  description?: string
  platform: AccountPlatform
  status: string
  sort_order: number
  account_count?: number
  active_account_count?: number
  rate_limited_account_count?: number
}

export interface OpsRecentAccount {
  id: number
  name: string
  platform: AccountPlatform
  group_ids: number[]
  last_used_at: string
}

export interface OpsRecentAccountsResponse {
  accounts: OpsRecentAccount[]
  limit: number
  start_time: string
  end_time: string
  generated_at: string
  truncated: boolean
}

export interface SchedulerScore {
  base_score: number
  sticky_score: number
  sticky_score_infinity: boolean
  sticky_weighted_enabled: boolean
}

export interface SchedulerGroupScore extends SchedulerScore {
  group_id: number | null
  group_name?: string
  group_priority?: number
}

export interface OpsAccount {
  id: number
  name: string
  notes: string | null
  platform: AccountPlatform
  type: AccountType
  status: "active" | "inactive" | "error"
  schedulable: boolean
  concurrency: number
  load_factor?: number | null
  current_concurrency: number
  priority: number
  rate_multiplier: number
  last_used_at: string | null
  expires_at: number | null
  auto_pause_on_expired: boolean
  created_at: string
  updated_at: string
  rate_limited_at: string | null
  rate_limit_reset_at: string | null
  overload_until: string | null
  temp_unschedulable_until: string | null
  group_ids?: number[]
  groups?: OpsGroup[]
  scheduler_score?: SchedulerScore | null
  scheduler_scores?: SchedulerGroupScore[] | null
  current_window_cost?: number
  active_sessions?: number
  current_rpm?: number
  quota_limit?: number
  quota_used?: number
  quota_daily_limit?: number
  quota_daily_used?: number
  quota_weekly_limit?: number
  quota_weekly_used?: number
  usage_windows?: OpsAccountUsageWindows
}

export interface OpsUsageWindow {
  label: string
  utilization: number
  window_minutes?: number | null
  resets_at?: string | null
}

export interface OpsAccountUsageWindows {
  items: OpsUsageWindow[]
  updated_at?: string | null
}

export interface OpsAccountPage {
  items: OpsAccount[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface AccountTodayStats {
  requests: number
  tokens: number
  cost: number
  standard_cost: number
  user_cost: number
}

export interface AccountTodayStatsResponse {
  stats: Record<string, AccountTodayStats>
}

export interface AccountUsageDetail {
  total?: AccountTodayStats
  window_stats?: Record<string, AccountTodayStats>
}

export interface AccountUsageDetailsResponse {
  details: Record<string, AccountUsageDetail>
  failed_total_ids: number[]
  failed_window_ids: number[]
}

export const DEFAULT_GROUP_METRICS_CONFIG: GroupMetricsWidgetConfig = {
  title: "核心服务指标",
  platform: "",
  groupIds: [],
  timeRange: "1h",
  refreshSeconds: 30,
  slaTarget: 99.9,
  ttftP95TargetMs: 2000,
  trendMetric: "requests",
  recentAccountLimit: 5,
}

export const DEFAULT_ACCOUNTS_CONFIG: AccountsWidgetConfig = {
  title: "账号运行状态",
  platform: "",
  accountType: "",
  statuses: [],
  excludedStatuses: [],
  groups: [],
  pageSize: 20,
  refreshSeconds: 60,
  showUsage: true,
  showSchedulerScore: false,
  sortBy: "name",
  sortOrder: "asc",
}

export function createDefaultOpsLayout(): OpsDashboardLayout {
  return {
    version: 1,
    widgets: [
      {
        id: "group_overview",
        type: "group_metrics",
        width: 6,
        height: "standard",
        config: { ...DEFAULT_GROUP_METRICS_CONFIG },
      },
      {
        id: "account_health",
        type: "accounts",
        width: 12,
        height: "tall",
        config: { ...DEFAULT_ACCOUNTS_CONFIG },
      },
    ],
  }
}

export function createOpsWidget(type: OpsWidgetType): OpsDashboardWidget {
  const suffix = `${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`
  if (type === "group_metrics") {
    return {
      id: `group_${suffix}`,
      type,
      width: 6,
      height: "standard",
      config: { ...DEFAULT_GROUP_METRICS_CONFIG, title: "分组服务指标" },
    }
  }
  return {
    id: `accounts_${suffix}`,
    type,
    width: 12,
    height: "tall",
    config: { ...DEFAULT_ACCOUNTS_CONFIG },
  }
}

export function normalizeOpsLayout(layout?: OpsDashboardLayout): OpsDashboardLayout {
  if (!layout || layout.version !== 1 || !Array.isArray(layout.widgets)) {
    return createDefaultOpsLayout()
  }
  const widths = new Set([4, 6, 8, 12])
  const heights = new Set(["compact", "standard", "tall"])
  return {
    version: 1,
    widgets: layout.widgets
      .filter((widget) => widget && typeof widget.id === "string" && typeof widget.type === "string")
      .map((widget) => {
      const raw = toRecord(widget.config)
      let config: unknown = raw
      if (widget.type === "group_metrics") {
        config = normalizeGroupMetricsConfig(raw)
      } else if (widget.type === "accounts") {
        config = normalizeAccountsConfig(raw)
      }
      return {
        ...widget,
        width: widths.has(widget.width) ? widget.width : 12,
        height: heights.has(widget.height) ? widget.height : "standard",
        config,
      }
    }),
  }
}

const PLATFORMS = new Set<AccountPlatform>(["anthropic", "openai", "gemini", "antigravity", "grok"])
const TIME_RANGES = new Set<OpsTimeRange>(["5m", "30m", "1h", "6h", "24h", "7d", "30d"])
const ACCOUNT_TYPES = new Set<AccountType>(["oauth", "setup-token", "apikey", "upstream", "bedrock", "service_account"])
const ACCOUNT_STATUSES = new Set<AccountStatus>(["active", "inactive", "error", "rate_limited", "temp_unschedulable", "unschedulable"])
const ACCOUNT_SORTS = new Set<AccountsWidgetConfig["sortBy"]>([
  "id",
  "name",
  "status",
  "schedulable",
  "priority",
  "rate_multiplier",
  "concurrency",
  "current_concurrency",
  "last_used_at",
  "created_at",
  "expires_at",
])
const REFRESH_INTERVALS = new Set([30, 60, 120, 300])
const PAGE_SIZES = new Set([10, 20, 50])
const RECENT_ACCOUNT_LIMITS = new Set([3, 5, 10, 20])

function normalizeGroupMetricsConfig(raw: Record<string, unknown>): GroupMetricsWidgetConfig {
  return {
    title: safeTitle(raw.title, DEFAULT_GROUP_METRICS_CONFIG.title),
    platform: isSetValue(raw.platform, PLATFORMS) ? raw.platform : "",
    groupIds: normalizeGroupIDs(raw.groupIds, raw.groupId),
    timeRange: isSetValue(raw.timeRange, TIME_RANGES) ? raw.timeRange : DEFAULT_GROUP_METRICS_CONFIG.timeRange,
    refreshSeconds: safeSetNumber(raw.refreshSeconds, REFRESH_INTERVALS, DEFAULT_GROUP_METRICS_CONFIG.refreshSeconds),
    slaTarget: safeNumber(raw.slaTarget, 0.01, 100, DEFAULT_GROUP_METRICS_CONFIG.slaTarget),
    ttftP95TargetMs: safeNumber(raw.ttftP95TargetMs, 1, 600_000, DEFAULT_GROUP_METRICS_CONFIG.ttftP95TargetMs),
    trendMetric: raw.trendMetric === "tokens" ? "tokens" : "requests",
    recentAccountLimit: safeSetNumber(
      raw.recentAccountLimit,
      RECENT_ACCOUNT_LIMITS,
      DEFAULT_GROUP_METRICS_CONFIG.recentAccountLimit,
    ),
  }
}

function normalizeGroupIDs(value: unknown, legacyValue: unknown): number[] {
  const candidates = Array.isArray(value) ? value : [legacyValue]
  const result: number[] = []
  const seen = new Set<number>()
  for (const candidate of candidates) {
    if (typeof candidate !== "number" || !Number.isSafeInteger(candidate) || candidate <= 0 || seen.has(candidate)) {
      continue
    }
    seen.add(candidate)
    result.push(candidate)
    if (result.length === 12) break
  }
  return result
}

function normalizeAccountsConfig(raw: Record<string, unknown>): AccountsWidgetConfig {
  const statuses = normalizeAccountStatuses(raw.statuses, raw.status)
  return {
    title: safeTitle(raw.title, DEFAULT_ACCOUNTS_CONFIG.title),
    platform: isSetValue(raw.platform, PLATFORMS) ? raw.platform : "",
    accountType: isSetValue(raw.accountType, ACCOUNT_TYPES) ? raw.accountType : "",
    statuses,
    excludedStatuses: statuses.length === 0 ? normalizeAccountStatuses(raw.excludedStatuses) : [],
    groups: normalizeAccountGroups(raw.groups, raw.group),
    pageSize: safeSetNumber(raw.pageSize, PAGE_SIZES, DEFAULT_ACCOUNTS_CONFIG.pageSize),
    refreshSeconds: safeSetNumber(raw.refreshSeconds, REFRESH_INTERVALS, DEFAULT_ACCOUNTS_CONFIG.refreshSeconds),
    showUsage: typeof raw.showUsage === "boolean" ? raw.showUsage : DEFAULT_ACCOUNTS_CONFIG.showUsage,
    showSchedulerScore: typeof raw.showSchedulerScore === "boolean"
      ? raw.showSchedulerScore
      : DEFAULT_ACCOUNTS_CONFIG.showSchedulerScore,
    sortBy: isSetValue(raw.sortBy, ACCOUNT_SORTS) ? raw.sortBy : DEFAULT_ACCOUNTS_CONFIG.sortBy,
    sortOrder: raw.sortOrder === "desc" ? "desc" : "asc",
  }
}

function normalizeAccountStatuses(value: unknown, legacyValue?: unknown): AccountStatus[] {
  const candidates = Array.isArray(value) ? value : [legacyValue]
  const result: AccountStatus[] = []
  const seen = new Set<AccountStatus>()
  for (const candidate of candidates) {
    if (!isSetValue(candidate, ACCOUNT_STATUSES) || seen.has(candidate)) continue
    seen.add(candidate)
    result.push(candidate)
  }
  return result
}

function normalizeAccountGroups(value: unknown, legacyValue: unknown): string[] {
  const candidates = Array.isArray(value) ? value : [legacyValue]
  const result: string[] = []
  const seen = new Set<string>()
  for (const candidate of candidates) {
    if (typeof candidate !== "string" || (candidate !== "ungrouped" && !/^[1-9]\d*$/.test(candidate)) || seen.has(candidate)) {
      continue
    }
    seen.add(candidate)
    result.push(candidate)
    if (result.length === 12) break
  }
  return result
}

function toRecord(value: unknown): Record<string, unknown> {
  return value != null && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

function safeTitle(value: unknown, fallback: string): string {
  return typeof value === "string" && value.trim() !== "" ? value.trim().slice(0, 64) : fallback
}

function safeNumber(value: unknown, min: number, max: number, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) && value >= min && value <= max
    ? value
    : fallback
}

function safeSetNumber(value: unknown, allowed: Set<number>, fallback: number): number {
  return typeof value === "number" && allowed.has(value) ? value : fallback
}

function isSetValue<T extends string>(value: unknown, allowed: Set<T>): value is T {
  return typeof value === "string" && allowed.has(value as T)
}
