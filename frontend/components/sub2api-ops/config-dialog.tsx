"use client"

import { useEffect, useState, type FormEvent } from "react"
import { CheckCircle2, LoaderCircle, PlugZap } from "lucide-react"
import { apiFetch } from "@/lib/api"
import type { Sub2APIOpsConfig } from "@/lib/sub2api-ops-types"
import { Button } from "@/components/ui/button"
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

interface ConfigDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  config: Sub2APIOpsConfig | null
  onSaved: (config: Sub2APIOpsConfig) => void
}

export function Sub2APIOpsConfigDialog({
  open,
  onOpenChange,
  config,
  onSaved,
}: ConfigDialogProps) {
  const [name, setName] = useState("")
  const [siteURL, setSiteURL] = useState("")
  const [adminKey, setAdminKey] = useState("")
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [testPassed, setTestPassed] = useState(false)

  useEffect(() => {
    if (!open) return
    setName(config?.name || "Sub2API")
    setSiteURL(config?.site_url || "")
    setAdminKey("")
    setError(null)
    setTestPassed(false)
  }, [open, config])

  async function handleTest() {
    setTesting(true)
    setError(null)
    setTestPassed(false)
    try {
      await apiFetch<{ ok: boolean }>("/sub2api-ops/config/test", {
        method: "POST",
        body: JSON.stringify({ name, site_url: siteURL, admin_key: adminKey }),
      })
      setTestPassed(true)
    } catch (error) {
      setError((error as Error).message || "连接测试失败")
    } finally {
      setTesting(false)
    }
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSaving(true)
    setError(null)
    try {
      const saved = await apiFetch<Sub2APIOpsConfig>("/sub2api-ops/config", {
        method: "PUT",
        body: JSON.stringify({ name, site_url: siteURL, admin_key: adminKey }),
      })
      onSaved(saved)
      onOpenChange(false)
    } catch (error) {
      setError((error as Error).message || "保存失败")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{config?.configured ? "连接设置" : "连接 Sub2API"}</DialogTitle>
          <DialogDescription>
            Admin Key 由 upstream-hub 后端加密保存，不会返回浏览器。
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="sub2api-ops-name">实例名称</Label>
            <Input
              id="sub2api-ops-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="生产环境"
              maxLength={128}
              required
              disabled={saving || testing}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="sub2api-ops-url">Sub2API 地址</Label>
            <Input
              id="sub2api-ops-url"
              type="url"
              value={siteURL}
              onChange={(event) => {
                setSiteURL(event.target.value)
                setTestPassed(false)
              }}
              placeholder="https://sub2api.example.com"
              required
              disabled={saving || testing}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="sub2api-ops-key">
              {config?.admin_key_configured ? "Admin Key（留空保持不变）" : "Admin Key"}
            </Label>
            <Input
              id="sub2api-ops-key"
              type="password"
              value={adminKey}
              onChange={(event) => {
                setAdminKey(event.target.value)
                setTestPassed(false)
              }}
              placeholder={config?.admin_key_configured ? "已配置" : "admin-..."}
              autoComplete="new-password"
              required={!config?.admin_key_configured}
              disabled={saving || testing}
            />
          </div>

          {error ? (
            <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive" role="alert">
              {error}
            </p>
          ) : null}
          {testPassed ? (
            <p className="flex items-center gap-2 text-sm font-medium text-success">
              <CheckCircle2 className="size-4" />
              连接正常，Admin Key 有效
            </p>
          ) : null}

          <DialogFooter className="sm:justify-between">
            <Button
              type="button"
              variant="outline"
              onClick={handleTest}
              disabled={testing || saving || !siteURL || (!adminKey && !config?.admin_key_configured)}
            >
              {testing ? <LoaderCircle className="animate-spin" /> : <PlugZap />}
              测试连接
            </Button>
            <div className="flex flex-col-reverse gap-2 sm:flex-row">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving || testing}>
                取消
              </Button>
              <Button type="submit" disabled={saving || testing}>
                {saving ? <LoaderCircle className="animate-spin" /> : null}
                保存
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
