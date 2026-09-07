"use client"

import { useEffect, useId, useState, type FormEvent } from "react"
import { ChevronsUpDown, LoaderCircle } from "lucide-react"
import type {
  AccountPlatform,
  AccountStatus,
  AccountsWidgetConfig,
  GroupMetricsWidgetConfig,
  OpsDashboardWidget,
  OpsGroup,
  OpsTimeRange,
  OpsWidgetHeight,
  OpsWidgetWidth,
} from "@/lib/sub2api-ops-types"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"

const ALL = "__all__"

interface WidgetConfigDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  widget: OpsDashboardWidget | null
  groups: OpsGroup[]
  onSave: (widget: OpsDashboardWidget) => Promise<void>
}

const PLATFORM_OPTIONS: Array<{ value: AccountPlatform; label: string }> = [
  { value: "anthropic", label: "Anthropic" },
  { value: "openai", label: "OpenAI" },
  { value: "gemini", label: "Gemini" },
  { value: "antigravity", label: "Antigravity" },
  { value: "grok", label: "Grok" },
]

const STATUS_OPTIONS: Array<{ value: AccountStatus; label: string }> = [
  { value: "active", label: "基础可调度" },
  { value: "inactive", label: "已停用" },
  { value: "error", label: "错误" },
  { value: "rate_limited", label: "限流中" },
  { value: "temp_unschedulable", label: "临时不可调度" },
  { value: "unschedulable", label: "暂停调度" },
]

export function WidgetConfigDialog({
  open,
  onOpenChange,
  widget,
  groups,
  onSave,
}: WidgetConfigDialogProps) {
  const [draft, setDraft] = useState<OpsDashboardWidget | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [slaTargetInput, setSLATargetInput] = useState("")
  const [ttftTargetInput, setTTFTTargetInput] = useState("")

  useEffect(() => {
    if (!open || !widget) return
    setDraft(structuredClone(widget))
    if (widget.type === "group_metrics") {
      const config = widget.config as GroupMetricsWidgetConfig
      setSLATargetInput(String(config.slaTarget))
      setTTFTTargetInput(String(config.ttftP95TargetMs))
    }
    setError(null)
    setSaving(false)
  }, [open, widget])

  if (!draft) return null

  const isGroupMetrics = draft.type === "group_metrics"
  const isAccounts = draft.type === "accounts"
  const groupConfig = draft.config as GroupMetricsWidgetConfig
  const accountsConfig = draft.config as AccountsWidgetConfig

  function updateGroupConfig(patch: Partial<GroupMetricsWidgetConfig>) {
    setDraft((current) => current ? {
      ...current,
      config: { ...(current.config as GroupMetricsWidgetConfig), ...patch },
    } : current)
  }

  function updateAccountsConfig(patch: Partial<AccountsWidgetConfig>) {
    setDraft((current) => current ? {
      ...current,
      config: { ...(current.config as AccountsWidgetConfig), ...patch },
    } : current)
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!draft) return
    let submitted = draft
    if (isGroupMetrics) {
      const slaTarget = parseDecimalInput(slaTargetInput)
      if (slaTarget == null || slaTarget <= 0 || slaTarget > 100) {
        setError("SLA 目标必须在 0.01 到 100 之间")
        return
      }
      const ttftP95TargetMs = parseDecimalInput(ttftTargetInput)
      if (ttftP95TargetMs == null || ttftP95TargetMs < 1 || ttftP95TargetMs > 600_000) {
        setError("TTFT P95 目标必须在 1 到 600000 ms 之间")
        return
      }
      if (groupConfig.groupIds.length > 12) {
        setError("最多选择 12 个分组")
        return
      }
      submitted = {
        ...draft,
        config: { ...groupConfig, slaTarget, ttftP95TargetMs },
      }
    }
    setSaving(true)
    setError(null)
    try {
      await onSave(submitted)
      onOpenChange(false)
    } catch (error) {
      setError((error as Error).message || "组件配置保存失败")
    } finally {
      setSaving(false)
    }
  }

  const title = isGroupMetrics ? "配置分组指标" : isAccounts ? "配置账号组件" : "配置组件"
  const description = isGroupMetrics
    ? "选择统计范围、目标阈值和趋势口径。"
    : "设置账号过滤、排序和信息密度。"

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => {
      if (!saving || nextOpen) onOpenChange(nextOpen)
    }}>
      <DialogContent
        className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
        onEscapeKeyDown={(event) => {
          if (saving) event.preventDefault()
        }}
        onPointerDownOutside={(event) => {
          if (saving) event.preventDefault()
        }}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5 sm:col-span-2">
              <Label htmlFor="widget-title">组件标题</Label>
              <Input
                id="widget-title"
                value={isGroupMetrics ? groupConfig.title : accountsConfig.title}
                maxLength={64}
                onChange={(event) => isGroupMetrics
                  ? updateGroupConfig({ title: event.target.value })
                  : updateAccountsConfig({ title: event.target.value })}
                required
              />
            </div>

            <SelectField
              label="组件宽度"
              value={String(draft.width)}
              onValueChange={(value) => setDraft({ ...draft, width: Number(value) as OpsWidgetWidth })}
              options={(isAccounts ? [8, 12] : [4, 6, 8, 12]).map((value) => ({
                value: String(value),
                label: `${value}/12 栏`,
              }))}
            />
            <SelectField
              label="组件高度"
              value={draft.height}
              onValueChange={(value) => setDraft({ ...draft, height: value as OpsWidgetHeight })}
              options={[
                { value: "compact", label: "紧凑" },
                { value: "standard", label: "标准" },
                { value: "tall", label: "扩展" },
              ]}
            />
          </div>

          {isGroupMetrics ? (
            <div className="grid gap-4 border-t border-border pt-5 sm:grid-cols-2">
              <SelectField
                label="平台"
                value={groupConfig.platform || ALL}
                onValueChange={(value) => {
                  const platform = value === ALL ? "" : value as AccountPlatform
                  updateGroupConfig({
                    platform,
                    groupIds: groupConfig.groupIds.filter((groupID) => {
                      const selectedGroup = groups.find((group) => group.id === groupID)
                      return !selectedGroup || !platform || selectedGroup.platform === platform
                    }),
                  })
                }}
                options={[{ value: ALL, label: "全部平台" }, ...PLATFORM_OPTIONS]}
              />
              <GroupMultiSelect
                label="分组"
                groups={groups}
                platform={groupConfig.platform}
                value={groupConfig.groupIds}
                onChange={(groupIds) => updateGroupConfig({ groupIds })}
                onLimit={() => setError("最多选择 12 个分组")}
              />
              <SelectField
                label="统计窗口"
                value={groupConfig.timeRange}
                onValueChange={(value) => updateGroupConfig({ timeRange: value as OpsTimeRange })}
                options={[
                  { value: "5m", label: "最近 5 分钟" },
                  { value: "30m", label: "最近 30 分钟" },
                  { value: "1h", label: "最近 1 小时" },
                  { value: "6h", label: "最近 6 小时" },
                  { value: "24h", label: "最近 24 小时" },
                  { value: "7d", label: "最近 7 天" },
                  { value: "30d", label: "最近 30 天" },
                ]}
              />
              <SelectField
                label="趋势指标"
                value={groupConfig.trendMetric}
                onValueChange={(value) => updateGroupConfig({ trendMetric: value as "requests" | "tokens" })}
                options={[
                  { value: "requests", label: "请求量" },
                  { value: "tokens", label: "Token 用量" },
                ]}
              />
              <SelectField
                label="最近调度账号数"
                value={String(groupConfig.recentAccountLimit)}
                onValueChange={(value) => updateGroupConfig({ recentAccountLimit: Number(value) })}
                options={[
                  { value: "3", label: "Top 3" },
                  { value: "5", label: "Top 5" },
                  { value: "10", label: "Top 10" },
                  { value: "20", label: "Top 20" },
                ]}
              />
              <NumberField
                label="SLA 目标（%）"
                value={slaTargetInput}
                min={0.01}
                max={100}
                step={0.01}
                onChange={setSLATargetInput}
              />
              <NumberField
                label="TTFT P95 目标（ms）"
                value={ttftTargetInput}
                min={1}
                max={600_000}
                step={1}
                onChange={setTTFTTargetInput}
              />
              <SelectField
                label="自动刷新"
                value={String(groupConfig.refreshSeconds)}
                onValueChange={(value) => updateGroupConfig({ refreshSeconds: Number(value) })}
                options={[
                  { value: "30", label: "30 秒" },
                  { value: "60", label: "1 分钟" },
                  { value: "120", label: "2 分钟" },
                  { value: "300", label: "5 分钟" },
                ]}
              />
            </div>
          ) : null}

          {isAccounts ? (
            <div className="grid gap-4 border-t border-border pt-5 sm:grid-cols-2">
              <SelectField
                label="平台"
                value={accountsConfig.platform || ALL}
                onValueChange={(value) => updateAccountsConfig({ platform: value === ALL ? "" : value as AccountPlatform })}
                options={[{ value: ALL, label: "全部平台" }, ...PLATFORM_OPTIONS]}
              />
              <SelectField
                label="账号类型"
                value={accountsConfig.accountType || ALL}
                onValueChange={(value) => updateAccountsConfig({ accountType: value === ALL ? "" : value as AccountsWidgetConfig["accountType"] })}
                options={[
                  { value: ALL, label: "全部类型" },
                  { value: "oauth", label: "OAuth" },
                  { value: "setup-token", label: "Setup Token" },
                  { value: "apikey", label: "API Key" },
                  { value: "upstream", label: "上游转发" },
                  { value: "bedrock", label: "AWS Bedrock" },
                  { value: "service_account", label: "Service Account" },
                ]}
              />
              <MultiSelectField
                label="运行状态"
                value={accountsConfig.statuses}
                options={STATUS_OPTIONS}
                emptyLabel="全部状态"
                searchPlaceholder="搜索状态"
                onChange={(statuses) => updateAccountsConfig({
                  statuses: statuses as AccountStatus[],
                  excludedStatuses: statuses.length > 0 ? [] : accountsConfig.excludedStatuses,
                })}
              />
              {accountsConfig.statuses.length === 0 ? (
                <MultiSelectField
                  label="排除状态"
                  value={accountsConfig.excludedStatuses}
                  options={STATUS_OPTIONS}
                  emptyLabel="不排除"
                  searchPlaceholder="搜索要排除的状态"
                  selectedPrefix="已排除"
                  onChange={(excludedStatuses) => updateAccountsConfig({ excludedStatuses: excludedStatuses as AccountStatus[] })}
                />
              ) : null}
              <MultiSelectField
                label="账号分组"
                value={accountsConfig.groups}
                emptyLabel="全部分组"
                searchPlaceholder="搜索分组"
                onLimit={() => setError("最多选择 12 个账号分组")}
                onChange={(selectedGroups) => updateAccountsConfig({ groups: selectedGroups })}
                options={[
                  { value: "ungrouped", label: "未分组" },
                  ...groups.map((group) => ({ value: String(group.id), label: group.name, meta: group.platform })),
                  ...accountsConfig.groups
                    .filter((groupID) => groupID !== "ungrouped" && !groups.some((group) => String(group.id) === groupID))
                    .map((groupID) => ({ value: groupID, label: `分组 #${groupID} · 不可用` })),
                ]}
              />
              <SelectField
                label="排序字段"
                value={accountsConfig.sortBy}
                onValueChange={(value) => updateAccountsConfig({ sortBy: value as AccountsWidgetConfig["sortBy"] })}
                options={[
                  { value: "id", label: "账号 ID" },
                  { value: "name", label: "名称" },
                  { value: "status", label: "状态" },
                  { value: "schedulable", label: "是否可调度" },
                  { value: "priority", label: "优先级" },
                  { value: "rate_multiplier", label: "费率倍率" },
                  { value: "current_concurrency", label: "当前并发" },
                  { value: "concurrency", label: "并发上限" },
                  { value: "last_used_at", label: "最近调用" },
                  { value: "created_at", label: "创建时间" },
                  { value: "expires_at", label: "到期时间" },
                ]}
              />
              <SelectField
                label="排序方向"
                value={accountsConfig.sortOrder}
                onValueChange={(value) => updateAccountsConfig({ sortOrder: value as "asc" | "desc" })}
                options={[
                  { value: "asc", label: "升序" },
                  { value: "desc", label: "降序" },
                ]}
              />
              <SelectField
                label="每页行数"
                value={String(accountsConfig.pageSize)}
                onValueChange={(value) => updateAccountsConfig({ pageSize: Number(value) })}
                options={[
                  { value: "10", label: "10 行" },
                  { value: "20", label: "20 行" },
                  { value: "50", label: "50 行" },
                ]}
              />
              <SelectField
                label="自动刷新"
                value={String(accountsConfig.refreshSeconds)}
                onValueChange={(value) => updateAccountsConfig({ refreshSeconds: Number(value) })}
                options={[
                  { value: "30", label: "30 秒" },
                  { value: "60", label: "1 分钟" },
                  { value: "120", label: "2 分钟" },
                  { value: "300", label: "5 分钟" },
                ]}
              />
              <SwitchField
                label="今日 / 总用量"
                checked={accountsConfig.showUsage}
                onCheckedChange={(checked) => updateAccountsConfig({ showUsage: checked })}
              />
              <SwitchField
                label="调度评分"
                checked={accountsConfig.showSchedulerScore}
                onCheckedChange={(checked) => updateAccountsConfig({ showSchedulerScore: checked })}
              />
            </div>
          ) : null}

          {error ? <p className="text-sm text-destructive" role="alert">{error}</p> : null}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>取消</Button>
            <Button type="submit" disabled={saving}>
              {saving ? <LoaderCircle className="animate-spin" /> : null}
              应用配置
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function GroupMultiSelect({
  label,
  groups,
  platform,
  value,
  onChange,
  onLimit,
}: {
  label: string
  groups: OpsGroup[]
  platform: AccountPlatform | ""
  value: number[]
  onChange: (value: number[]) => void
  onLimit: () => void
}) {
  const id = useId()
  const [open, setOpen] = useState(false)
  const options = groups.filter((group) => !platform || group.platform === platform)
  const unavailableIDs = value.filter((groupID) => !groups.some((group) => group.id === groupID))

  function toggle(groupID: number) {
    if (value.includes(groupID)) {
      onChange(value.filter((item) => item !== groupID))
      return
    }
    if (value.length >= 12) {
      onLimit()
      return
    }
    onChange([...value, groupID])
  }

  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            id={id}
            type="button"
            variant="outline"
            role="combobox"
            aria-expanded={open}
            className="w-full justify-between font-normal"
          >
            <span className="truncate">{selectedGroupsLabel(value, groups)}</span>
            <ChevronsUpDown className="size-4 shrink-0 text-muted-foreground" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0" align="start">
          <Command>
            <CommandInput placeholder="搜索分组" />
            <CommandList>
              <CommandEmpty>没有匹配的分组</CommandEmpty>
              <CommandGroup>
                <CommandItem
                  value="全部分组"
                  aria-label={`全部分组，${value.length === 0 ? "已选择" : "未选择"}`}
                  onSelect={() => onChange([])}
                >
                  <Checkbox checked={value.length === 0} tabIndex={-1} aria-hidden="true" className="pointer-events-none" />
                  <span>全部分组</span>
                </CommandItem>
                {options.map((group) => (
                  <CommandItem
                    key={group.id}
                    value={`${group.name} ${group.platform} ${group.id}`}
                    aria-label={`${group.name}，${value.includes(group.id) ? "已选择" : "未选择"}`}
                    onSelect={() => toggle(group.id)}
                  >
                    <Checkbox checked={value.includes(group.id)} tabIndex={-1} aria-hidden="true" className="pointer-events-none" />
                    <span className="min-w-0 flex-1 truncate">{group.name}</span>
                    <span className="text-[10px] text-muted-foreground">{group.platform}</span>
                  </CommandItem>
                ))}
                {unavailableIDs.map((groupID) => (
                  <CommandItem
                    key={groupID}
                    value={`不可用分组 ${groupID}`}
                    aria-label={`分组 ${groupID}，已选择，不可用`}
                    onSelect={() => toggle(groupID)}
                  >
                    <Checkbox checked tabIndex={-1} aria-hidden="true" className="pointer-events-none" />
                    <span>分组 #{groupID} · 不可用</span>
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  )
}

function MultiSelectField({
  label,
  value,
  options,
  emptyLabel,
  searchPlaceholder,
  selectedPrefix,
  onChange,
  onLimit,
}: {
  label: string
  value: string[]
  options: Array<{ value: string; label: string; meta?: string }>
  emptyLabel: string
  searchPlaceholder: string
  selectedPrefix?: string
  onChange: (value: string[]) => void
  onLimit?: () => void
}) {
  const id = useId()
  const [open, setOpen] = useState(false)

  function toggle(optionValue: string) {
    if (value.includes(optionValue)) {
      onChange(value.filter((item) => item !== optionValue))
      return
    }
    if (value.length >= 12) {
      onLimit?.()
      return
    }
    onChange([...value, optionValue])
  }

  const selectedLabels = value.map((selected) => options.find((option) => option.value === selected)?.label ?? selected)
  let valueLabel = emptyLabel
  if (selectedLabels.length > 0) {
    const selection = selectedLabels.length <= 2 ? selectedLabels.join(" / ") : `${selectedLabels.length} 项`
    valueLabel = selectedPrefix ? `${selectedPrefix} ${selection}` : selection
  }

  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            id={id}
            type="button"
            variant="outline"
            role="combobox"
            aria-expanded={open}
            className="w-full justify-between font-normal"
          >
            <span className="truncate">{valueLabel}</span>
            <ChevronsUpDown className="size-4 shrink-0 text-muted-foreground" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0" align="start">
          <Command>
            <CommandInput placeholder={searchPlaceholder} />
            <CommandList>
              <CommandEmpty>没有匹配项</CommandEmpty>
              <CommandGroup>
                <CommandItem
                  value={`${emptyLabel} all`}
                  aria-label={`${emptyLabel}，${value.length === 0 ? "已选择" : "未选择"}`}
                  onSelect={() => onChange([])}
                >
                  <Checkbox checked={value.length === 0} tabIndex={-1} aria-hidden="true" className="pointer-events-none" />
                  <span>{emptyLabel}</span>
                </CommandItem>
                {options.map((option) => (
                  <CommandItem
                    key={option.value}
                    value={`${option.label} ${option.meta ?? ""} ${option.value}`}
                    aria-label={`${option.label}，${value.includes(option.value) ? "已选择" : "未选择"}`}
                    onSelect={() => toggle(option.value)}
                  >
                    <Checkbox checked={value.includes(option.value)} tabIndex={-1} aria-hidden="true" className="pointer-events-none" />
                    <span className="min-w-0 flex-1 truncate">{option.label}</span>
                    {option.meta ? <span className="text-[10px] text-muted-foreground">{option.meta}</span> : null}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  )
}

function selectedGroupsLabel(value: number[], groups: OpsGroup[]): string {
  if (value.length === 0) return "全部分组"
  const labels = value.map((groupID) => groups.find((group) => group.id === groupID)?.name ?? `分组 #${groupID}`)
  if (labels.length <= 2) return labels.join(" / ")
  return `已选择 ${labels.length} 个分组`
}

function SelectField({
  label,
  value,
  options,
  onValueChange,
}: {
  label: string
  value: string
  options: Array<{ value: string; label: string }>
  onValueChange: (value: string) => void
}) {
  const id = useId()
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Select value={value} onValueChange={onValueChange}>
        <SelectTrigger id={id} className="w-full"><SelectValue /></SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

function NumberField({
  label,
  value,
  min,
  max,
  step,
  onChange,
}: {
  label: string
  value: string
  min: number
  max?: number
  step: number
  onChange: (value: string) => void
}) {
  const id = useId()
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type="number"
        value={value}
        min={min}
        max={max}
        step={step}
        inputMode="decimal"
        onChange={(event) => onChange(event.target.value)}
        required
      />
    </div>
  )
}

function parseDecimalInput(raw: string): number | null {
  const value = raw.trim()
  if (!/^(?:\d+(?:\.\d*)?|\.\d+)$/.test(value)) return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

function SwitchField({
  label,
  checked,
  onCheckedChange,
}: {
  label: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
}) {
  const id = useId()
  return (
    <div className="flex min-h-10 items-center justify-between rounded-md border border-border px-3">
      <Label htmlFor={id}>{label}</Label>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  )
}
