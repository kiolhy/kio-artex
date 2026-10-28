"use client";

import * as React from "react";
import { useRouter } from "next/navigation";

import { Skeleton } from "@/components/ui/skeleton";

export default function LegacyAssetsRedirectPage() {
  const router = useRouter();

  React.useEffect(() => {
    router.replace("/work/assets");
  }, [router]);

  return (
    <div className="flex flex-1 flex-col gap-3 py-8" aria-label="正在打开资料资产">
      <Skeleton className="h-8 w-48" />
      <Skeleton className="h-36 w-full rounded-[24px]" />
    </div>
  );
}
