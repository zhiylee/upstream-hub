"use client"

import { useEffect, useRef, useState } from "react"
import { apiFetch } from "@/lib/api"
import { useManualRefreshTick } from "@/lib/refresh-context"

export interface PollingQueryState<T> {
  data: T | null
  loading: boolean
  refreshing: boolean
  error: string | null
  updatedAt: Date | null
  refetch: () => void
}

export function usePollingQuery<T>(
  path: string | null,
  intervalSeconds: number,
): PollingQueryState<T> {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(path !== null)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)
  const [bump, setBump] = useState(0)
  const manualRefreshTick = useManualRefreshTick()
  const hasDataRef = useRef(false)
  const lastLoadStartedAtRef = useRef(0)
  const previousPathRef = useRef<string | null>(path)
  const safeIntervalSeconds = Number.isFinite(intervalSeconds)
    ? Math.min(3600, Math.max(10, intervalSeconds))
    : 60

  useEffect(() => {
    if (previousPathRef.current !== path) {
      previousPathRef.current = path
      hasDataRef.current = false
      lastLoadStartedAtRef.current = 0
      setData(null)
      setUpdatedAt(null)
    }
    if (path === null) {
      setLoading(false)
      setRefreshing(false)
      return
    }

    let active = true
    let requestRunning = false
    const controller = new AbortController()
    const load = async (force = false) => {
      if (!active || requestRunning || document.visibilityState === "hidden") return
      if (!force && Date.now() - lastLoadStartedAtRef.current < safeIntervalSeconds * 1000) return
      requestRunning = true
      lastLoadStartedAtRef.current = Date.now()
      if (hasDataRef.current) setRefreshing(true)
      else setLoading(true)
      setError(null)
      try {
        const next = await apiFetch<T>(path, { signal: controller.signal })
        if (!active) return
        hasDataRef.current = true
        setData(next)
        setUpdatedAt(new Date())
      } catch (error) {
        if (!active) return
        setError((error as Error).message || "加载失败")
      } finally {
        requestRunning = false
        if (active) {
          setLoading(false)
          setRefreshing(false)
        }
      }
    }

    void load(true)
    const interval = window.setInterval(
      () => void load(true),
      safeIntervalSeconds * 1000,
    )
    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") void load()
    }
    document.addEventListener("visibilitychange", onVisibilityChange)
    return () => {
      active = false
      controller.abort()
      window.clearInterval(interval)
      document.removeEventListener("visibilitychange", onVisibilityChange)
    }
  }, [path, safeIntervalSeconds, manualRefreshTick, bump])

  return {
    data,
    loading,
    refreshing,
    error,
    updatedAt,
    refetch: () => setBump((value) => value + 1),
  }
}
