import { useEffect, useMemo, useState } from "react"
import { useTheme } from "next-themes"
import { Activity, Github, LayoutDashboard, LogOut, Menu, RefreshCw, ServerCog, Sun, Moon } from "lucide-react"
import { NavLink, useLocation } from "react-router-dom"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { useAuth } from "@/lib/auth-context"
import { useTriggerRefresh } from "@/lib/refresh-context"
import { useChannels } from "@/lib/queries"
import { relativeTime } from "@/lib/format"

export function MonitorHeader() {
  const { theme, setTheme } = useTheme()
  const { username, authDisabled, logout } = useAuth()
  const refresh = useTriggerRefresh()
  const channels = useChannels()
  const location = useLocation()
  const [mounted, setMounted] = useState(false)
  const [syncing, setSyncing] = useState(false)

  useEffect(() => setMounted(true), [])

  /**
   * 找出所有渠道中最近一次采集时间——这是"上次采集"展示的依据，
   * 让用户知道页面上的余额到底是多新的快照（区别于"我刚点了刷新"）。
   */
  const lastCollectedAt = useMemo(() => {
    const list = channels.data ?? []
    let best: string | null = null
    let bestT = -Infinity
    for (const c of list) {
      if (!c.last_balance_at) continue
      const t = new Date(c.last_balance_at).getTime()
      if (Number.isFinite(t) && t > bestT) {
        bestT = t
        best = c.last_balance_at
      }
    }
    return best
  }, [channels.data])

  function handleRefresh() {
    setSyncing(true)
    refresh()
    setTimeout(() => setSyncing(false), 800)
  }

  return (
    <header className="sticky top-0 z-20 border-b border-border bg-background/95 backdrop-blur-sm">
      <div className="mx-auto flex h-14 max-w-360 items-center justify-between gap-4 px-5">
        {/* left: logo + title */}
        <div className="flex min-w-0 items-center gap-2.5">
          <div className="flex size-8 items-center justify-center rounded-lg bg-foreground text-background">
            <Activity className="size-4" strokeWidth={2.5} />
          </div>
          <h1 className="hidden text-base font-semibold tracking-tight text-foreground sm:block">Upstream-hub</h1>

          <nav className="ml-3 hidden items-center gap-1 md:flex" aria-label="主导航">
            <HeaderNavLink to="/" end icon={<LayoutDashboard />}>渠道监控</HeaderNavLink>
            {authDisabled ? null : (
              <HeaderNavLink to="/sub2api-ops" icon={<ServerCog />}>Sub2API 运维</HeaderNavLink>
            )}
          </nav>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" className="size-8 md:hidden" aria-label="打开导航">
                <Menu />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-48">
              <DropdownMenuLabel>监控视图</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem asChild><NavLink to="/"><LayoutDashboard />渠道监控</NavLink></DropdownMenuItem>
              {authDisabled ? null : (
                <DropdownMenuItem asChild><NavLink to="/sub2api-ops"><ServerCog />Sub2API 运维</NavLink></DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        {/* right: actions */}
        <div className="flex items-center gap-3">
          {/* last collected + refresh */}
          <div className="hidden items-center gap-2 lg:flex">
            <span className="text-xs text-muted-foreground">
              {location.pathname.startsWith("/sub2api-ops") ? "运维数据按组件刷新" : (
                <>{"上次采集 "}<span className="font-medium text-foreground">{relativeTime(lastCollectedAt)}</span></>
              )}
            </span>
            <Tooltip delayDuration={200}>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleRefresh}
                  disabled={syncing}
                  className="gap-1.5 border-border bg-background text-foreground hover:bg-muted"
                  aria-label="刷新视图"
                >
                  <RefreshCw className={cn("size-3.5", syncing && "animate-spin")} />
                  {"刷新视图"}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom" className="max-w-xs text-xs">
                <p>{"重新拉取最新的快照数据。"}</p>
                <p className="mt-1 text-muted-foreground">
                  {"提示：实际采集由后台定时任务执行，如需立即采集请到具体渠道点 \"同步\"。"}
                </p>
              </TooltipContent>
            </Tooltip>
          </div>

          {/* mobile-only refresh (no tooltip / no timestamp to save space) */}
          <Button
            variant="outline"
            size="sm"
            onClick={handleRefresh}
            disabled={syncing}
            className="gap-1.5 border-border bg-background text-foreground hover:bg-muted lg:hidden"
            aria-label="刷新视图"
          >
            <RefreshCw className={cn("size-3.5", syncing && "animate-spin")} />
            {"刷新"}
          </Button>

          {/* GitHub repo link */}
          <Tooltip delayDuration={200}>
            <TooltipTrigger asChild>
              <Button
                asChild
                variant="outline"
                size="icon"
                className="hidden size-8 border-border bg-background text-foreground hover:bg-muted xl:inline-flex"
                aria-label="GitHub 仓库"
              >
                <a
                  href="https://github.com/worryzyy/upstream-hub"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <Github className="size-3.5" />
                </a>
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="text-xs">
              {"GitHub · worryzyy/upstream-hub"}
            </TooltipContent>
          </Tooltip>

          {/* theme toggle */}
          <Tooltip delayDuration={200}>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
                className="size-8 border-border bg-background text-foreground hover:bg-muted"
                aria-label="切换主题"
              >
                {mounted && theme === "dark" ? (
                  <Moon className="size-3.5" />
                ) : (
                  <Sun className="size-3.5" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="text-xs">
              {mounted && theme === "dark" ? "深色模式 · 点击切换浅色" : "浅色模式 · 点击切换深色"}
            </TooltipContent>
          </Tooltip>

          {/* logout — 鉴权关闭时整个按钮不显示 */}
          {authDisabled ? null : (
            <Tooltip delayDuration={200}>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="icon"
                  onClick={logout}
                  className="size-8 border-border bg-background text-foreground hover:bg-muted"
                  aria-label="退出登录"
                >
                  <LogOut className="size-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom" className="text-xs">
                {username ? `${username} · 退出登录` : "退出登录"}
              </TooltipContent>
            </Tooltip>
          )}
        </div>
      </div>
    </header>
  )
}

function HeaderNavLink({
  to,
  end,
  icon,
  children,
}: {
  to: string
  end?: boolean
  icon: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) => cn(
        "flex h-8 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium transition-colors [&_svg]:size-3.5",
        isActive ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-muted/70 hover:text-foreground",
      )}
    >
      {icon}
      {children}
    </NavLink>
  )
}
