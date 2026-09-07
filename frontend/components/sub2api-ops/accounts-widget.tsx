"use client"

import { useEffect, useState } from "react"
import {
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  CircleGauge,
  LoaderCircle,
  RefreshCw,
  Users,
} from "lucide-react"
import { apiFetch } from "@/lib/api"
import { money, relativeTime } from "@/lib/format"
import { usePollingQuery } from "@/lib/sub2api-ops-query"
import type {
  AccountTodayStats,
  AccountTodayStatsResponse,
  AccountUsageDetail,
  AccountUsageDetailsResponse,
  AccountsWidgetConfig,
  OpsAccount,
  OpsAccountPage,
  OpsWidgetHeight,
} from "@/lib/sub2api-ops-types"
import { cn } from "@/lib/utils"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Progress } from "@/components/ui/progress"
import { Skeleton } from "@/components/ui/skeleton"
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

interface AccountsWidgetProps {
  config: AccountsWidgetConfig
  height: OpsWidgetHeight
}

interface AccountState {
  label: string
  tone: "good" | "warning" | "bad" | "muted"
}

export function AccountsWidget({ config, height }: AccountsWidgetProps) {
  const [page, setPage] = useState(1)
  const [todayStats, setTodayStats] = useState<Record<string, AccountTodayStats>>({})
  const [usageLoading, setUsageLoading] = useState(false)
  const [usageError, setUsageError] = useState(false)

  useEffect(() => {
    setPage(1)
  }, [
    config.platform,
    config.accountType,
    config.statuses.join(","),
    config.excludedStatuses.join(","),
    config.groups.join(","),
    config.pageSize,
    config.sortBy,
    config.sortOrder,
  ])

  const params = new URLSearchParams({
    page: String(page),
    page_size: String(config.pageSize),
    sort_by: config.sortBy,
    sort_order: config.sortOrder,
    include_scheduler_score: config.showSchedulerScore ? "1" : "0",
  })
  if (config.platform) params.set("platform", config.platform)
  if (config.accountType) params.set("type", config.accountType)
  if (config.statuses.length > 0) params.set("statuses", config.statuses.join(","))
  if (config.excludedStatuses.length > 0) params.set("exclude_statuses", config.excludedStatuses.join(","))
  if (config.groups.length > 0) params.set("groups", config.groups.join(","))

  const query = usePollingQuery<OpsAccountPage>(
    `/sub2api-ops/accounts?${params.toString()}`,
    config.refreshSeconds,
  )
  const accountIDs = query.data?.items.map((account) => account.id) ?? []
  const accountIDsKey = [...accountIDs].sort((left, right) => left - right).join(",")
  const windowAccounts = (query.data?.items ?? [])
    .filter(supportsWindowStats)
    .sort((left, right) => left.id - right.id)
  const windowAccountIDs = windowAccounts.map((account) => account.id)
  const detailParams = new URLSearchParams({
    account_ids: accountIDsKey,
    include_total: config.showUsage ? "1" : "0",
  })
  if (windowAccountIDs.length > 0) {
    detailParams.set("window_account_ids", windowAccountIDs.join(","))
    detailParams.set("window_revision", usageWindowRevision(windowAccounts))
  }
  const detailsQuery = usePollingQuery<AccountUsageDetailsResponse>(
    accountIDs.length > 0 && (config.showUsage || windowAccountIDs.length > 0)
      ? `/sub2api-ops/accounts/usage-details?${detailParams.toString()}`
      : null,
    Math.max(300, config.refreshSeconds),
  )

  useEffect(() => {
    if (query.data && query.data.pages > 0 && page > query.data.pages) {
      setPage(query.data.pages)
    }
  }, [page, query.data])

  useEffect(() => {
    if (!config.showUsage || accountIDs.length === 0) {
      setTodayStats({})
      setUsageLoading(false)
      setUsageError(false)
      return
    }
    let active = true
    const controller = new AbortController()
    setUsageLoading(true)
    setUsageError(false)
    apiFetch<AccountTodayStatsResponse>("/sub2api-ops/accounts/today-stats", {
      method: "POST",
      body: JSON.stringify({ account_ids: accountIDs }),
      signal: controller.signal,
    })
      .then((response) => {
        if (active) setTodayStats(response.stats ?? {})
      })
      .catch(() => {
        if (active) setUsageError(true)
      })
      .finally(() => {
        if (active) setUsageLoading(false)
      })
    return () => {
      active = false
      controller.abort()
    }
  }, [config.showUsage, accountIDsKey, query.updatedAt?.getTime()])

  if (query.loading) return <AccountsSkeleton />
  if (query.error && !query.data) {
    return <AccountsError message={query.error} onRetry={query.refetch} />
  }

  const accounts = query.data?.items ?? []
  const states = accounts.map(getAccountState)
  const healthyCount = states.filter((state) => state.tone === "good").length
  const riskCount = states.filter((state) => state.tone === "warning" || state.tone === "bad").length
  const concurrency = accounts.reduce((sum, account) => sum + (account.current_concurrency || 0), 0)
  const capacity = accounts.reduce((sum, account) => sum + Math.max(0, account.concurrency || 0), 0)
  const currentStats = accounts
    .map((account) => todayStats[String(account.id)])
    .filter((stats): stats is AccountTodayStats => stats != null)
  const totalTodayRequests = currentStats.reduce((sum, stats) => sum + stats.requests, 0)
  const totalTodayUserCost = currentStats.reduce((sum, stats) => sum + stats.user_cost, 0)
  const compact = height === "compact"

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="grid shrink-0 grid-cols-2 divide-x divide-border border-b border-border @3xl/widget:grid-cols-4">
        <SummaryStat label="匹配账号" value={formatInteger(query.data?.total ?? 0)} detail={`本页 ${accounts.length}`} />
        <SummaryStat label="本页健康" value={String(healthyCount)} detail={riskCount > 0 ? `${riskCount} 个需关注` : "当前页无异常"} tone={riskCount > 0 ? "warning" : "good"} />
        {!compact ? (
          <>
            <SummaryStat label="本页并发" value={capacity > 0 ? `${concurrency}/${capacity}` : String(concurrency)} detail={capacity > 0 ? `负载 ${Math.round((concurrency / capacity) * 100)}%` : "未设置容量"} />
            <SummaryStat
              label="本页今日请求"
              value={!config.showUsage
                ? "未启用"
                : usageLoading && currentStats.length === 0 ? "—"
                : usageError && currentStats.length === 0 ? "—" : formatCompact(totalTodayRequests)}
              detail={usageLoading ? "正在更新用量" : usageError ? "用量更新失败" : `用户扣费 ${money(totalTodayUserCost)}`}
              tone={usageError ? "warning" : "neutral"}
            />
          </>
        ) : null}
      </div>

      <div className="flex h-9 shrink-0 items-center justify-between border-b border-border px-3">
        <div className="flex min-w-0 flex-1 items-center gap-2 text-[11px] text-muted-foreground">
          <CircleGauge className="size-3.5" />
          <span className="truncate">{filterSummary(config)}</span>
        </div>
        <div className="flex shrink-0 items-center gap-1 text-[11px] text-muted-foreground">
          {query.refreshing || usageLoading || detailsQuery.refreshing ? <LoaderCircle className="size-3 animate-spin" /> : null}
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={() => {
              query.refetch()
              detailsQuery.refetch()
            }}
            aria-label="刷新账号"
          >
            <RefreshCw className="size-3.5" />
          </Button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-auto">
        {accounts.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
            <Users className="size-7 text-muted-foreground/60" />
            <p className="text-sm font-medium">没有匹配的账号</p>
            <p className="text-xs text-muted-foreground">调整组件过滤条件后重试。</p>
          </div>
        ) : (
          <table className={cn(
            "w-full table-fixed caption-bottom text-sm",
            config.showUsage
              ? config.showSchedulerScore ? "min-w-[90rem]" : "min-w-[82rem]"
              : config.showSchedulerScore ? "min-w-[64rem]" : "min-w-[56rem]",
          )}>
            <TableHeader className="sticky top-0 z-10 bg-card">
              <TableRow className="hover:bg-card">
                <TableHead className="w-52 pl-4 text-xs">账号</TableHead>
                <TableHead className="w-32 text-xs">状态</TableHead>
                <TableHead className="w-36 text-xs">负载</TableHead>
                {config.showUsage ? <TableHead className="w-52 text-xs">今日用量</TableHead> : null}
                {config.showUsage ? <TableHead className="w-52 text-xs">总用量</TableHead> : null}
                <TableHead className="w-64 text-xs">用量窗口</TableHead>
                {config.showSchedulerScore ? <TableHead className="w-32 text-xs">调度评分</TableHead> : null}
                <TableHead className="w-42 text-xs">活跃 / 存活</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {accounts.map((account) => (
                <AccountRow
                  key={account.id}
                  account={account}
                  config={config}
                  today={todayStats[String(account.id)]}
                  detail={detailsQuery.data?.details[String(account.id)]}
                  usagePending={usageLoading && todayStats[String(account.id)] == null}
                  usageError={usageError && todayStats[String(account.id)] == null}
                  detailPending={detailsQuery.loading && detailsQuery.data?.details[String(account.id)] == null}
                  totalError={Boolean(detailsQuery.error) || (detailsQuery.data?.failed_total_ids?.includes(account.id) ?? false)}
                  windowError={supportsWindowStats(account) && (
                    Boolean(detailsQuery.error) || (detailsQuery.data?.failed_window_ids?.includes(account.id) ?? false)
                  )}
                />
              ))}
            </TableBody>
          </table>
        )}
      </div>

      <div className="flex h-10 shrink-0 items-center justify-between border-t border-border px-3 text-[11px] text-muted-foreground">
        <span>
          第 {query.data?.page ?? page} / {Math.max(1, query.data?.pages ?? 1)} 页
          {query.error ? <span className="ml-2 text-warning">刷新失败，保留上次数据</span> : null}
        </span>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            disabled={page <= 1}
            onClick={() => setPage((value) => Math.max(1, value - 1))}
            aria-label="上一页"
          >
            <ChevronLeft />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            disabled={page >= (query.data?.pages ?? 1)}
            onClick={() => setPage((value) => value + 1)}
            aria-label="下一页"
          >
            <ChevronRight />
          </Button>
        </div>
      </div>
    </div>
  )
}

function AccountRow({
  account,
  config,
  today,
  detail,
  usagePending,
  usageError,
  detailPending,
  totalError,
  windowError,
}: {
  account: OpsAccount
  config: AccountsWidgetConfig
  today?: AccountTodayStats
  detail?: AccountUsageDetail
  usagePending: boolean
  usageError: boolean
  detailPending: boolean
  totalError: boolean
  windowError: boolean
}) {
  const state = getAccountState(account)
  const load = account.concurrency > 0
    ? Math.min(100, ((account.current_concurrency || 0) / account.concurrency) * 100)
    : 0
  const score = getSchedulerScore(account, config.groups)

  return (
    <TableRow className="h-14">
      <TableCell className="pl-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="max-w-30 truncate font-medium" title={account.name}>{account.name}</span>
            <Badge variant="outline" className="px-1.5 py-0 text-[9px] font-normal uppercase">{account.platform}</Badge>
          </div>
          <p className="mt-0.5 truncate text-[10px] text-muted-foreground">
            {account.groups?.map((group) => group.name).join(" / ") || typeLabel(account.type)}
          </p>
        </div>
      </TableCell>
      <TableCell>
        <div title={state.label}>
          <Badge
            variant="outline"
            className={cn(
              "gap-1 border-transparent px-1.5 py-0.5 text-[10px]",
              state.tone === "good" && "bg-success/10 text-success",
              state.tone === "warning" && "bg-warning/12 text-warning",
              state.tone === "bad" && "bg-danger/10 text-danger",
              state.tone === "muted" && "bg-muted text-muted-foreground",
            )}
          >
            <span className="size-1.5 rounded-full bg-current" />
            {state.label}
          </Badge>
        </div>
      </TableCell>
      <TableCell>
        <div className="w-27">
          <div className="mb-1 flex justify-between font-mono text-[10px] tabular-nums">
            <span>{account.current_concurrency || 0}</span>
            <span className="text-muted-foreground">/ {account.concurrency || "—"}</span>
          </div>
          <Progress
            value={load}
            className="h-1.5 bg-muted"
            aria-label={`并发负载 ${Math.round(load)}%`}
          />
        </div>
      </TableCell>
      {config.showUsage ? (
        <TableCell>
          <AccountStatsCell stats={today} pending={usagePending} error={usageError} />
        </TableCell>
      ) : null}
      {config.showUsage ? (
        <TableCell>
          <AccountStatsCell stats={detail?.total} pending={detailPending} error={totalError} />
        </TableCell>
      ) : null}
      <TableCell>
        <AccountUsageWindows
          account={account}
          stats={detail?.window_stats}
          pending={supportsWindowStats(account) && detailPending}
          error={windowError}
        />
      </TableCell>
      {config.showSchedulerScore ? (
        <TableCell>
          <p className="font-mono text-xs font-medium tabular-nums">{score}</p>
          <p className="mt-0.5 text-[10px] text-muted-foreground">优先级 {account.priority}</p>
        </TableCell>
      ) : null}
      <TableCell>
        <p className="text-xs">{relativeTime(account.last_used_at)}</p>
        <p className="mt-0.5 text-[10px] text-muted-foreground">
          存活 {accountAge(account.created_at)}{account.expires_at ? ` · ${expiryLabel(account.expires_at)}` : ""}
        </p>
      </TableCell>
    </TableRow>
  )
}

function AccountStatsCell({
  stats,
  pending,
  error,
}: {
  stats?: AccountTodayStats
  pending: boolean
  error: boolean
}) {
  if (!stats) {
    return (
      <p className={cn("text-xs text-muted-foreground", error && "text-warning")}>
        {pending ? "加载中" : error ? "更新失败" : "—"}
      </p>
    )
  }
  return (
    <div className="space-y-0.5">
      <p className="font-mono text-xs font-medium tabular-nums">
        {formatCompact(stats.requests)} req · {formatCompact(stats.tokens)} tok
      </p>
      <p className="font-mono text-[10px] text-muted-foreground tabular-nums">账号扣费 {money(stats.cost)}</p>
      <p className="font-mono text-[10px] text-muted-foreground tabular-nums">用户扣费 {money(stats.user_cost)}</p>
      {error ? <p className="text-[9px] text-warning">更新失败，显示旧数据</p> : null}
    </div>
  )
}

function AccountUsageWindows({
  account,
  stats,
  pending,
  error,
}: {
  account: OpsAccount
  stats?: Record<string, AccountTodayStats>
  pending: boolean
  error: boolean
}) {
  const windows = account.usage_windows?.items ?? []
  const statsEntries = Object.entries(stats ?? {})
  if (windows.length === 0 && statsEntries.length === 0) {
    return (
      <p className={cn("text-xs text-muted-foreground", error && "text-warning")}>
        {pending ? "窗口用量加载中" : error ? "窗口用量更新失败" : "暂无窗口用量"}
      </p>
    )
  }
  const windowLabels = new Set(windows.map((window) => window.label))
  const unmatchedStats = statsEntries.filter(([label]) => !windowLabels.has(label))
  return (
    <div className="space-y-1.5" title={account.usage_windows?.updated_at ? `更新于 ${formatDateTime(account.usage_windows.updated_at)}` : undefined}>
      {windows.map((window) => {
        const utilization = Number.isFinite(window.utilization) ? Math.max(0, window.utilization) : 0
        const windowStats = stats?.[window.label]
        return (
          <div key={`${window.label}-${window.resets_at ?? "now"}`} className="min-w-0">
            {windowStats ? <WindowStatsSummary stats={windowStats} /> : null}
            <div className="mb-0.5 flex items-center justify-between gap-2 font-mono text-[10px] tabular-nums">
              <span className="font-sans text-muted-foreground">{usageWindowLabel(window.label)}</span>
              <span className={cn(
                utilization >= 100 && "text-danger",
                utilization >= 80 && utilization < 100 && "text-warning",
              )}>
                {formatPercent(utilization)}
              </span>
            </div>
            <Progress
              value={Math.min(100, utilization)}
              className="h-1.5 bg-muted"
              aria-label={`${usageWindowLabel(window.label)} 使用率 ${formatPercent(utilization)}`}
            />
            {window.resets_at ? (
              <p className="mt-0.5 truncate text-[9px] text-muted-foreground">重置 {resetTimeLabel(window.resets_at)}</p>
            ) : null}
          </div>
        )
      })}
      {unmatchedStats.map(([label, windowStats]) => (
        <div key={`stats-${label}`} className="border-t border-border/70 pt-1">
          <p className="mb-0.5 text-[9px] text-muted-foreground">{usageWindowLabel(label)} 用量</p>
          <WindowStatsSummary stats={windowStats} />
        </div>
      ))}
      {pending ? <p className="text-[9px] text-muted-foreground">窗口用量加载中</p> : null}
      {!pending && error ? <p className="text-[9px] text-warning">窗口用量更新失败</p> : null}
    </div>
  )
}

function WindowStatsSummary({ stats }: { stats: AccountTodayStats }) {
  return (
    <div className="mb-1 text-[9px] leading-tight text-muted-foreground">
      <p className="font-mono tabular-nums">
        {formatCompact(stats.requests)} req · {formatCompact(stats.tokens)} tok
      </p>
      <p className="font-mono tabular-nums">
        账号扣费 {money(stats.cost)} · 用户扣费 {money(stats.user_cost)}
      </p>
    </div>
  )
}

function usageWindowLabel(label: string): string {
  return {
    monthly: "月度",
    "7d_sonnet": "7d Sonnet",
    "7d_fable": "7d Fable",
    gemini_shared_daily: "Gemini 共享日窗口",
    gemini_pro_daily: "Gemini Pro 日窗口",
    gemini_flash_daily: "Gemini Flash 日窗口",
    gemini_shared_minute: "Gemini 共享分钟窗口",
    gemini_pro_minute: "Gemini Pro 分钟窗口",
    gemini_flash_minute: "Gemini Flash 分钟窗口",
  }[label] ?? label
}

function supportsWindowStats(account: OpsAccount): boolean {
  return (account.usage_windows?.items.length ?? 0) > 0
    || account.platform === "gemini"
    || account.platform === "grok"
}

function usageWindowRevision(accounts: OpsAccount[]): string {
  const value = accounts.map((account) => [
    account.id,
    account.usage_windows?.updated_at ?? "",
    ...(account.usage_windows?.items ?? []).map((window) => `${window.label}:${window.resets_at ?? ""}`),
  ].join(":"))
    .join("|")
  let hash = 2_166_136_261
  for (let index = 0; index < value.length; index += 1) {
    hash = Math.imul(hash ^ value.charCodeAt(index), 16_777_619)
  }
  return (hash >>> 0).toString(36)
}

function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return "—"
  return `${value.toFixed(value >= 10 ? 0 : 1)}%`
}

function formatDateTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN")
}

function resetTimeLabel(value: string): string {
  const remaining = new Date(value).getTime() - Date.now()
  if (!Number.isFinite(remaining)) return "—"
  if (remaining <= 0) return relativeTime(value)

  const minutes = Math.ceil(remaining / 60_000)
  if (minutes < 60) return `${minutes} 分钟后`

  const hours = Math.ceil(remaining / 3_600_000)
  if (hours < 24) return `${hours} 小时后`
  return `${Math.ceil(remaining / 86_400_000)} 天后`
}

function SummaryStat({
  label,
  value,
  detail,
  tone = "neutral",
}: {
  label: string
  value: string
  detail: string
  tone?: "good" | "warning" | "neutral"
}) {
  return (
    <div className="min-w-0 px-4 py-3">
      <p className="text-[10px] text-muted-foreground">{label}</p>
      <p className={cn(
        "mt-0.5 truncate font-mono text-lg font-semibold tabular-nums",
        tone === "good" && "text-success",
        tone === "warning" && "text-warning",
      )}>
        {value}
      </p>
      <p className="truncate text-[10px] text-muted-foreground">{detail}</p>
    </div>
  )
}

function getAccountState(account: OpsAccount): AccountState {
  const now = Date.now()
  if (account.status === "error") return { label: "错误", tone: "bad" }
  if (account.status !== "active") return { label: "已停用", tone: "muted" }
  if (account.auto_pause_on_expired && account.expires_at != null && account.expires_at * 1000 <= now) {
    return { label: "已过期", tone: "bad" }
  }
  if (future(account.temp_unschedulable_until, now)) return { label: "临时暂停", tone: "warning" }
  if (future(account.rate_limit_reset_at, now)) return { label: "限流中", tone: "warning" }
  if (future(account.overload_until, now)) return { label: "上游过载", tone: "warning" }
  if (quotaExceeded(account)) return { label: "配额耗尽", tone: "warning" }
  if (!account.schedulable) return { label: "暂停调度", tone: "muted" }
  return { label: "健康", tone: "good" }
}

function quotaExceeded(account: OpsAccount): boolean {
  return exceeded(account.quota_used, account.quota_limit)
    || exceeded(account.quota_daily_used, account.quota_daily_limit)
    || exceeded(account.quota_weekly_used, account.quota_weekly_limit)
}

function exceeded(used?: number, limit?: number): boolean {
  return limit != null && limit > 0 && used != null && used >= limit
}

function getSchedulerScore(account: OpsAccount, groups: string[]): string {
  const groupID = groups.length === 1 ? Number(groups[0]) : Number.NaN
  const selected = Number.isFinite(groupID) && groupID > 0
    ? account.scheduler_scores?.find((score) => score.group_id === groupID)
    : undefined
  const score = selected ?? account.scheduler_score
  if (!score) return "—"
  if (score.sticky_score_infinity) return "+∞"
  const value = score.sticky_weighted_enabled ? score.sticky_score : score.base_score
  return Number.isFinite(value) ? value.toFixed(3).replace(/\.?0+$/, "") : "—"
}

function filterSummary(config: AccountsWidgetConfig): string {
  const parts: string[] = []
  if (config.platform) parts.push(config.platform)
  if (config.accountType) parts.push(typeLabel(config.accountType))
  if (config.groups.length === 1) parts.push(config.groups[0] === "ungrouped" ? "未分组" : "指定分组")
  if (config.groups.length > 1) parts.push(`${config.groups.length} 个分组`)
  if (config.statuses.length === 1) parts.push(statusFilterLabel(config.statuses[0]))
  if (config.statuses.length > 1) parts.push(`${config.statuses.length} 个状态`)
  if (config.excludedStatuses.length > 0) {
    parts.push(`排除 ${config.excludedStatuses.map(statusFilterLabel).join(" / ")}`)
  }
  return parts.length > 0 ? parts.join(" · ") : "全部账号"
}

function statusFilterLabel(status: string): string {
  return {
    active: "基础可调度",
    inactive: "已停用",
    error: "错误",
    rate_limited: "限流中",
    temp_unschedulable: "临时暂停",
    unschedulable: "暂停调度",
  }[status] ?? status
}

function typeLabel(type: string): string {
  return {
    oauth: "OAuth",
    "setup-token": "Setup Token",
    apikey: "API Key",
    upstream: "上游转发",
    bedrock: "AWS Bedrock",
    service_account: "Service Account",
  }[type] ?? type
}

function future(value: string | null, now: number): boolean {
  return value != null && new Date(value).getTime() > now
}

function accountAge(value: string): string {
  const elapsed = Math.max(0, Date.now() - new Date(value).getTime())
  const days = Math.floor(elapsed / 86_400_000)
  if (days >= 365) return `${(days / 365).toFixed(1)} 年`
  if (days > 0) return `${days} 天`
  const hours = Math.floor(elapsed / 3_600_000)
  return hours > 0 ? `${hours} 小时` : "不足 1 小时"
}

function expiryLabel(unixSeconds: number): string {
  const remaining = unixSeconds * 1000 - Date.now()
  if (remaining <= 0) return "已到期"
  const days = Math.ceil(remaining / 86_400_000)
  return `${days} 天后到期`
}

function formatInteger(value: number): string {
  return Number.isFinite(value) ? value.toLocaleString("zh-CN") : "—"
}

function formatCompact(value: number): string {
  if (!Number.isFinite(value)) return "—"
  return new Intl.NumberFormat("zh-CN", { notation: "compact", maximumFractionDigits: 1 }).format(value)
}

function AccountsSkeleton() {
  return (
    <div className="space-y-3 p-4">
      <div className="grid grid-cols-2 gap-3 @3xl/widget:grid-cols-4">
        {Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-16" />)}
      </div>
      {Array.from({ length: 6 }).map((_, index) => <Skeleton key={index} className="h-11 w-full" />)}
    </div>
  )
}

function AccountsError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
      <AlertCircle className="size-7 text-danger" />
      <div>
        <p className="text-sm font-medium">账号加载失败</p>
        <p className="mt-1 max-w-md text-xs text-muted-foreground">{message}</p>
      </div>
      <Button size="sm" variant="outline" onClick={onRetry}><RefreshCw />重试</Button>
    </div>
  )
}
