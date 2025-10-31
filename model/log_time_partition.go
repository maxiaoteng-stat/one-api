package model

import (
	"time"
)

// TimeRange 表示时间范围
type TimeRange struct {
	Start int64
	End   int64
}

// DataSourcePartition 数据源分区结果
type DataSourcePartition struct {
	// MySQL查询的时间范围（可能有多个不连续的范围）
	MySQLRanges []TimeRange
	// Redis查询的日期列表（YYYY-MM-DD格式）
	RedisDates []string
}

// PartitionByDataSource 根据数据源(Redis/MySQL)划分时间范围
// 规则：
// 1. 今天的数据 -> MySQL（实时数据）
// 2. 昨天到179天前的完整日期 -> Redis（已缓存数据）
// 3. 非完整日期（有时分秒）-> MySQL
// 4. 180天前及更早 -> MySQL（历史冷数据）
func PartitionByDataSource(startTimestamp, endTimestamp int64) *DataSourcePartition {
	result := &DataSourcePartition{
		MySQLRanges: make([]TimeRange, 0),
		RedisDates:  make([]string, 0),
	}

	loc := time.Local
	now := time.Now().In(loc)

	// 今天的开始时间（00:00:00）
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	todayStartTs := todayStart.Unix()

	// Redis缓存边界：179天前的开始时间（今天是第180天）
	redisCacheBoundary := todayStart.AddDate(0, 0, -179)
	redisCacheBoundaryTs := redisCacheBoundary.Unix()

	startTime := time.Unix(startTimestamp, 0).In(loc)
	endTime := time.Unix(endTimestamp, 0).In(loc)

	// 获取开始日期的当天开始和结束时间
	startDayBegin := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, loc)
	startDayEnd := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 23, 59, 59, 0, loc)

	// 获取结束日期的当天开始时间
	endDayBegin := time.Date(endTime.Year(), endTime.Month(), endTime.Day(), 0, 0, 0, 0, loc)

	// 1. 处理180天前的历史数据（MySQL冷数据区）
	if startTimestamp < redisCacheBoundaryTs {
		historyEnd := endTimestamp
		if endTimestamp >= redisCacheBoundaryTs {
			historyEnd = redisCacheBoundary.Add(-time.Second).Unix()
		}
		result.MySQLRanges = append(result.MySQLRanges, TimeRange{
			Start: startTimestamp,
			End:   historyEnd,
		})

		// 如果查询范围完全在180天前，直接返回
		if endTimestamp < redisCacheBoundaryTs {
			return result
		}

		// 调整开始时间到Redis缓存边界
		startTimestamp = redisCacheBoundaryTs
		startTime = time.Unix(startTimestamp, 0).In(loc)
		startDayBegin = time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, loc)
		startDayEnd = time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 23, 59, 59, 0, loc)
	}

	// 2. 处理开始日期的非整日部分（如果不是从00:00:00开始，且不是今天）
	if startTime.After(startDayBegin) && startDayBegin.Before(todayStart) {
		// 从开始时间到当天结束（或查询结束时间，取较小值）
		rangeEnd := startDayEnd.Unix()
		if endTimestamp < rangeEnd {
			rangeEnd = endTimestamp
		}
		result.MySQLRanges = append(result.MySQLRanges, TimeRange{
			Start: startTimestamp,
			End:   rangeEnd,
		})

		// 调整Redis开始日期为第二天
		startDayBegin = startDayBegin.AddDate(0, 0, 1)
	}

	// 3. 处理结束日期的非整日部分（如果不是到23:59:59结束，且不是今天）
	var redisEndDate time.Time
	if endDayBegin.Before(todayStart) {
		// 结束日期在今天之前
		if endTime.After(endDayBegin) && endTime.Before(endDayBegin.Add(24*time.Hour)) {
			// 结束时间不是一天的结束（有时分秒）
			result.MySQLRanges = append(result.MySQLRanges, TimeRange{
				Start: endDayBegin.Unix(),
				End:   endTimestamp,
			})
			// Redis区间到前一天结束
			redisEndDate = endDayBegin.AddDate(0, 0, -1)
		} else {
			// 结束时间是一天的结束，Redis包含这一天
			redisEndDate = endDayBegin
		}
	} else {
		// 结束日期是今天或之后，Redis区间到昨天
		redisEndDate = todayStart.AddDate(0, 0, -1)
	}

	// 4. 生成Redis日期列表（完整的自然日）
	if startDayBegin.Before(todayStart) && !startDayBegin.After(redisEndDate) {
		current := startDayBegin
		for !current.After(redisEndDate) {
			result.RedisDates = append(result.RedisDates, current.Format("2006-01-02"))
			current = current.AddDate(0, 0, 1)
		}
	}

	// 5. 处理今天的数据（MySQL实时数据区）
	if endTimestamp >= todayStartTs {
		mysqlStart := startTimestamp
		if mysqlStart < todayStartTs {
			mysqlStart = todayStartTs
		}
		result.MySQLRanges = append(result.MySQLRanges, TimeRange{
			Start: mysqlStart,
			End:   endTimestamp,
		})
	}

	return result
}

// MergeTimeRanges 合并多个时间范围（如果需要用一条SQL查询）
func MergeTimeRanges(ranges []TimeRange) []TimeRange {
	if len(ranges) == 0 {
		return ranges
	}

	// 简单情况：如果只有一个范围或者范围不连续，返回原数组
	// 可以根据需要实现更复杂的合并逻辑
	return ranges
}
