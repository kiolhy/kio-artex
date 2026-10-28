"use client";

import * as React from "react";
import { Loader2, RefreshCw, Save } from "lucide-react";
import { toast } from "sonner";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { api } from "@/lib/api";
import type { NotificationChannel, WorkSettings } from "@/lib/types";

export function ReminderDefaultsCard() {
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const [loadError, setLoadError] = React.useState("");
  const [minutes, setMinutes] = React.useState("10");
  const [channelId, setChannelId] = React.useState<string>("none");
  const [channels, setChannels] = React.useState<NotificationChannel[]>([]);

  const load = React.useCallback(() => {
    setLoading(true);
    setLoadError("");
    Promise.all([api.workSettings(), api.notifyChannels()])
      .then(([settings, list]) => {
        setMinutes(String(settings.default_reminder_minutes ?? 10));
        setChannelId(settings.default_channel_id ? String(settings.default_channel_id) : "none");
        setChannels(list.filter((channel) => channel.enabled));
      })
      .catch((error) => {
        setLoadError((error as Error).message || "读取提醒设置失败");
      })
      .finally(() => setLoading(false));
  }, []);

  React.useEffect(() => {
    load();
  }, [load]);

  async function save() {
    setSaving(true);
    try {
      const value = Math.max(0, Number(minutes) || 0);
      const patch: Partial<Pick<WorkSettings, "default_reminder_minutes" | "default_channel_id">> = {
        default_reminder_minutes: value,
        default_channel_id: channelId === "none" ? null : Number(channelId),
      };
      await api.updateWorkSettings(patch);
      toast.success("提醒默认设置已保存");
    } catch (error) {
      toast.error(`保存失败：${(error as Error).message}`);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>个人提醒默认值</CardTitle>
        <CardDescription>新建待办未单独设置提醒时，会使用这里的提前时间和通知渠道。</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        {loadError ? (
          <Alert variant="destructive">
            <AlertTitle>提醒设置读取失败</AlertTitle>
            <AlertDescription className="space-y-2">
              <p>{loadError}。为避免覆盖现有配置，保存功能已暂时禁用。</p>
              <Button type="button" variant="outline" size="sm" onClick={load} disabled={loading}>
                <RefreshCw className={loading ? "animate-spin" : undefined} />
                重试
              </Button>
            </AlertDescription>
          </Alert>
        ) : loading ? (
          <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
            正在读取提醒设置…
          </p>
        ) : (
          <>
            <div className="grid gap-2">
              <Label htmlFor="reminder-minutes">默认提前提醒（分钟）</Label>
              <Input
                id="reminder-minutes"
                type="number"
                min={0}
                value={minutes}
                onChange={(event) => setMinutes(event.target.value)}
              />
              <p className="text-xs text-muted-foreground">填 0 表示到截止时间时提醒。</p>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="reminder-channel">默认提醒渠道</Label>
              <Select value={channelId} onValueChange={setChannelId}>
                <SelectTrigger id="reminder-channel" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">仅应用内</SelectItem>
                  {channels.map((channel) => (
                    <SelectItem key={channel.id} value={String(channel.id)}>
                      {channel.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button onClick={save} disabled={saving || loading}>
              {saving ? <Loader2 className="animate-spin" /> : <Save />}
              {saving ? "保存中…" : "保存提醒默认值"}
            </Button>
          </>
        )}
      </CardContent>
    </Card>
  );
}
