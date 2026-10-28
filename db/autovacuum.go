package db

// autovacuum.go —— 给高频写入表单独调 autovacuum（来源：二开 kismetpro 7f5fd0f 的表级部分）。
//
// 为什么只做表级、不动 shared_buffers 之类的实例级参数：实例级参数改的是别人家的
// 数据库（很多人用托管 PG 或共用实例），改错了整个实例都受影响，而且需要重启。
// 表级 storage 参数是幂等的、只影响这几张表、随时可以改回来。
//
// 为什么这几张表需要单独调：autovacuum 默认按 20% 的死元组比例触发。对 activity /
// llm_usage / llm_records / tool_usage 这种"每轮对话都写、又按保留期批量删"的表，
// 表一大之后 20% 的阈值等于永远不触发——死元组与索引膨胀一直挂着，查询越来越慢，
// 直到某次全表 VACUUM 才回落。把比例降到 2%，并给 append-only 表显式写上 insert
// 阈值，让后台回收跟得上写入节奏。

import (
	"fmt"
	"log"
)

// hotTable 描述一张需要单独调优的表。
type hotTable struct {
	name string
	// vacuumScale / analyzeScale 是 autovacuum 的触发比例（0.02 = 2%）。
	vacuumScale  float64
	analyzeScale float64
	// insertThreshold > 0 时同时设置 autovacuum_vacuum_insert_threshold（PG 13+）。
	insertThreshold int
}

// hotTables 是调优名单。表名是代码常量（不接受外部输入），因此直接拼进 DDL 是安全的。
var hotTables = []hotTable{
	{name: "activity", vacuumScale: 0.02, analyzeScale: 0.01, insertThreshold: 1000},
	{name: "llm_usage", vacuumScale: 0.02, analyzeScale: 0.01, insertThreshold: 1000},
	{name: "llm_records", vacuumScale: 0.02, analyzeScale: 0.01, insertThreshold: 1000},
	{name: "tool_usage", vacuumScale: 0.02, analyzeScale: 0.01, insertThreshold: 1000},
}

// hotTableStatement 生成一张表的 ALTER 语句。
// withInsertThreshold=false 时退化成只设两个比例，供 PG 12 及更老的实例使用。
func hotTableStatement(t hotTable, withInsertThreshold bool) string {
	stmt := fmt.Sprintf(
		`ALTER TABLE %s SET (autovacuum_vacuum_scale_factor = %g, autovacuum_analyze_scale_factor = %g`,
		t.name, t.vacuumScale, t.analyzeScale)
	if withInsertThreshold && t.insertThreshold > 0 {
		stmt += fmt.Sprintf(`, autovacuum_vacuum_insert_threshold = %d`, t.insertThreshold)
	}
	return stmt + ")"
}

// hotTableStatements 生成全部语句（拆出来是为了能脱离数据库单测）。
func hotTableStatements(withInsertThreshold bool) []string {
	out := make([]string, 0, len(hotTables))
	for _, t := range hotTables {
		out = append(out, hotTableStatement(t, withInsertThreshold))
	}
	return out
}

// TuneHotTables 对高频写入表设置表级 autovacuum 参数。幂等：每次启动重设一遍即可。
//
// 全程 best-effort：表不存在就跳过（不同版本的库表集合不同），参数不被支持就退化成
// 只设比例，仍然失败只记日志——存储参数调优绝不该挡住启动。
func (d *DB) TuneHotTables() error {
	withInsert := true
	for _, t := range hotTables {
		exists, err := d.tableExists(t.name)
		if err != nil {
			log.Printf("[db] autovacuum: 检查表 %s 失败，跳过：%v", t.name, err)
			continue
		}
		if !exists {
			continue
		}
		if _, err := d.Exec(hotTableStatement(t, withInsert)); err != nil {
			if withInsert && t.insertThreshold > 0 {
				// 可能是 PG 12 及更老（没有 insert 阈值参数）：整体退化后重试。
				log.Printf("[db] autovacuum: %s 设置 insert 阈值失败（%v），改为只设比例", t.name, err)
				withInsert = false
				if _, err2 := d.Exec(hotTableStatement(t, false)); err2 != nil {
					log.Printf("[db] autovacuum: %s 设置失败（已忽略）：%v", t.name, err2)
				}
				continue
			}
			log.Printf("[db] autovacuum: %s 设置失败（已忽略）：%v", t.name, err)
		}
	}
	return nil
}

// tableExists 用 to_regclass 判断表是否存在（不存在的表返回 NULL 而不是报错）。
func (d *DB) tableExists(name string) (bool, error) {
	var reg *string
	if err := d.QueryRow(`SELECT to_regclass($1)::text`, name).Scan(&reg); err != nil {
		return false, err
	}
	return reg != nil && *reg != "", nil
}
