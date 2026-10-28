"use client";

import Link from "next/link";
import { BellRing, ArrowRight } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { WorkPageHeader } from "../_components/work-shell";

export default function WorkSettingsPage() {
  return (
    <div className="flex flex-1 flex-col gap-4 md:gap-6">
      <WorkPageHeader
        title="个人提醒"
        description="个人提醒已并入通知推送，便于统一管理应用内提醒、外部渠道和投递记录。"
      />
      <Alert>
        <BellRing />
        <AlertTitle>提醒设置已迁移</AlertTitle>
        <AlertDescription className="space-y-3">
          <p>
            请前往“系统 → 通知推送 → 提醒设置”，配置默认提前时间、默认渠道，并查看投递状态。此旧链接继续保留，
            避免收藏和历史链接失效。
          </p>
          <Button asChild>
            <Link href="/system/notify">
              前往通知推送
              <ArrowRight data-icon="inline-end" />
            </Link>
          </Button>
        </AlertDescription>
      </Alert>
    </div>
  );
}
