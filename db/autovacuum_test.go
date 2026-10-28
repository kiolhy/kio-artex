package db

import (
	"strings"
	"testing"
)

// 语句必须是幂等的 ALTER TABLE ... SET（可重复执行），且只碰名单里的表。
func TestHotTableStatements(t *testing.T) {
	stmts := hotTableStatements(true)
	if len(stmts) != len(hotTables) {
		t.Fatalf("语句数应为 %d，实际 %d", len(hotTables), len(stmts))
	}
	for i, s := range stmts {
		if !strings.HasPrefix(s, "ALTER TABLE "+hotTables[i].name+" SET (") {
			t.Errorf("语句 %q 未针对表 %s", s, hotTables[i].name)
		}
		if !strings.Contains(s, "autovacuum_vacuum_scale_factor = 0.02") {
			t.Errorf("语句 %q 缺少 vacuum 比例", s)
		}
		if !strings.Contains(s, "autovacuum_analyze_scale_factor = 0.01") {
			t.Errorf("语句 %q 缺少 analyze 比例", s)
		}
		if !strings.Contains(s, "autovacuum_vacuum_insert_threshold = 1000") {
			t.Errorf("语句 %q 缺少 insert 阈值", s)
		}
		if !strings.HasSuffix(s, ")") {
			t.Errorf("语句 %q 括号不闭合", s)
		}
	}
}

// 老版本 PG 退化成只设比例时，不能残留 insert 阈值参数。
func TestHotTableStatementsWithoutInsertThreshold(t *testing.T) {
	for _, s := range hotTableStatements(false) {
		if strings.Contains(s, "insert_threshold") {
			t.Errorf("退化语句不该含 insert 阈值：%q", s)
		}
	}
}

// 名单只应包含真实存在的高频写入表，且不得接受外部输入（这里是代码常量）。
func TestHotTablesAreExpectedSet(t *testing.T) {
	want := map[string]bool{"activity": true, "llm_usage": true, "llm_records": true, "tool_usage": true}
	if len(hotTables) != len(want) {
		t.Fatalf("名单大小变了（%d），确认新表确实高频写入后再更新测试", len(hotTables))
	}
	for _, h := range hotTables {
		if !want[h.name] {
			t.Errorf("未预期的表 %q", h.name)
		}
		if h.vacuumScale <= 0 || h.vacuumScale >= 1 {
			t.Errorf("%s 的 vacuum 比例不合理：%v", h.name, h.vacuumScale)
		}
		if h.analyzeScale <= 0 || h.analyzeScale >= 1 {
			t.Errorf("%s 的 analyze 比例不合理：%v", h.name, h.analyzeScale)
		}
	}
}
