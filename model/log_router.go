package model

import (
	"fmt"
	"time"

	"github.com/songquanpeng/one-api/common/logger"
)

// GetTableNameByDate 根据日期获取表名
func GetTableNameByDate(date time.Time) string {
	// 按半年分表: logs_2025h1, logs_2025h2
	if date.Month() <= 6 {
		return fmt.Sprintf("logs_%dh1", date.Year())
	}
	return fmt.Sprintf("logs_%dh2", date.Year())
}

// GetTableNameByTimestamp 根据时间戳获取表名
func GetTableNameByTimestamp(timestamp int64) string {
	return GetTableNameByDate(time.Unix(timestamp, 0))
}

// GetCurrentTableName 获取当前时间对应的表名
func GetCurrentTableName() string {
	return GetTableNameByDate(time.Now())
}

// GetTableNamesByDateRange 根据日期范围获取所有涉及的表名（使用内存缓存，性能优化）
func GetTableNamesByDateRange(startDate, endDate time.Time) []string {
	// 限制结束时间不能超过当前时间
	now := time.Now()
	if endDate.After(now) {
		endDate = now
	}

	var result []string

	startYear := startDate.Year()
	endYear := endDate.Year()
	startHalf := 1
	if startDate.Month() > 6 {
		startHalf = 2
	}
	endHalf := 1
	if endDate.Month() > 6 {
		endHalf = 2
	}

	// 遍历涉及的所有半年
	for year := startYear; year <= endYear; year++ {
		firstHalf := 1
		lastHalf := 2

		if year == startYear {
			firstHalf = startHalf
		}
		if year == endYear {
			lastHalf = endHalf
		}

		for half := firstHalf; half <= lastHalf; half++ {
			tableName := fmt.Sprintf("logs_%dh%d", year, half)

			// 从内存缓存检查表是否存在（不再查询数据库）
			if IsTableInCache(tableName) {
				result = append(result, tableName)
			}
		}
	}

	// 如果没有找到任何表，返回当前表
	if len(result) == 0 {
		logger.SysLog("未找到任何分表，使用当前表")
		return []string{GetCurrentTableName()}
	}

	return result
}
