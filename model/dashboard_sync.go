package model

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/logger"
)

var (
	dashboardSyncMutex sync.Mutex
	syncStopChan       chan struct{}
)

// StartDashboardCacheSync 启动统计缓存同步任务
func StartDashboardCacheSync() {
	if !common.RedisEnabled {
		logger.SysLog("Redis未启用，跳过统计缓存同步")
		return
	}

	logger.SysLog("启动统计缓存同步任务...")

	// 首次启动时，检查是否需要初始化历史数据
	go func() {
		if !IsLogStatsCacheInitialized() {
			logger.SysLog("检测到首次启动，开始加载历史数据...")
			if err := initHistoricalData(); err != nil {
				logger.SysError(fmt.Sprintf("加载历史数据失败: %s", err.Error()))
			} else {
				logger.SysLog("历史数据加载完成")
				SetLogStatsCacheInitialized()
			}
		} else {
			logger.SysLog("统计缓存已初始化，跳过历史数据加载")
		}
	}()

	// 启动定时任务，每天00:10执行
	startDailySyncTask()
}

// startDailySyncTask 启动每日同步任务
func startDailySyncTask() {
	syncStopChan = make(chan struct{})

	go func() {
		for {
			// 每次循环都重新计算下一个00:10的时间
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), 0, 10, 0, 0, now.Location())

			// 如果今天的00:10已经过了，计算明天的00:10
			if now.After(next) {
				next = next.AddDate(0, 0, 1)
			}

			delay := next.Sub(now)
			logger.SysLog(fmt.Sprintf("统计缓存同步任务将在 %s 后执行 (目标时间: %s)", delay, next.Format("2006-01-02 15:04:05")))

			timer := time.NewTimer(delay)

			select {
			case <-timer.C:
				syncPreviousDayData()
			case <-syncStopChan:
				timer.Stop()
				logger.SysLog("统计缓存同步任务已停止")
				return
			}
		}
	}()
}

// StopDashboardCacheSync 停止统计缓存同步任务
func StopDashboardCacheSync() {
	if syncStopChan != nil {
		close(syncStopChan)
	}
}

// initHistoricalData 初始化历史数据（加载最近180天的数据）
func initHistoricalData() error {
	dashboardSyncMutex.Lock()
	defer dashboardSyncMutex.Unlock()

	logger.SysLog("开始加载最近180天的历史数据到Redis...")

	now := time.Now()
	startDate := now.AddDate(0, 0, -LogStatsCacheDays)

	// 按天分批处理
	successCount := 0
	failCount := 0

	for day := 0; day < LogStatsCacheDays; day++ {
		currentDate := startDate.AddDate(0, 0, day)
		dateStr := currentDate.Format("2006-01-02")

		if err := syncOneDayData(currentDate); err != nil {
			logger.SysError(fmt.Sprintf("同步日期 %s 的数据失败: %s", dateStr, err.Error()))
			failCount++
		} else {
			successCount++
		}

		// 每10天打印一次进度
		if (day+1)%10 == 0 {
			logger.SysLog(fmt.Sprintf("历史数据加载进度: %d/%d 天", day+1, LogStatsCacheDays))
		}
	}

	logger.SysLog(fmt.Sprintf("历史数据加载完成: 成功 %d 天, 失败 %d 天", successCount, failCount))

	return nil
}

// syncPreviousDayData 同步前一天的数据到Redis
func syncPreviousDayData() {
	dashboardSyncMutex.Lock()
	defer dashboardSyncMutex.Unlock()

	yesterday := time.Now().AddDate(0, 0, -1)
	dateStr := yesterday.Format("2006-01-02")

	logger.SysLog(fmt.Sprintf("开始同步前一天(%s)的数据到Redis...", dateStr))

	if err := syncOneDayData(yesterday); err != nil {
		logger.SysError(fmt.Sprintf("同步前一天数据失败: %s", err.Error()))
		return
	}

	logger.SysLog(fmt.Sprintf("前一天(%s)数据同步完成", dateStr))

	// 清理180天前的缓存
	cleanupDate := time.Now().AddDate(0, 0, -LogStatsCacheDays-1)
	if err := CleanOldLogStatsCache(cleanupDate); err != nil {
		logger.SysError(fmt.Sprintf("清理过期统计缓存失败: %s", err.Error()))
	}
}

// syncOneDayData 同步某一天的数据到Redis（统一查询）
func syncOneDayData(date time.Time) error {
	dateStr := date.Format("2006-01-02")

	// 计算该天的时间范围
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24*time.Hour - time.Second)

	startTimestamp := int(startOfDay.Unix())
	endTimestamp := int(endOfDay.Unix())

	// 使用手动分表的查询
	tables := getTablesForRange(int64(startTimestamp), int64(endTimestamp))

	// 构建UNION ALL查询 - 一次查询获取所有维度
	var queries []string
	for _, table := range tables {
		queries = append(queries, fmt.Sprintf(`
			SELECT 
				user_id,
				token_name,
				model_name,
				SUM(prompt_tokens) as prompt_tokens,
				SUM(completion_tokens) as completion_tokens,
				COUNT(id) as request_count,
				SUM(quota) as quota,
				MAX(created_at) as last_used_time
			FROM %s
			WHERE type = 2
			AND created_at BETWEEN %d AND %d
			GROUP BY user_id, token_name, model_name
		`, table, startTimestamp, endTimestamp))
	}

	if len(queries) == 0 {
		logger.SysLog(fmt.Sprintf("日期 %s 无可查询的表", dateStr))
		return nil
	}

	finalQuery := strings.Join(queries, " UNION ALL ")

	var results []struct {
		UserId           int    `gorm:"column:user_id"`
		TokenName        string `gorm:"column:token_name"`
		ModelName        string `gorm:"column:model_name"`
		PromptTokens     int    `gorm:"column:prompt_tokens"`
		CompletionTokens int    `gorm:"column:completion_tokens"`
		RequestCount     int    `gorm:"column:request_count"`
		Quota            int    `gorm:"column:quota"`
		LastUsedTime     int64  `gorm:"column:last_used_time"`
	}

	err := LOG_DB.Raw(finalQuery).Scan(&results).Error
	if err != nil {
		return fmt.Errorf("查询统计数据失败: %w", err)
	}

	if len(results) == 0 {
		logger.SysLog(fmt.Sprintf("日期 %s 无统计数据", dateStr))
		return nil
	}

	// 按 user_id 分组
	userStatsMap := make(map[int]map[string]*DetailedStatItem)

	for _, r := range results {
		if _, exists := userStatsMap[r.UserId]; !exists {
			userStatsMap[r.UserId] = make(map[string]*DetailedStatItem)
		}

		// field格式: token_name:model_name
		field := r.TokenName + ":" + r.ModelName
		userStatsMap[r.UserId][field] = &DetailedStatItem{
			TokenName:        r.TokenName,
			ModelName:        r.ModelName,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			RequestCount:     r.RequestCount,
			Quota:            r.Quota,
			LastUsedTime:     r.LastUsedTime,
		}
	}

	// 批量写入Redis
	writeCount := 0
	for userId, statsMap := range userStatsMap {
		if err := SetDetailedStatsBatch(userId, dateStr, statsMap); err != nil {
			logger.SysError(fmt.Sprintf("写入统计数据失败 userId=%d, date=%s: %s",
				userId, dateStr, err.Error()))
			continue
		}
		writeCount += len(statsMap)
	}

	logger.SysLog(fmt.Sprintf("日期 %s: 写入了 %d 个用户的 %d 条统计数据", dateStr, len(userStatsMap), writeCount))

	return nil
}
