package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/logger"
)

const (
	LogStatsCachePrefix = "log_stats:" // 统一的日志统计缓存前缀
	LogStatsCacheDays   = 180          // 缓存180天（半年）
	LogStatsCacheTTL    = 180 * 24 * time.Hour
	LogStatsInitFlag    = "log_stats:initialized"
)

// DetailedStatItem 详细统计项（包含所有维度）
// Key: log_stats:{userId}:{date}, Field: {token_name}:{model_name}, Value: JSON
type DetailedStatItem struct {
	TokenName        string `json:"token_name"`
	ModelName        string `json:"model_name"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	RequestCount     int    `json:"request_count"`
	Quota            int    `json:"quota"`
	LastUsedTime     int64  `json:"last_used_time"`
}

// SetDetailedStatsBatch 批量写入某个用户某天的详细统计数据
func SetDetailedStatsBatch(userId int, date string, statsMap map[string]*DetailedStatItem) error {
	if !common.RedisEnabled {
		return fmt.Errorf("Redis未启用")
	}

	if len(statsMap) == 0 {
		return nil
	}

	ctx := context.Background()
	key := fmt.Sprintf("%s%d:%s", LogStatsCachePrefix, userId, date)

	fields := make(map[string]interface{})
	for field, stats := range statsMap {
		jsonData, err := json.Marshal(stats)
		if err != nil {
			logger.SysError(fmt.Sprintf("序列化统计数据失败: %s", err.Error()))
			continue
		}
		fields[field] = string(jsonData)
	}

	if len(fields) == 0 {
		return nil
	}

	// 批量写入
	_, err := common.RDB.HMSet(ctx, key, fields).Result()
	if err != nil {
		return fmt.Errorf("批量写入统计数据失败: %w", err)
	}

	// 设置过期时间
	common.RDB.Expire(ctx, key, LogStatsCacheTTL)

	return nil
}

// GetDetailedStatsByDate 获取某个用户某天的详细统计数据
func GetDetailedStatsByDate(userId int, date string) (map[string]*DetailedStatItem, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("Redis未启用")
	}

	ctx := context.Background()
	key := fmt.Sprintf("%s%d:%s", LogStatsCachePrefix, userId, date)

	result, err := common.RDB.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("从Redis读取统计数据失败: %w", err)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("缓存数据不存在")
	}

	statsMap := make(map[string]*DetailedStatItem)
	for field, jsonStr := range result {
		var item DetailedStatItem
		if err := json.Unmarshal([]byte(jsonStr), &item); err != nil {
			logger.SysError(fmt.Sprintf("解析统计数据失败: field=%s, error=%s", field, err.Error()))
			continue
		}
		statsMap[field] = &item
	}

	return statsMap, nil
}

// GetDetailedStatsByDateRange 获取某个用户某个日期范围内的详细统计数据
func GetDetailedStatsByDateRange(userId int, startDate, endDate time.Time) (map[string]map[string]*DetailedStatItem, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("Redis未启用")
	}

	result := make(map[string]map[string]*DetailedStatItem)
	current := startDate

	adjustedEndDate := endDate
	// if endDate.Hour() == 0 && endDate.Minute() == 0 && endDate.Second() == 0 {
	// 	adjustedEndDate = endDate.AddDate(0, 0, -1)
	// }

	for current.Before(adjustedEndDate) || current.Equal(adjustedEndDate) {
		dateStr := current.Format("2006-01-02")
		stats, err := GetDetailedStatsByDate(userId, dateStr)
		if err != nil {
			// 某天数据不存在，跳过
			current = current.AddDate(0, 0, 1)
			continue
		}
		result[dateStr] = stats
		current = current.AddDate(0, 0, 1)
	}

	return result, nil
}

// AggregateDashboardStats 从详细统计聚合为Dashboard统计（按model_name聚合）
func AggregateDashboardStats(detailedStats map[string]map[string]*DetailedStatItem) []*LogStatistic {
	// key: day:model_name
	statsMap := make(map[string]*LogStatistic)

	for date, dayStats := range detailedStats {
		for _, item := range dayStats {
			key := date + ":" + item.ModelName
			if existing, ok := statsMap[key]; ok {
				existing.RequestCount += item.RequestCount
				existing.Quota += item.Quota
				existing.PromptTokens += item.PromptTokens
				existing.CompletionTokens += item.CompletionTokens
			} else {
				statsMap[key] = &LogStatistic{
					Day:              date,
					ModelName:        item.ModelName,
					RequestCount:     item.RequestCount,
					Quota:            item.Quota,
					PromptTokens:     item.PromptTokens,
					CompletionTokens: item.CompletionTokens,
				}
			}
		}
	}

	var result []*LogStatistic
	for _, stat := range statsMap {
		result = append(result, stat)
	}

	return result
}

// AggregateTokenStats 从详细统计聚合为Token统计（按token_name聚合）
func AggregateTokenStats(detailedStats map[string]map[string]*DetailedStatItem) map[string]*TokenStatItem {
	// key: token_name
	statsMap := make(map[string]*TokenStatItem)

	for _, dayStats := range detailedStats {
		for _, item := range dayStats {
			if existing, ok := statsMap[item.TokenName]; ok {
				existing.PromptTokens += item.PromptTokens
				existing.CompletionTokens += item.CompletionTokens
				existing.RequestCount += item.RequestCount
				if item.LastUsedTime > existing.LastUsedTime {
					existing.LastUsedTime = item.LastUsedTime
				}
			} else {
				statsMap[item.TokenName] = &TokenStatItem{
					TotalTokens:      item.PromptTokens + item.CompletionTokens,
					PromptTokens:     item.PromptTokens,
					CompletionTokens: item.CompletionTokens,
					RequestCount:     item.RequestCount,
					LastUsedTime:     item.LastUsedTime,
				}
			}
		}
	}

	// 更新 TotalTokens
	for _, stat := range statsMap {
		stat.TotalTokens = stat.PromptTokens + stat.CompletionTokens
	}

	return statsMap
}

// GetDashboardStatsRange 获取Dashboard统计（保持接口兼容）
func GetDashboardStatsRange(userId int, startDate, endDate time.Time) ([]*LogStatistic, error) {
	detailedStats, err := GetDetailedStatsByDateRange(userId, startDate, endDate)
	if err != nil {
		return nil, err
	}

	return AggregateDashboardStats(detailedStats), nil
}

// GetTokenStatsByDateRange 获取Token统计（支持 excludeModels 过滤）
func GetTokenStatsByDateRange(userId int, startDate, endDate string, excludeModels string) (map[string]map[string]*TokenStatItem, error) {
	startTime, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("解析开始日期失败: %w", err)
	}

	endTime, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("解析结束日期失败: %w", err)
	}

	detailedStats, err := GetDetailedStatsByDateRange(userId, startTime, endTime)
	if err != nil {
		return nil, err
	}

	// 解析 excludeModels
	excludeModelSet := make(map[string]bool)
	if excludeModels != "" {
		for _, model := range strings.Split(excludeModels, ",") {
			excludeModelSet[strings.TrimSpace(model)] = true
		}
	}

	// 按日期分组返回Token统计
	result := make(map[string]map[string]*TokenStatItem)
	for date, dayStats := range detailedStats {
		tokenStats := make(map[string]*TokenStatItem)
		for _, item := range dayStats {
			// 过滤 excludeModels
			if excludeModelSet[item.ModelName] {
				continue
			}

			if existing, ok := tokenStats[item.TokenName]; ok {
				existing.PromptTokens += item.PromptTokens
				existing.CompletionTokens += item.CompletionTokens
				existing.RequestCount += item.RequestCount
				if item.LastUsedTime > existing.LastUsedTime {
					existing.LastUsedTime = item.LastUsedTime
				}
			} else {
				tokenStats[item.TokenName] = &TokenStatItem{
					TotalTokens:      item.PromptTokens + item.CompletionTokens,
					PromptTokens:     item.PromptTokens,
					CompletionTokens: item.CompletionTokens,
					RequestCount:     item.RequestCount,
					LastUsedTime:     item.LastUsedTime,
				}
			}
		}
		// 更新 TotalTokens
		for _, stat := range tokenStats {
			stat.TotalTokens = stat.PromptTokens + stat.CompletionTokens
		}
		result[date] = tokenStats
	}

	return result, nil
}

// GetTokenStatsByDateRangeAggregated 获取聚合的Token统计（跨所有日期）
func GetTokenStatsByDateRangeAggregated(userId int, startDate, endDate string) (map[string]*TokenStatItem, error) {
	startTime, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("解析开始日期失败: %w", err)
	}

	endTime, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("解析结束日期失败: %w", err)
	}

	detailedStats, err := GetDetailedStatsByDateRange(userId, startTime, endTime)
	if err != nil {
		return nil, err
	}

	return AggregateTokenStats(detailedStats), nil
}

// CleanOldLogStatsCache 清理指定日期之前的统计缓存
func CleanOldLogStatsCache(beforeDate time.Time) error {
	if !common.RedisEnabled {
		return fmt.Errorf("Redis未启用")
	}

	ctx := context.Background()
	pattern := LogStatsCachePrefix + "*"

	iter := common.RDB.Scan(ctx, 0, pattern, 100).Iterator()
	deletedCount := 0
	beforeDateStr := beforeDate.Format("2006-01-02")

	for iter.Next(ctx) {
		key := iter.Val()
		// 解析日期，只删除早于指定日期的缓存
		parts := strings.Split(key, ":")
		if len(parts) >= 3 {
			dateStr := parts[2]
			if dateStr < beforeDateStr {
				err := common.RDB.Del(ctx, key).Err()
				if err != nil {
					logger.SysError(fmt.Sprintf("删除缓存key %s 失败: %s", key, err.Error()))
				} else {
					deletedCount++
				}
			}
		}
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("扫描keys失败: %w", err)
	}

	logger.SysLog(fmt.Sprintf("清理了 %d 个早于 %s 的统计缓存", deletedCount, beforeDateStr))
	return nil
}

// IsLogStatsCacheInitialized 检查统计缓存是否已初始化
func IsLogStatsCacheInitialized() bool {
	if !common.RedisEnabled {
		return false
	}

	ctx := context.Background()
	result, err := common.RDB.Get(ctx, LogStatsInitFlag).Result()
	if err != nil {
		return false
	}

	return result == "1"
}

// SetLogStatsCacheInitialized 标记统计缓存已初始化
func SetLogStatsCacheInitialized() error {
	if !common.RedisEnabled {
		return fmt.Errorf("Redis未启用")
	}

	ctx := context.Background()
	return common.RDB.Set(ctx, LogStatsInitFlag, "1", 0).Err()
}

// TokenStatItem Token统计项（保持兼容）
type TokenStatItem struct {
	TotalTokens      int   `json:"total_tokens"`
	PromptTokens     int   `json:"prompt_tokens"`
	CompletionTokens int   `json:"completion_tokens"`
	RequestCount     int   `json:"request_count"`
	LastUsedTime     int64 `json:"last_used_time,omitempty"`
}
