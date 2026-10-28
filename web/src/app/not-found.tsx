"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";

export default function NotFound() {
  const router = useRouter();

  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-4 px-6 text-center">
      <p className="text-sm font-medium text-muted-foreground">404</p>
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold">页面不存在</h1>
        <p className="text-muted-foreground">这个地址可能已失效，或页面已经被移动。</p>
      </div>
      <div className="flex flex-wrap items-center justify-center gap-2">
        <Button type="button" variant="outline" onClick={() => router.back()}>
          返回上一页
        </Button>
        <Link prefetch={false} replace href="/dashboard">
          <Button>返回工作台</Button>
        </Link>
      </div>
    </div>
  );
}
