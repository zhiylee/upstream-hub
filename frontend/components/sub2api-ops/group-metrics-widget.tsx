"use client"

import { Activity, AlertCircle, CheckCircle2, Clock3, Gauge, LoaderCircle, RadioTower, RefreshCw } from "lucide-react"
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import type {
  GroupMetricsWidgetConfig,
  OpsRecentAccountsResponse,
  OpsSnapshot,
  OpsWidgetHeight,
  OpsWidgetWidth,
} from "@/lib/sub2api-ops-types"
import { usePollingQuery } from "@/lib/sub2api-ops-query"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

interface GroupMetricsWidgetProps {
  config: GroupMetricsWidgetConfig
  width: OpsWidgetWidth
  height: OpsWidgetHeight
}

export function GroupMetricsWidget({ config, width, height }: GroupMetricsWidgetProps) {
  const params = new URLSearchParams({ time_range: config.timeRange })
  if (config.platform) params.set("platform", config.platform)
  if (config.groupIds.length > 0) params.set("group_ids", config.groupIds.join(","))
  const query = usePollingQuery<OpsSnapshot>(
    `/sub2api-ops/snapshot?${params.toString()}`,
    config.refreshSeconds,
  )
  const recentParams = new URLSearchParams({
    time_range: config.timeRange,
    limit: String(config.recentAccountLimit),
  })
  if (config.platform) recentParams.set("platform", config.platform)
  if (config.groupIds.length > 0) recentParams.set("group_ids", config.groupIds.join(","))
  const recentQuery = usePollingQuery<OpsRecentAccountsResponse>(
    `/sub2api-ops/recent-accounts?${recentParams.toString()}`,
    config.refreshSeconds,
  )

  if (query.loading) return <MetricsSkeleton />
  if (query.error && !query.data) {
    return <WidgetError message={query.error} onRetry={query.refetch} />
  }

  const overview = query.data?.overview
  if (!overview) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 px-6 text-center">
        <Activity className="size-7 text-muted-foreground/60" />
        <p className="text-sm font-medium">当前范围暂无指标样本</p>
        <p className="text-xs text-muted-foreground">请检查 Sub2API Ops 是否启用，或调整统计范围。</p>
      </div>
    )
  }

  const hasRequests = overview.request_count_total > 0
  const slaPercent = hasRequests ? overview.sla * 100 : null
  const errorPercent = hasRequests ? overview.error_rate * 100 : null
  const upstreamErrorPercent = hasRequests ? overview.upstream_error_rate * 100 : null
  const ttftP95 = overview.ttft.p95_ms
  const worstGroupPercentiles = query.data?.aggregation?.percentile_mode === "worst_group"
  const slaHealthy = slaPercent != null && slaPercent >= config.slaTarget
  const ttftHealthy = ttftP95 != null && ttftP95 <= config.ttftP95TargetMs
  const overallHealthy = hasRequests && slaHealthy && (ttftP95 == null || ttftHealthy)
  const points = query.data?.throughput_trend?.points ?? []
  const trendData = points.map((point) => ({
    time: formatBucket(point.bucket_start, config.timeRange),
    fullTime: new Date(point.bucket_start).toLocaleString("zh-CN"),
    value: config.trendMetric === "tokens" ? point.token_consumed : point.request_count,
  }))
  const compact = height === "compact"
  const metricColumns = width <= 6 ? "grid-cols-2" : "grid-cols-2 @3xl/widget:grid-cols-4"

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-10 shrink-0 items-center justify-between border-b border-border/70 px-4">
        <div className={cn("flex items-center gap-2 text-xs font-medium", overallHealthy ? "text-success" : "text-warning")}>
          {overallHealthy ? <CheckCircle2 className="size-3.5" /> : <AlertCircle className="size-3.5" />}
          {overallHealthy ? "指标达标" : hasRequests ? "需要关注" : "等待流量"}
        </div>
        <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
          {query.refreshing || recentQuery.refreshing ? <LoaderCircle className="size-3 animate-spin" /> : null}
          {query.data?.generated_at ? `生成于 ${formatClock(query.data.generated_at)}` : ""}
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={() => {
              query.refetch()
              recentQuery.refetch()
            }}
            aria-label="刷新指标"
          >
            <RefreshCw className="size-3.5" />
          </Button>
        </div>
      </div>

      <div className={cn("grid shrink-0 border-b border-border/70", metricColumns)}>
        <MetricStat
          label="SLA"
          value={formatPercent(slaPercent, 2)}
          detail={`目标 ≥ ${config.slaTarget}%`}
          tone={slaPercent == null ? "neutral" : slaHealthy ? "good" : "bad"}
          icon={<Gauge />}
        />
        <MetricStat
          label={worstGroupPercentiles ? "最慢分组 TTFT P95" : "TTFT P95"}
          value={formatDuration(ttftP95)}
          detail={worstGroupPercentiles
            ? `${query.data?.aggregation?.group_ids.length ?? 0} 组 · 目标 ≤ ${formatDuration(config.ttftP95TargetMs)}`
            : `目标 ≤ ${formatDuration(config.ttftP95TargetMs)}`}
          tone={ttftP95 == null ? "neutral" : ttftHealthy ? "good" : "bad"}
          icon={<Clock3 />}
        />
        <MetricStat
          label="请求量"
          value={formatCompact(overview.request_count_total)}
          detail={`成功 ${formatCompact(overview.success_count)}`}
          tone="neutral"
        />
        <MetricStat
          label="错误率"
          value={formatPercent(errorPercent, 2)}
          detail={`上游 ${formatPercent(upstreamErrorPercent, 2)}`}
          tone={errorPercent != null && errorPercent > 1 ? "bad" : "neutral"}
        />
        {!compact ? (
          <>
            <MetricStat
              label="Token"
              value={formatCompact(overview.token_consumed)}
              detail={`TPS ${formatRate(overview.tps.current)}`}
              tone="neutral"
            />
            <MetricStat
              label="QPS"
              value={formatRate(overview.qps.current)}
              detail={`峰值 ${formatRate(overview.qps.peak)}`}
              tone="neutral"
            />
          </>
        ) : null}
      </div>

      <RecentAccountsBand
        data={recentQuery.data}
        loading={recentQuery.loading}
        error={recentQuery.error}
      />

      {!compact ? (
        <div className="min-h-0 flex-1 px-3 pb-3 pt-3">
          <div className="mb-2 flex items-center justify-between px-1">
            <p className="text-[11px] font-medium text-muted-foreground">
              {config.trendMetric === "tokens" ? "Token 趋势" : "请求趋势"}
            </p>
            {query.error ? <span className="text-[11px] text-warning">刷新失败，保留上次数据</span> : null}
          </div>
          {trendData.length > 0 ? (
            <ResponsiveContainer width="100%" height="90%">
              <AreaChart data={trendData} margin={{ top: 4, right: 8, left: -12, bottom: 0 }}>
                <CartesianGrid stroke="var(--border)" strokeDasharray="3 3" vertical={false} />
                <XAxis
                  dataKey="time"
                  axisLine={false}
                  tickLine={false}
                  minTickGap={28}
                  tick={{ fill: "var(--muted-foreground)", fontSize: 10 }}
                />
                <YAxis
                  axisLine={false}
                  tickLine={false}
                  width={48}
                  tickFormatter={formatCompact}
                  tick={{ fill: "var(--muted-foreground)", fontSize: 10 }}
                />
                <Tooltip content={<TrendTooltip metric={config.trendMetric} />} />
                <Area
                  type="monotone"
                  dataKey="value"
                  stroke="var(--brand)"
                  strokeWidth={2}
                  fill="var(--brand)"
                  fillOpacity={0.12}
                  activeDot={{ r: 3, fill: "var(--brand)", strokeWidth: 0 }}
                />
              </AreaChart>
            </ResponsiveContainer>
          ) : (
            <div className="flex h-full items-center justify-center text-xs text-muted-foreground">暂无趋势数据</div>
          )}
        </div>
      ) : (
        <div className="grid flex-1 grid-cols-3 items-center divide-x divide-border px-2 text-center">
          <SmallRate label="当前 QPS" value={formatRate(overview.qps.current)} />
          <SmallRate label="峰值 QPS" value={formatRate(overview.qps.peak)} />
          <SmallRate label="平均 TPS" value={formatRate(overview.tps.avg)} />
        </div>
      )}
    </div>
  )
}

function RecentAccountsBand({
  data,
  loading,
  error,
}: {
  data: OpsRecentAccountsResponse | null
  loading: boolean
  error: string | null
}) {
  let status = ""
  if (loading && !data) status = "正在读取最近调度"
  else if (error && !data) status = "最近调度记录不可用"
  else if (data?.accounts.length === 0) status = "当前范围没有调度账号"

  return (
    <div className="flex h-15 shrink-0 items-center gap-3 border-b border-border/70 px-4">
      <div className="flex w-23 shrink-0 items-center gap-2">
        <RadioTower className="size-3.5 text-muted-foreground" />
        <div>
          <p className="text-[10px] text-muted-foreground">最近调度</p>
          <p className="font-mono text-xs font-semibold tabular-nums">
            {data ? `${data.accounts.length} 个账号` : "—"}
          </p>
        </div>
      </div>
      <div className="min-w-0 flex-1 overflow-x-auto">
        {status ? (
          <p className={cn("text-xs text-muted-foreground", error && "text-warning")}>{status}</p>
        ) : (
          <div className="flex min-w-max items-center">
            {data?.accounts.map((account, index) => (
              <div
                key={account.id}
                className={cn("flex shrink-0 items-center gap-2 px-3", index > 0 && "border-l border-border")}
              >
                <span className="max-w-32 truncate text-xs font-medium" title={account.name}>{account.name}</span>
                <span className="font-mono text-[10px] tabular-nums text-muted-foreground">
                  {formatClock(account.last_used_at)}
                </span>
              </div>
            ))}
            {data?.truncated ? <span className="px-3 text-[10px] text-muted-foreground">已取 Top {data.limit}</span> : null}
            {error ? <span className="px-3 text-[10px] text-warning">更新失败</span> : null}
          </div>
        )}
      </div>
    </div>
  )
}

function MetricStat({
  label,
  value,
  detail,
  tone,
  icon,
}: {
  label: string
  value: string
  detail: string
  tone: "good" | "bad" | "neutral"
  icon?: React.ReactNode
}) {
  return (
    <div className="min-w-0 border-r border-border/70 px-4 py-3 last:border-r-0">
      <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
        {icon ? <span className="[&_svg]:size-3">{icon}</span> : null}
        {label}
      </div>
      <p className={cn(
        "mt-1 truncate font-mono text-xl font-semibold tabular-nums",
        tone === "good" && "text-success",
        tone === "bad" && "text-danger",
      )}>
        {value}
      </p>
      <p className="mt-0.5 truncate text-[10px] text-muted-foreground">{detail}</p>
    </div>
  )
}

function SmallRate({ label, value }: { label: string; value: string }) {
  return (
    <div className="px-2">
      <p className="text-[10px] text-muted-foreground">{label}</p>
      <p className="mt-1 font-mono text-base font-semibold tabular-nums">{value}</p>
    </div>
  )
}

function MetricsSkeleton() {
  return (
    <div className="space-y-4 p-4">
      <Skeleton className="h-8 w-full" />
      <div className="grid grid-cols-2 gap-3">
        {Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-18" />)}
      </div>
      <Skeleton className="h-48 w-full" />
    </div>
  )
}

function WidgetError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
      <AlertCircle className="size-7 text-danger" />
      <div>
        <p className="text-sm font-medium">指标加载失败</p>
        <p className="mt-1 max-w-md text-xs text-muted-foreground">{message}</p>
      </div>
      <Button size="sm" variant="outline" onClick={onRetry}><RefreshCw />重试</Button>
    </div>
  )
}

function TrendTooltip({ active, payload, metric }: { active?: boolean; payload?: Array<{ payload: { fullTime: string; value: number } }>; metric: "requests" | "tokens" }) {
  if (!active || !payload?.length) return null
  const point = payload[0].payload
  return (
    <div className="rounded-md border border-border bg-popover px-3 py-2 shadow-md">
      <p className="text-[10px] text-muted-foreground">{point.fullTime}</p>
      <p className="mt-1 font-mono text-sm font-semibold tabular-nums">
        {formatCompact(point.value)} {metric === "tokens" ? "tokens" : "requests"}
      </p>
    </div>
  )
}

function formatCompact(value: number): string {
  if (!Number.isFinite(value)) return "—"
  return new Intl.NumberFormat("zh-CN", { notation: "compact", maximumFractionDigits: 1 }).format(value)
}

function formatPercent(value: number | null, digits: number): string {
  return value == null || !Number.isFinite(value) ? "—" : `${value.toFixed(digits)}%`
}

function formatDuration(value: number | null): string {
  if (value == null || !Number.isFinite(value)) return "—"
  if (value < 1000) return `${Math.round(value)} ms`
  return `${(value / 1000).toFixed(value < 10000 ? 2 : 1)} s`
}

function formatRate(value: number): string {
  if (!Number.isFinite(value)) return "—"
  return value >= 100 ? value.toFixed(0) : value >= 10 ? value.toFixed(1) : value.toFixed(2)
}

function formatClock(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" })
}

function formatBucket(value: string, range: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  if (range === "7d" || range === "30d") return `${date.getMonth() + 1}/${date.getDate()}`
  return date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })
}
