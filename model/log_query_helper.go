package model

import (
	"strings"
	"time"

	"github.com/songquanpeng/one-api/common"
)

// getTablesForRange 获取时间范围内涉及的所有分表
func getTablesForRange(startTimestamp, endTimestamp int64) []string {
	if !common.UsingMySQL {
		return []string{"logs"}
	}

	// 如果没有指定时间范围，使用最近1年
	if startTimestamp == 0 && endTimestamp == 0 {
		return getTablesForRecentPeriod()
	}

	// 如果只指定了一个时间，设置默认值
	if startTimestamp == 0 {
		startTimestamp = time.Unix(endTimestamp, 0).AddDate(-1, 0, 0).Unix()
	}
	if endTimestamp == 0 {
		endTimestamp = time.Now().Unix()
	}

	startDate := time.Unix(startTimestamp, 0)
	endDate := time.Unix(endTimestamp, 0)
	return GetTableNamesByDateRange(startDate, endDate)
}

// getTablesForRecentPeriod 获取最近一段时间的分表（用于没有明确时间范围的查询）
// 默认查询最近1年的数据
func getTablesForRecentPeriod() []string {
	if !common.UsingMySQL {
		return []string{"logs"}
	}

	now := time.Now()
	// 从1年前到现在
	startTimestamp := now.AddDate(-1, 0, 0).Unix()
	endTimestamp := now.Unix()

	return getTablesForRange(startTimestamp, endTimestamp)
}

// getAllExistingTables 获取数据库中所有已存在的 logs 分表（用于删除等操作）
// 优化：从内存缓存获取，避免查询数据库
func getAllExistingTables() []string {
	if !common.UsingMySQL {
		return []string{"logs"}
	}

	// 从缓存中获取所有表
	var tables []string
	existingTableCache.Range(func(key, value interface{}) bool {
		if tableName, ok := key.(string); ok {
			tables = append(tables, tableName)
		}
		return true
	})

	if len(tables) == 0 {
		// 缓存为空，返回当前表
		return []string{GetCurrentTableName()}
	}

	return tables
}

// buildUnionSQL 构建跨表UNION ALL的SQL片段
func buildUnionSQL(tables []string, sqlTemplate string) string {
	if len(tables) == 0 {
		return ""
	}

	if len(tables) == 1 {
		return strings.Replace(sqlTemplate, "{TABLE}", tables[0], -1)
	}

	// 多表UNION ALL
	var unions []string
	for _, table := range tables {
		sql := strings.Replace(sqlTemplate, "{TABLE}", table, -1)
		unions = append(unions, sql)
	}

	return strings.Join(unions, " UNION ALL ")
}
