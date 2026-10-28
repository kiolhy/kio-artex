package server

import (
	"context"
	"log"
	"time"

	"github.com/Autumn-27/artex/db"
)

const workReminderTick = 30 * time.Second

type workReminderWorker struct {
	pg *db.DB
}

func (w *workReminderWorker) Run(ctx context.Context) {
	if w.pg == nil {
		return
	}
	t := time.NewTicker(workReminderTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.step(ctx)
		}
	}
}

func (w *workReminderWorker) step(ctx context.Context) {
	if _, err := w.pg.DispatchDueWorkReminders(ctx, time.Now(), 100); err != nil {
		log.Printf("[work] 处理任务提醒失败: %v", err)
	}
}
