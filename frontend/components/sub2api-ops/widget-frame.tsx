"use client"

import type { DragEvent, ReactNode } from "react"
import {
  ArrowLeft,
  ArrowRight,
  GripVertical,
  Settings2,
  Trash2,
} from "lucide-react"
import type { OpsDashboardWidget } from "@/lib/sub2api-ops-types"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

const WIDTH_CLASSES: Record<number, string> = {
  4: "md:col-span-6 xl:col-span-4",
  6: "md:col-span-6",
  8: "md:col-span-12 xl:col-span-8",
  12: "md:col-span-12",
}

const HEIGHT_CLASSES = {
  compact: "h-90",
  standard: "h-125",
  tall: "h-170",
}

interface WidgetFrameProps {
  widget: OpsDashboardWidget
  title: string
  subtitle: string
  icon: ReactNode
  editing: boolean
  isFirst: boolean
  isLast: boolean
  children: ReactNode
  onConfigure: () => void
  onMove: (direction: -1 | 1) => void
  onRemove: () => void
  onDragStart: (event: DragEvent<HTMLElement>) => void
  onDragEnd: () => void
  onDragOver: (event: DragEvent<HTMLElement>) => void
  onDrop: (event: DragEvent<HTMLElement>) => void
}

export function WidgetFrame({
  widget,
  title,
  subtitle,
  icon,
  editing,
  isFirst,
  isLast,
  children,
  onConfigure,
  onMove,
  onRemove,
  onDragStart,
  onDragEnd,
  onDragOver,
  onDrop,
}: WidgetFrameProps) {
  return (
    <section
      draggable={editing}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDragOver={onDragOver}
      onDrop={onDrop}
      className={cn(
        "@container/widget col-span-12 flex min-w-0 flex-col overflow-hidden rounded-lg border bg-card shadow-none transition-colors",
        WIDTH_CLASSES[widget.width] ?? WIDTH_CLASSES[12],
        HEIGHT_CLASSES[widget.height],
        editing && "border-dashed border-foreground/30 hover:border-foreground/60",
      )}
    >
      <header className="flex h-15 shrink-0 items-center justify-between gap-3 border-b border-border px-4">
        <div className="flex min-w-0 items-center gap-2.5">
          {editing ? <GripVertical className="size-4 shrink-0 cursor-grab text-muted-foreground" /> : null}
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-foreground">
            {icon}
          </span>
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold text-foreground">{title}</h2>
            <p className="truncate text-[11px] text-muted-foreground">{subtitle}</p>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-1">
          {editing ? (
            <>
              <IconButton label="前移" disabled={isFirst} onClick={() => onMove(-1)}>
                <ArrowLeft />
              </IconButton>
              <IconButton label="后移" disabled={isLast} onClick={() => onMove(1)}>
                <ArrowRight />
              </IconButton>
              <IconButton label="删除组件" destructive onClick={onRemove}>
                <Trash2 />
              </IconButton>
            </>
          ) : null}
          <IconButton label="配置组件" onClick={onConfigure}>
            <Settings2 />
          </IconButton>
        </div>
      </header>
      <div className="min-h-0 flex-1">{children}</div>
    </section>
  )
}

function IconButton({
  label,
  disabled,
  destructive,
  onClick,
  children,
}: {
  label: string
  disabled?: boolean
  destructive?: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          disabled={disabled}
          onClick={onClick}
          className={cn("size-8", destructive && "text-destructive hover:bg-destructive/10 hover:text-destructive")}
          aria-label={label}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="bottom">{label}</TooltipContent>
    </Tooltip>
  )
}
