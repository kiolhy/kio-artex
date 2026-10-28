"use client";

import { Bell, BellOff, BellRing } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { useBrowserNotificationPermission } from "@/hooks/use-work-reminder-feed";

export function BrowserReminderButton() {
  const { state, request } = useBrowserNotificationPermission();
  const granted = state === "granted";
  const unsupported = state === "unsupported";
  const denied = state === "denied";
  let title = "启用浏览器系统通知";
  let buttonLabel = "启用浏览器提醒";
  let Icon = Bell;
  if (unsupported) {
    title = "当前浏览器不支持系统通知";
    buttonLabel = "不支持系统通知";
    Icon = BellOff;
  } else if (granted) {
    title = "浏览器提醒已启用";
    buttonLabel = "浏览器提醒已开";
    Icon = BellRing;
  } else if (denied) {
    title = "请在浏览器站点设置中允许通知";
    buttonLabel = "浏览器提醒已关闭";
    Icon = BellOff;
  }

  return (
    <Button
      variant="outline"
      size="sm"
      className="h-10 sm:h-7"
      disabled={unsupported || granted}
      onClick={() => {
        if (denied) {
          toast.error("请在浏览器站点设置中重新允许通知");
          return;
        }
        void request().then((result) => {
          if (result === "granted") toast.success("浏览器提醒已启用");
          if (result === "denied") toast.error("浏览器提醒被拒绝，可在浏览器站点设置中重新开启");
        });
      }}
      title={title}
      aria-label={title}
    >
      <Icon className="size-4" aria-hidden="true" />
      {buttonLabel}
    </Button>
  );
}
