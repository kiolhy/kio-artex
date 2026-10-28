"use client";

import * as React from "react";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { SortableContext, arrayMove, rectSortingStrategy, useSortable } from "@dnd-kit/sortable";
import { EyeOff, GripVertical, Maximize2, Minimize2, Settings2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import type { WorkDashboardCardLayout } from "@/lib/types";
import { QuickCaptureCard } from "./quick-capture-card";
import { WorkCalendarCard } from "./work-calendar-card";
import { WorkFocusCard } from "./work-focus-card";
import { WorkAutomationCard } from "./work-automation-card";
import { PersonalAssetsCard } from "./personal-assets-card";
import { DASHBOARD_CARD_LABELS, type DashboardCardType } from "./use-dashboard-layout";

function cardContent(type: DashboardCardType, onTaskCreated: () => void, refreshKey: number) {
  switch (type) {
    case "personal-focus":
      return <WorkFocusCard refreshKey={refreshKey} />;
    case "personal-quick-capture":
      return <QuickCaptureCard onCreated={onTaskCreated} />;
    case "personal-calendar":
      return <WorkCalendarCard refreshKey={refreshKey} />;
    case "personal-automation":
      return <WorkAutomationCard refreshKey={refreshKey} />;
    case "personal-assets":
      return <PersonalAssetsCard refreshKey={refreshKey} />;
    default:
      return null;
  }
}

function spanClass(span: WorkDashboardCardLayout["span"]) {
  if (span >= 4) return "xl:col-span-4";
  if (span === 3) return "xl:col-span-3";
  if (span === 2) return "xl:col-span-2";
  return "xl:col-span-1";
}

function SortableCard({
  card,
  children,
  draggable,
  resizable,
  onHide,
  onResize,
}: {
  card: WorkDashboardCardLayout;
  children: React.ReactNode;
  draggable: boolean;
  resizable: boolean;
  onHide: () => void;
  onResize: (span: WorkDashboardCardLayout["span"]) => void;
}) {
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({
    id: card.type,
    disabled: !draggable,
  });

  return (
    <section
      ref={setNodeRef}
      style={{
        transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined,
        transition,
      }}
      className={cn("col-span-1 sm:col-span-2 xl:col-span-1", spanClass(card.span), isDragging && "z-10 opacity-70")}
      aria-label={DASHBOARD_CARD_LABELS[card.type as DashboardCardType] ?? "工作台卡片"}
    >
      <div className="group/card relative h-full rounded-xl bg-card ring-1 ring-foreground/10">
        <div className="absolute right-2 top-2 z-10 flex gap-1 opacity-100 transition-opacity [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-focus-within/card:opacity-100 [@media(hover:hover)]:group-hover/card:opacity-100">
          {draggable && (
            <button
              ref={setActivatorNodeRef}
              type="button"
              className="inline-flex size-11 cursor-grab touch-none items-center justify-center rounded-lg border bg-background/95 text-muted-foreground shadow-sm hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label="拖拽排序"
              {...attributes}
              {...listeners}
            >
              <GripVertical className="size-4" />
            </button>
          )}
          {resizable && (
            <>
              <button
                type="button"
                className="inline-flex size-11 items-center justify-center rounded-lg border bg-background/95 text-muted-foreground shadow-sm hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                aria-label="缩小卡片"
                onClick={() => onResize(Math.max(1, card.span - 1) as WorkDashboardCardLayout["span"])}
              >
                <Minimize2 className="size-4" />
              </button>
              <button
                type="button"
                className="inline-flex size-11 items-center justify-center rounded-lg border bg-background/95 text-muted-foreground shadow-sm hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                aria-label="放大卡片"
                onClick={() => onResize(Math.min(4, card.span + 1) as WorkDashboardCardLayout["span"])}
              >
                <Maximize2 className="size-4" />
              </button>
            </>
          )}
          <button
            type="button"
            className="inline-flex size-11 items-center justify-center rounded-lg border bg-background/95 text-muted-foreground shadow-sm hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            aria-label="隐藏卡片"
            onClick={onHide}
          >
            <EyeOff className="size-4" />
          </button>
        </div>
        <div className="h-full pt-12 pb-2 [@media(hover:hover)]:pt-0">{children}</div>
      </div>
    </section>
  );
}

function StaticCard({
  card,
  children,
  onHide,
}: {
  card: WorkDashboardCardLayout;
  children: React.ReactNode;
  onHide: () => void;
}) {
  return (
    <section
      className="col-span-1"
      aria-label={DASHBOARD_CARD_LABELS[card.type as DashboardCardType] ?? "工作台卡片"}
    >
      <div className="group/card relative h-full rounded-xl bg-card ring-1 ring-foreground/10">
        <div className="absolute top-2 right-2 z-10">
          <button
            type="button"
            className="inline-flex size-11 items-center justify-center rounded-lg border bg-background/95 text-muted-foreground shadow-sm hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            aria-label="隐藏卡片"
            onClick={onHide}
          >
            <EyeOff className="size-4" />
          </button>
        </div>
        <div className="h-full pt-12 pb-2">{children}</div>
      </div>
    </section>
  );
}

export function DashboardGrid({
  cards,
  onChange,
  isTablet,
  isDesktop,
  onTaskCreated,
  refreshKey,
}: {
  cards: WorkDashboardCardLayout[];
  onChange: (cards: WorkDashboardCardLayout[]) => void;
  isTablet: boolean;
  isDesktop: boolean;
  onTaskCreated: () => void;
  refreshKey: number;
}) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor),
  );
  const visible = cards.filter((card) => card.visible);

  function handleDragEnd(event: DragEndEvent) {
    if (!event.over || event.active.id === event.over.id) return;
    const oldIndex = cards.findIndex((card) => card.type === event.active.id);
    const newIndex = cards.findIndex((card) => card.type === event.over?.id);
    if (oldIndex < 0 || newIndex < 0) return;
    onChange(arrayMove(cards, oldIndex, newIndex));
  }

  function toggle(type: string, visible: boolean) {
    onChange(cards.map((card) => (card.type === type ? { ...card, visible } : card)));
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">
          {isDesktop
            ? "桌面端：可拖拽、缩放、隐藏和恢复卡片。"
            : isTablet
              ? "平板端：支持移动和排序，不支持缩放。"
              : "手机端：单列展示，自动关闭拖拽和缩放。"}
        </p>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm" className="h-10 sm:h-7">
              <Settings2 className="size-4" />
              配置卡片
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            {cards.map((card) => (
              <DropdownMenuCheckboxItem
                key={card.type}
                checked={card.visible}
                onCheckedChange={(checked) => toggle(card.type, checked)}
              >
                {DASHBOARD_CARD_LABELS[card.type as DashboardCardType] ?? card.type}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {!isDesktop && !isTablet ? (
        <div className="grid grid-cols-1 gap-3">
          {visible.map((card) => {
            const content = cardContent(card.type as DashboardCardType, onTaskCreated, refreshKey);
            if (content === null) return null;
            return (
              <StaticCard
                key={card.type}
                card={card}
                onHide={() => toggle(card.type, false)}
              >
                {content}
              </StaticCard>
            );
          })}
        </div>
      ) : (
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={visible.map((card) => card.type)} strategy={rectSortingStrategy}>
            <div className={cn("grid gap-3", "sm:grid-cols-2 xl:grid-cols-4")}>
            {visible.map((card) => {
              const content = cardContent(card.type as DashboardCardType, onTaskCreated, refreshKey);
              if (content === null) return null;
              return (
                <SortableCard
                  key={card.type}
                  card={card}
                  draggable={isDesktop || isTablet}
                  resizable={isDesktop}
                  onHide={() => toggle(card.type, false)}
                  onResize={(span) => onChange(cards.map((item) => (item.type === card.type ? { ...item, span } : item)))}
                >
                  {content}
                </SortableCard>
              );
            })}
            </div>
          </SortableContext>
        </DndContext>
      )}
    </div>
  );
}
