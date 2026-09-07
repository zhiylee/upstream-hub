"use client"

import { useEffect, useRef, useState, type DragEvent } from "react"
import {
  Activity,
  Boxes,
  Check,
  LayoutDashboard,
  LoaderCircle,
  Plus,
  RefreshCw,
  ServerCog,
  Settings2,
  Users,
} from "lucide-react"
import { toast } from "sonner"
import { apiFetch } from "@/lib/api"
import { useTriggerRefresh } from "@/lib/refresh-context"
import { usePollingQuery } from "@/lib/sub2api-ops-query"
import {
  createDefaultOpsLayout,
  createOpsWidget,
  normalizeOpsLayout,
  type AccountsWidgetConfig,
  type GroupMetricsWidgetConfig,
  type OpsDashboardLayout,
  type OpsDashboardWidget,
  type OpsGroup,
  type OpsWidgetType,
  type Sub2APIOpsConfig,
} from "@/lib/sub2api-ops-types"
import { AccountsWidget } from "@/components/sub2api-ops/accounts-widget"
import { Sub2APIOpsConfigDialog } from "@/components/sub2api-ops/config-dialog"
import { GroupMetricsWidget } from "@/components/sub2api-ops/group-metrics-widget"
import { WidgetConfigDialog } from "@/components/sub2api-ops/widget-config-dialog"
import { WidgetFrame } from "@/components/sub2api-ops/widget-frame"
import { Button } from "@/components/ui/button"
import { useConfirm } from "@/components/ui/confirm-dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"

export default function Sub2APIOpsPage() {
  const [config, setConfig] = useState<Sub2APIOpsConfig | null>(null)
  const [layout, setLayout] = useState<OpsDashboardLayout>(() => createDefaultOpsLayout())
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [loadAttempt, setLoadAttempt] = useState(0)
  const [configOpen, setConfigOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [savingLayout, setSavingLayout] = useState(false)
  const [selectedWidget, setSelectedWidget] = useState<OpsDashboardWidget | null>(null)
  const [draggedWidgetID, setDraggedWidgetID] = useState<string | null>(null)
  const layoutRef = useRef(layout)
  const confirmedLayoutRef = useRef(layout)
  const saveChainRef = useRef<Promise<void>>(Promise.resolve())
  const pendingSavesRef = useRef(0)
  const refresh = useTriggerRefresh()
  const { confirm, dialog: confirmDialog } = useConfirm()

  useEffect(() => {
    let active = true
    setLoading(true)
    setLoadError(null)
    apiFetch<Sub2APIOpsConfig>("/sub2api-ops/config")
      .then((response) => {
        if (!active) return
        setConfig(response)
        const next = normalizeOpsLayout(response.layout)
        applyLocalLayout(next)
        confirmedLayoutRef.current = next
        if (!response.configured) setConfigOpen(true)
      })
      .catch((error) => {
        if (active) setLoadError((error as Error).message || "配置加载失败")
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [loadAttempt])

  const groupsQuery = usePollingQuery<OpsGroup[]>(
    config?.configured ? "/sub2api-ops/groups" : null,
    300,
  )
  const groups = groupsQuery.data ?? []

  function applyLocalLayout(next: OpsDashboardLayout) {
    layoutRef.current = next
    setLayout(next)
  }

  function persistLayout(next: OpsDashboardLayout): Promise<void> {
    applyLocalLayout(next)
    pendingSavesRef.current += 1
    setSavingLayout(true)
    const operation = saveChainRef.current
      .catch(() => undefined)
      .then(async () => {
        await apiFetch<OpsDashboardLayout>("/sub2api-ops/layout", {
          method: "PUT",
          body: JSON.stringify(next),
        })
        confirmedLayoutRef.current = next
      })
      .catch((error) => {
        if (layoutRef.current === next) applyLocalLayout(confirmedLayoutRef.current)
        toast.error("大盘布局保存失败", { description: (error as Error).message })
        throw error
      })
      .finally(() => {
        pendingSavesRef.current -= 1
        if (pendingSavesRef.current === 0) setSavingLayout(false)
      })
    saveChainRef.current = operation.catch(() => undefined)
    return operation
  }

  async function handleConfigSaved(saved: Sub2APIOpsConfig) {
    setConfig(saved)
    const next = normalizeOpsLayout(saved.layout)
    applyLocalLayout(next)
    if (saved.layout) confirmedLayoutRef.current = next
    if (!saved.layout) {
      try {
        await persistLayout(next)
      } catch {
        return
      }
    }
    toast.success("Sub2API 连接已保存")
    refresh()
  }

  async function addWidget(type: OpsWidgetType) {
    const widget = createOpsWidget(type)
    const current = layoutRef.current
    const next = { ...current, widgets: [...current.widgets, widget] }
    try {
      await persistLayout(next)
      setSelectedWidget(widget)
    } catch {
      return
    }
  }

  async function saveWidget(widget: OpsDashboardWidget) {
    const current = layoutRef.current
    const next = {
      ...current,
      widgets: current.widgets.map((item) => item.id === widget.id ? widget : item),
    }
    await persistLayout(next)
  }

  async function removeWidget(widget: OpsDashboardWidget) {
    const accepted = await confirm({
      title: "删除组件？",
      description: `“${widgetTitle(widget)}”及其配置将从大盘移除。`,
      confirmLabel: "删除",
      destructive: true,
    })
    if (!accepted) return
    const current = layoutRef.current
    try {
      await persistLayout({
        ...current,
        widgets: current.widgets.filter((item) => item.id !== widget.id),
      })
    } catch {
      return
    }
  }

  function moveWidget(index: number, direction: -1 | 1) {
    const target = index + direction
    const current = layoutRef.current
    if (target < 0 || target >= current.widgets.length) return
    const widgets = [...current.widgets]
    const [widget] = widgets.splice(index, 1)
    widgets.splice(target, 0, widget)
    applyLocalLayout({ ...current, widgets })
  }

  function dropWidget(targetID: string) {
    if (!draggedWidgetID || draggedWidgetID === targetID) return
    const current = layoutRef.current
    const sourceIndex = current.widgets.findIndex((widget) => widget.id === draggedWidgetID)
    const targetIndex = current.widgets.findIndex((widget) => widget.id === targetID)
    if (sourceIndex < 0 || targetIndex < 0) return
    const widgets = [...current.widgets]
    const [widget] = widgets.splice(sourceIndex, 1)
    widgets.splice(targetIndex, 0, widget)
    applyLocalLayout({ ...current, widgets })
    setDraggedWidgetID(null)
  }

  async function finishEditing() {
    const next = layoutRef.current
    setEditing(false)
    try {
      await persistLayout(next)
    } catch {
      setEditing(true)
      return
    }
  }

  if (loading) return <PageSkeleton />
  if (loadError) {
    return (
      <div className="flex min-h-80 flex-col items-center justify-center gap-3 text-center">
        <ServerCog className="size-8 text-danger" />
        <div>
          <h1 className="text-base font-semibold">运维配置加载失败</h1>
          <p className="mt-1 text-sm text-muted-foreground">{loadError}</p>
        </div>
        <Button size="sm" variant="outline" onClick={() => setLoadAttempt((value) => value + 1)}>
          <RefreshCw />重试
        </Button>
      </div>
    )
  }

  return (
    <section className="space-y-4">
      <header className="flex flex-col gap-3 border-b border-border pb-4 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h1 className="text-lg font-semibold text-foreground">Sub2API 运维</h1>
            {config?.configured ? (
              <span className="inline-flex items-center gap-1.5 rounded-md bg-success/10 px-2 py-1 text-[11px] font-medium text-success">
                <span className="size-1.5 rounded-full bg-success" />
                已连接
              </span>
            ) : (
              <span className="inline-flex items-center gap-1.5 rounded-md bg-warning/10 px-2 py-1 text-[11px] font-medium text-warning">
                <span className="size-1.5 rounded-full bg-warning" />
                未配置
              </span>
            )}
          </div>
          <p className="mt-1 truncate text-xs text-muted-foreground">
            {config?.configured ? `${config.name} · ${config.site_url}` : "配置管理连接后开始采集实时指标"}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {savingLayout ? (
            <span className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
              <LoaderCircle className="size-3 animate-spin" />保存中
            </span>
          ) : null}
          {config?.configured ? (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm"><Plus />添加组件</Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-52">
                <DropdownMenuLabel>组件库</DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem onSelect={() => void addWidget("group_metrics")}>
                  <Activity />分组服务指标
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => void addWidget("accounts")}>
                  <Users />账号运行状态
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          ) : null}
          {config?.configured ? (
            editing ? (
              <Button size="sm" onClick={() => void finishEditing()} disabled={savingLayout}>
                <Check />完成布局
              </Button>
            ) : (
              <Button variant="outline" size="sm" onClick={() => setEditing(true)} disabled={savingLayout}>
                <LayoutDashboard />调整布局
              </Button>
            )
          ) : null}
          <Button variant="outline" size="icon" className="size-8" onClick={() => setConfigOpen(true)} aria-label="连接设置">
            <Settings2 />
          </Button>
        </div>
      </header>

      {!config?.configured ? (
        <div className="flex h-72 flex-col items-center justify-center rounded-lg border border-dashed border-border px-6 text-center">
          <span className="flex size-11 items-center justify-center rounded-lg bg-muted">
            <ServerCog className="size-5 text-muted-foreground" />
          </span>
          <h2 className="mt-4 text-base font-semibold">尚未连接 Sub2API</h2>
          <Button className="mt-4" onClick={() => setConfigOpen(true)}><Settings2 />配置连接</Button>
        </div>
      ) : layout.widgets.length === 0 ? (
        <div className="flex h-72 flex-col items-center justify-center rounded-lg border border-dashed border-border px-6 text-center">
          <Boxes className="size-8 text-muted-foreground/60" />
          <h2 className="mt-3 text-sm font-semibold">大盘暂无组件</h2>
          <Button className="mt-4" size="sm" variant="outline" onClick={() => void addWidget("group_metrics")}>
            <Plus />添加组件
          </Button>
        </div>
      ) : (
        <div className="grid grid-cols-12 gap-3">
          {layout.widgets.map((widget, index) => (
            <WidgetFrame
              key={widget.id}
              widget={widget}
              title={widgetTitle(widget)}
              subtitle={widgetSubtitle(widget, groups)}
              icon={widget.type === "accounts" ? <Users className="size-4" /> : <Activity className="size-4" />}
              editing={editing}
              isFirst={index === 0}
              isLast={index === layout.widgets.length - 1}
              onConfigure={() => setSelectedWidget(widget)}
              onMove={(direction) => moveWidget(index, direction)}
              onRemove={() => void removeWidget(widget)}
              onDragStart={(event: DragEvent<HTMLElement>) => {
                event.dataTransfer.effectAllowed = "move"
                setDraggedWidgetID(widget.id)
              }}
              onDragEnd={() => setDraggedWidgetID(null)}
              onDragOver={(event) => {
                if (editing) event.preventDefault()
              }}
              onDrop={(event) => {
                event.preventDefault()
                dropWidget(widget.id)
              }}
            >
              {renderWidget(widget)}
            </WidgetFrame>
          ))}
        </div>
      )}

      <Sub2APIOpsConfigDialog
        open={configOpen}
        onOpenChange={setConfigOpen}
        config={config}
        onSaved={(saved) => void handleConfigSaved(saved)}
      />
      <WidgetConfigDialog
        open={selectedWidget !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedWidget(null)
        }}
        widget={selectedWidget}
        groups={groups}
        onSave={saveWidget}
      />
      {confirmDialog}
    </section>
  )
}

function renderWidget(widget: OpsDashboardWidget) {
  if (widget.type === "group_metrics") {
    return (
      <GroupMetricsWidget
        config={widget.config as GroupMetricsWidgetConfig}
        width={widget.width}
        height={widget.height}
      />
    )
  }
  if (widget.type === "accounts") {
    return <AccountsWidget config={widget.config as AccountsWidgetConfig} height={widget.height} />
  }
  return (
    <div className="flex h-full items-center justify-center px-6 text-center text-sm text-muted-foreground">
      当前版本不支持组件类型 {widget.type}
    </div>
  )
}

function widgetTitle(widget: OpsDashboardWidget): string {
  const config = widget.config as { title?: string }
  return config.title || (widget.type === "accounts" ? "账号运行状态" : "服务指标")
}

function widgetSubtitle(widget: OpsDashboardWidget, groups: OpsGroup[]): string {
  if (widget.type === "group_metrics") {
    const config = widget.config as GroupMetricsWidgetConfig
    const selected = config.groupIds.map((groupID) => groups.find((item) => item.id === groupID)?.name ?? `分组 #${groupID}`)
    const scope = selected.length === 0
      ? "全部分组"
      : selected.length <= 2 ? selected.join(" / ") : `${selected.length} 个分组`
    return `${scope} · ${config.platform || "全部平台"} · ${timeRangeLabel(config.timeRange)} · Top ${config.recentAccountLimit}`
  }
  if (widget.type === "accounts") {
    const config = widget.config as AccountsWidgetConfig
    const selected = config.groups.map((groupID) => groupID === "ungrouped"
      ? "未分组"
      : groups.find((item) => String(item.id) === groupID)?.name ?? `分组 #${groupID}`)
    const scope = selected.length === 0
      ? "全部分组"
      : selected.length <= 2 ? selected.join(" / ") : `${selected.length} 个分组`
    return `${scope} · ${config.platform || "全部平台"} · ${config.pageSize} 行`
  }
  return widget.type
}

function timeRangeLabel(value: string): string {
  return {
    "5m": "5 分钟",
    "30m": "30 分钟",
    "1h": "1 小时",
    "6h": "6 小时",
    "24h": "24 小时",
    "7d": "7 天",
    "30d": "30 天",
  }[value] ?? value
}

function PageSkeleton() {
  return (
    <section className="space-y-4">
      <div className="flex flex-col gap-3 border-b border-border pb-4 sm:flex-row sm:items-end sm:justify-between">
        <div className="space-y-2"><Skeleton className="h-6 w-40 max-w-full" /><Skeleton className="h-4 w-72 max-w-full" /></div>
        <Skeleton className="h-8 w-64 max-w-full" />
      </div>
      <div className="grid grid-cols-12 gap-3">
        <Skeleton className="col-span-12 h-125 md:col-span-6" />
        <Skeleton className="col-span-12 h-125 md:col-span-6" />
        <Skeleton className="col-span-12 h-170" />
      </div>
    </section>
  )
}
