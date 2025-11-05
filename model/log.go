package model

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
)

type Log struct {
	Id                int    `json:"id"`
	UserId            int    `json:"user_id" gorm:"index"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index:idx_created_at_type"`
	Type              int    `json:"type" gorm:"index:idx_created_at_type"`
	Content           string `json:"content"`
	Username          string `json:"username" gorm:"index:index_username_model_name,priority:2;default:''"`
	TokenName         string `json:"token_name" gorm:"index;default:''"`
	ModelName         string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota             int    `json:"quota" gorm:"default:0"`
	PromptTokens      int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens  int    `json:"completion_tokens" gorm:"default:0"`
	ChannelId         int    `json:"channel" gorm:"index"`
	RequestId         string `json:"request_id" gorm:"default:''"`
	ElapsedTime       int64  `json:"elapsed_time" gorm:"default:0"` // unit is ms
	IsStream          bool   `json:"is_stream" gorm:"default:false"`
	SystemPromptReset bool   `json:"system_prompt_reset" gorm:"default:false"`
}

const (
	LogTypeUnknown = iota
	LogTypeTopup
	LogTypeConsume
	LogTypeManage
	LogTypeSystem
	LogTypeTest
)

func recordLogHelper(ctx context.Context, log *Log) {
	requestId := helper.GetRequestID(ctx)
	log.RequestId = requestId

	var err error
	if common.UsingMySQL {
		// MySQL 使用分表
		tableName := GetCurrentTableName()
		err = LOG_DB.Table(tableName).Create(log).Error
	} else {
		err = LOG_DB.Create(log).Error
	}

	if err != nil {
		logger.Error(ctx, "failed to record log: "+err.Error())
		return
	}
	logger.Infof(ctx, "record log: %+v", log)

	// 更新Redis中的日配额使用量
	if common.RedisEnabled && log.Type == LogTypeConsume && log.TokenName != "" {
		err := IncrTokenDailyUsage(log.TokenName, int64(log.Quota))
		if err != nil {
			logger.Error(ctx, "更新Redis中token日使用量失败: "+err.Error())
		}
	}
}

func RecordLog(ctx context.Context, userId int, logType int, content string) {
	if logType == LogTypeConsume && !config.LogConsumeEnabled {
		return
	}
	log := &Log{
		UserId:    userId,
		Username:  GetUsernameById(userId),
		CreatedAt: helper.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	recordLogHelper(ctx, log)
}

func RecordTopupLog(ctx context.Context, userId int, content string, quota int) {
	log := &Log{
		UserId:    userId,
		Username:  GetUsernameById(userId),
		CreatedAt: helper.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   content,
		Quota:     quota,
	}
	recordLogHelper(ctx, log)
}

func RecordConsumeLog(ctx context.Context, log *Log) {
	if !config.LogConsumeEnabled {
		return
	}
	log.Username = GetUsernameById(log.UserId)
	log.CreatedAt = helper.GetTimestamp()
	log.Type = LogTypeConsume
	recordLogHelper(ctx, log)
}

func RecordTestLog(ctx context.Context, log *Log) {
	log.CreatedAt = helper.GetTimestamp()
	log.Type = LogTypeTest
	recordLogHelper(ctx, log)
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int) (logs []*Log, err error) {
	tables := getTablesForRange(startTimestamp, endTimestamp)

	// 构建WHERE条件
	var conditions []string
	var baseArgs []interface{}

	if logType != LogTypeUnknown {
		conditions = append(conditions, "type = ?")
		baseArgs = append(baseArgs, logType)
	}
	if modelName != "" {
		conditions = append(conditions, "model_name = ?")
		baseArgs = append(baseArgs, modelName)
	}
	if username != "" {
		conditions = append(conditions, "username = ?")
		baseArgs = append(baseArgs, username)
	}
	if tokenName != "" {
		conditions = append(conditions, "token_name = ?")
		baseArgs = append(baseArgs, tokenName)
	}
	if startTimestamp != 0 {
		conditions = append(conditions, "created_at >= ?")
		baseArgs = append(baseArgs, startTimestamp)
	}
	if endTimestamp != 0 {
		conditions = append(conditions, "created_at <= ?")
		baseArgs = append(baseArgs, endTimestamp)
	}
	if channel != 0 {
		conditions = append(conditions, "channel_id = ?")
		baseArgs = append(baseArgs, channel)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// 单表优化：直接查询，避免 UNION ALL
	if len(tables) == 1 {
		sql := fmt.Sprintf("SELECT * FROM %s%s ORDER BY id DESC LIMIT ? OFFSET ?", tables[0], whereClause)
		args := append(baseArgs, num, startIdx)
		err = LOG_DB.Raw(sql, args...).Scan(&logs).Error
		return logs, err
	}

	// 多表优化：在子查询中先排序和限制，减少需要合并的数据量
	// 每个子查询取 startIdx + num 条，确保合并后能得到足够的数据
	subQueryLimit := startIdx + num
	sqlTemplate := fmt.Sprintf("(SELECT * FROM {TABLE}%s ORDER BY id DESC LIMIT %d)", whereClause, subQueryLimit)
	unionSQL := buildUnionSQL(tables, sqlTemplate)

	// 为每个表重复参数
	var args []interface{}
	for range tables {
		args = append(args, baseArgs...)
	}
	args = append(args, num, startIdx)

	finalSQL := fmt.Sprintf("SELECT * FROM (%s) AS logs ORDER BY id DESC LIMIT ? OFFSET ?", unionSQL)
	err = LOG_DB.Raw(finalSQL, args...).Scan(&logs).Error
	return logs, err
}

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int) (logs []*Log, err error) {
	tables := getTablesForRange(startTimestamp, endTimestamp)

	// 构建WHERE条件
	var conditions []string
	var baseArgs []interface{}

	conditions = append(conditions, "user_id = ?")
	baseArgs = append(baseArgs, userId)

	if logType != LogTypeUnknown {
		conditions = append(conditions, "type = ?")
		baseArgs = append(baseArgs, logType)
	}
	if modelName != "" {
		conditions = append(conditions, "model_name = ?")
		baseArgs = append(baseArgs, modelName)
	}
	if tokenName != "" {
		conditions = append(conditions, "token_name = ?")
		baseArgs = append(baseArgs, tokenName)
	}
	if startTimestamp != 0 {
		conditions = append(conditions, "created_at >= ?")
		baseArgs = append(baseArgs, startTimestamp)
	}
	if endTimestamp != 0 {
		conditions = append(conditions, "created_at <= ?")
		baseArgs = append(baseArgs, endTimestamp)
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")

	// 单表优化：直接查询
	if len(tables) == 1 {
		sql := fmt.Sprintf("SELECT * FROM %s%s ORDER BY id DESC LIMIT ? OFFSET ?", tables[0], whereClause)
		args := append(baseArgs, num, startIdx)
		err = LOG_DB.Raw(sql, args...).Scan(&logs).Error
		return logs, err
	}

	// 多表优化：在子查询中先排序和限制
	subQueryLimit := startIdx + num
	sqlTemplate := fmt.Sprintf("(SELECT * FROM {TABLE}%s ORDER BY id DESC LIMIT %d)", whereClause, subQueryLimit)
	unionSQL := buildUnionSQL(tables, sqlTemplate)

	// 为每个表重复参数
	var args []interface{}
	for range tables {
		args = append(args, baseArgs...)
	}
	args = append(args, num, startIdx)

	finalSQL := fmt.Sprintf("SELECT * FROM (%s) AS logs ORDER BY id DESC LIMIT ? OFFSET ?", unionSQL)
	err = LOG_DB.Raw(finalSQL, args...).Scan(&logs).Error
	return logs, err
}

func SearchAllLogs(keyword string) (logs []*Log, err error) {
	// 搜索最近1年的数据
	tables := getTablesForRecentPeriod()

	// 单表优化
	if len(tables) == 1 {
		sql := fmt.Sprintf("SELECT * FROM %s WHERE type = ? OR content LIKE ? ORDER BY id DESC LIMIT ?", tables[0])
		err = LOG_DB.Raw(sql, keyword, keyword+"%", config.MaxRecentItems).Scan(&logs).Error
		return logs, err
	}

	// 多表优化：每个子查询先取 MaxRecentItems 条
	sqlTemplate := fmt.Sprintf("(SELECT * FROM {TABLE} WHERE type = ? OR content LIKE ? ORDER BY id DESC LIMIT %d)", config.MaxRecentItems)
	unionSQL := buildUnionSQL(tables, sqlTemplate)

	// 为每个表重复参数
	var args []interface{}
	for range tables {
		args = append(args, keyword, keyword+"%")
	}
	args = append(args, config.MaxRecentItems)

	finalSQL := fmt.Sprintf("SELECT * FROM (%s) AS logs ORDER BY id DESC LIMIT ?", unionSQL)
	err = LOG_DB.Raw(finalSQL, args...).Scan(&logs).Error
	return logs, err
}

func SearchUserLogs(userId int, keyword string) (logs []*Log, err error) {
	// 搜索最近1年的数据
	tables := getTablesForRecentPeriod()

	// 单表优化
	if len(tables) == 1 {
		sql := fmt.Sprintf("SELECT * FROM %s WHERE user_id = ? AND type = ? ORDER BY id DESC LIMIT ?", tables[0])
		err = LOG_DB.Raw(sql, userId, keyword, config.MaxRecentItems).Scan(&logs).Error
		return logs, err
	}

	// 多表优化：每个子查询先取 MaxRecentItems 条
	sqlTemplate := fmt.Sprintf("(SELECT * FROM {TABLE} WHERE user_id = ? AND type = ? ORDER BY id DESC LIMIT %d)", config.MaxRecentItems)
	unionSQL := buildUnionSQL(tables, sqlTemplate)

	// 为每个表重复参数
	var args []interface{}
	for range tables {
		args = append(args, userId, keyword)
	}
	args = append(args, config.MaxRecentItems)

	finalSQL := fmt.Sprintf("SELECT * FROM (%s) AS logs ORDER BY id DESC LIMIT ?", unionSQL)
	err = LOG_DB.Raw(finalSQL, args...).Scan(&logs).Error
	return logs, err
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, excludeModels string) (quota int64) {
	tables := getTablesForRange(startTimestamp, endTimestamp)

	ifnull := "ifnull"
	if common.UsingPostgreSQL {
		ifnull = "COALESCE"
	}

	// 构建WHERE条件
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "type = ?")
	args = append(args, LogTypeConsume)

	if username != "" {
		conditions = append(conditions, "username = ?")
		args = append(args, username)
	}
	if tokenName != "" {
		conditions = append(conditions, "token_name = ?")
		args = append(args, tokenName)
	}
	if startTimestamp != 0 {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, startTimestamp)
	}
	if endTimestamp != 0 {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, endTimestamp)
	}
	if modelName != "" {
		conditions = append(conditions, "model_name = ?")
		args = append(args, modelName)
	}
	if channel != 0 {
		conditions = append(conditions, "channel_id = ?")
		args = append(args, channel)
	}
	if excludeModels != "" {
		conditions = append(conditions, "model_name NOT IN (?)")
		args = append(args, strings.Split(excludeModels, ","))
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")

	// 构建UNION ALL查询
	sqlTemplate := fmt.Sprintf("SELECT %s(sum(quota),0) as quota FROM {TABLE}%s", ifnull, whereClause)
	unionSQL := buildUnionSQL(tables, sqlTemplate)

	// 为每个表重复参数
	var finalArgs []interface{}
	for range tables {
		finalArgs = append(finalArgs, args...)
	}

	finalSQL := fmt.Sprintf("SELECT %s(sum(quota),0) FROM (%s) AS combined", ifnull, unionSQL)
	LOG_DB.Raw(finalSQL, finalArgs...).Scan(&quota)
	return quota
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tables := getTablesForRange(startTimestamp, endTimestamp)

	ifnull := "ifnull"
	if common.UsingPostgreSQL {
		ifnull = "COALESCE"
	}

	// 构建WHERE条件
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "type = ?")
	args = append(args, LogTypeConsume)

	if username != "" {
		conditions = append(conditions, "username = ?")
		args = append(args, username)
	}
	if tokenName != "" {
		conditions = append(conditions, "token_name = ?")
		args = append(args, tokenName)
	}
	if startTimestamp != 0 {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, startTimestamp)
	}
	if endTimestamp != 0 {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, endTimestamp)
	}
	if modelName != "" {
		conditions = append(conditions, "model_name = ?")
		args = append(args, modelName)
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")

	// 构建UNION ALL查询
	sqlTemplate := fmt.Sprintf("SELECT %s(sum(prompt_tokens),0) + %s(sum(completion_tokens),0) as token FROM {TABLE}%s", ifnull, ifnull, whereClause)
	unionSQL := buildUnionSQL(tables, sqlTemplate)

	// 为每个表重复参数
	var finalArgs []interface{}
	for range tables {
		finalArgs = append(finalArgs, args...)
	}

	finalSQL := fmt.Sprintf("SELECT %s(sum(token),0) FROM (%s) AS combined", ifnull, unionSQL)
	LOG_DB.Raw(finalSQL, finalArgs...).Scan(&token)
	return token
}

func DeleteOldLog(targetTimestamp int64) (int64, error) {
	// 删除操作需要查询所有已存在的表
	tables := getAllExistingTables()

	var totalAffected int64
	for _, table := range tables {
		result := LOG_DB.Table(table).Where("created_at < ?", targetTimestamp).Delete(&Log{})
		if result.Error != nil {
			return totalAffected, result.Error
		}
		totalAffected += result.RowsAffected
	}

	return totalAffected, nil
}

type LogStatistic struct {
	Day              string `gorm:"column:day"`
	ModelName        string `gorm:"column:model_name"`
	RequestCount     int    `gorm:"column:request_count"`
	Quota            int    `gorm:"column:quota"`
	PromptTokens     int    `gorm:"column:prompt_tokens"`
	CompletionTokens int    `gorm:"column:completion_tokens"`
}

func SearchLogsByDayAndModel(userId, start, end int, excludeModels string) (LogStatistics []*LogStatistic, err error) {
	tables := getTablesForRange(int64(start), int64(end))

	groupSelect := "DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d') as day"

	if common.UsingPostgreSQL {
		groupSelect = "TO_CHAR(date_trunc('day', to_timestamp(created_at)), 'YYYY-MM-DD') as day"
	}

	if common.UsingSQLite {
		groupSelect = "strftime('%Y-%m-%d', datetime(created_at, 'unixepoch')) as day"
	}

	// 构建UNION ALL查询
	var sqlTemplate string
	if excludeModels == "" {
		sqlTemplate = fmt.Sprintf(`
			SELECT %s,
			model_name, count(1) as request_count,
			sum(quota) as quota,
			sum(prompt_tokens) as prompt_tokens,
			sum(completion_tokens) as completion_tokens
			FROM {TABLE}
			WHERE type=2
			AND user_id= ?
			AND created_at BETWEEN ? AND ?
			GROUP BY day, model_name`, groupSelect)
	} else {
		sqlTemplate = fmt.Sprintf(`
			SELECT %s,
			model_name, count(1) as request_count,
			sum(quota) as quota,
			sum(prompt_tokens) as prompt_tokens,
			sum(completion_tokens) as completion_tokens
			FROM {TABLE}
			WHERE type=2
			AND user_id= ?
			AND created_at BETWEEN ? AND ?
			AND model_name NOT IN (?)
			GROUP BY day, model_name`, groupSelect)
	}

	unionSQL := buildUnionSQL(tables, sqlTemplate)
	finalSQL := fmt.Sprintf("SELECT * FROM (%s) AS logs ORDER BY day, model_name", unionSQL)

	// 为每个表重复参数
	var args []interface{}
	if excludeModels == "" {
		for range tables {
			args = append(args, userId, start, end)
		}
	} else {
		for range tables {
			args = append(args, userId, start, end, strings.Split(excludeModels, ","))
		}
	}

	err = LOG_DB.Raw(finalSQL, args...).Scan(&LogStatistics).Error

	return LogStatistics, err
}

func GetUserTokenModelUsage(userId int, tokenName string, startTimestamp int64, endTimestamp int64, excludeModels string, timeInterval string) ([]struct {
	ModelName    string `json:"model_name"`
	CreatedAt    string `json:"created_at"`
	Usage        int    `json:"usage"`
	RequestCount int    `json:"request_count"`
}, error) {
	tables := getTablesForRange(startTimestamp, endTimestamp)

	ifnull := "ifnull"
	if common.UsingPostgreSQL {
		ifnull = "COALESCE"
	}

	var results []struct {
		ModelName    string `json:"model_name"`
		CreatedAt    string `json:"created_at"`
		Usage        int    `json:"usage"`
		RequestCount int    `json:"request_count"`
	}

	// 构建WHERE条件
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "user_id = ? AND token_name = ?")
	args = append(args, userId, tokenName)

	if startTimestamp != 0 {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, startTimestamp)
	}
	if endTimestamp != 0 {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, endTimestamp)
	}
	if excludeModels != "" {
		conditions = append(conditions, "model_name NOT IN (?)")
		args = append(args, strings.Split(excludeModels, ","))
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")

	// 根据 timeInterval 选择不同的分组方式
	var dateFormatExpr string
	if timeInterval == "hour" {
		// 按小时分组
		dateFormatExpr = "DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d %H:00:00')"
	} else {
		// 按天分组（默认）
		dateFormatExpr = "DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d 00:00:00')"
	}

	// 构建UNION ALL查询
	sqlTemplate := fmt.Sprintf(
		"SELECT model_name, %s as created_at, "+
			"%s(sum(prompt_tokens + completion_tokens),0) as `usage`, COUNT(1) as request_count "+
			"FROM {TABLE}%s "+
			"GROUP BY model_name, %s",
		dateFormatExpr, ifnull, whereClause, dateFormatExpr)

	unionSQL := buildUnionSQL(tables, sqlTemplate)
	finalSQL := fmt.Sprintf("SELECT * FROM (%s) AS logs", unionSQL)

	// 为每个表重复参数
	var finalArgs []interface{}
	for range tables {
		finalArgs = append(finalArgs, args...)
	}

	err := LOG_DB.Raw(finalSQL, finalArgs...).Scan(&results).Error
	return results, err
}

// TokenUsageStat 表示Token使用统计的结构
type TokenUsageStat struct {
	Username         string `json:"username"`
	TokenName        string `json:"token_name"`
	TotalTokens      int    `json:"total_tokens"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	RequestCount     int    `json:"request_count"`
	LastUsedTime     int64  `json:"last_used_time"`
}

// splitTimeToHybridWindows 将时间范围拆分为：首日、中间完整自然日、末日
// 返回：首日范围(startRange)、中间完整自然日列表(middleDays)、末日范围(endRange)
// 若起止同一天，仅返回首日范围，middleDays 和 endRange 为空
func splitTimeToHybridWindows(startTs, endTs int64, loc *time.Location) (startRange [2]int64, middleDays []string, endRange [2]int64) {
	if loc == nil {
		loc = time.Local
	}
	if endTs < startTs {
		return
	}

	startT := time.Unix(startTs, 0).In(loc)
	endT := time.Unix(endTs, 0).In(loc)

	// 同一天
	if startT.Year() == endT.Year() && startT.YearDay() == endT.YearDay() {
		startRange = [2]int64{startTs, endTs}
		return
	}

	// 首日结束 23:59:59
	startDayEnd := time.Date(startT.Year(), startT.Month(), startT.Day(), 23, 59, 59, 0, loc)
	startRange = [2]int64{startTs, startDayEnd.Unix()}

	// 末日开始 00:00:00
	endDayStart := time.Date(endT.Year(), endT.Month(), endT.Day(), 0, 0, 0, 0, loc)
	endRange = [2]int64{endDayStart.Unix(), endTs}

	// 中间完整自然日：从首日的下一天00:00开始，到末日（包含）
	// 首日的下一天
	nextDayAfterStart := time.Date(startT.Year(), startT.Month(), startT.Day()+1, 0, 0, 0, 0, loc)

	// 遍历中间的完整自然日（包含末日）
	cur := nextDayAfterStart
	for !cur.After(endDayStart) {
		middleDays = append(middleDays, cur.Format("2006-01-02"))
		cur = cur.AddDate(0, 0, 1)
	}

	return
}

// getTokenStatsFromRedisDays 从 Redis 获取指定自然日的 Token 聚合统计
// userIdOpt 为空表示所有用户；否则仅指定用户
func getTokenStatsFromRedisDays(userIdOpt *int, dates []string, excludeModels string) (map[string]*TokenUsageStat, error) {
	statsMap := make(map[string]*TokenUsageStat)

	if userIdOpt != nil {
		// 单用户
		if len(dates) == 0 {
			return statsMap, nil
		}

		// 查询用户信息（只查一次）
		var user User
		if err := DB.Where("id = ?", *userIdOpt).First(&user).Error; err != nil {
			return statsMap, fmt.Errorf("获取用户信息失败: %w", err)
		}

		startDate := dates[0]
		endDate := dates[len(dates)-1]
		cachedData, err := GetTokenStatsByDateRange(*userIdOpt, startDate, endDate, excludeModels)
		if err != nil {
			return statsMap, err
		}

		// 遍历每个日期的每个token，聚合到最终结果
		for _, d := range dates {
			if dayStats, ok := cachedData[d]; ok {
				for tokenName, item := range dayStats {
					key := user.Username + ":" + tokenName
					if existing, ok := statsMap[key]; ok {
						existing.TotalTokens += item.TotalTokens
						existing.PromptTokens += item.PromptTokens
						existing.CompletionTokens += item.CompletionTokens
						existing.RequestCount += item.RequestCount
						if item.LastUsedTime > existing.LastUsedTime {
							existing.LastUsedTime = item.LastUsedTime
						}
					} else {
						statsMap[key] = &TokenUsageStat{
							Username:         user.Username,
							TokenName:        tokenName,
							TotalTokens:      item.TotalTokens,
							PromptTokens:     item.PromptTokens,
							CompletionTokens: item.CompletionTokens,
							RequestCount:     item.RequestCount,
							LastUsedTime:     item.LastUsedTime,
						}
					}
				}
			}
		}
		return statsMap, nil
	}

	// 全部用户
	var users []User
	if err := DB.Find(&users).Error; err != nil {
		return nil, fmt.Errorf("获取用户列表失败: %w", err)
	}
	if len(dates) == 0 {
		return statsMap, nil
	}

	startDate := dates[0]
	endDate := dates[len(dates)-1]

	for _, user := range users {
		cachedData, err := GetTokenStatsByDateRange(user.Id, startDate, endDate, excludeModels)
		if err != nil {
			logger.SysLog(fmt.Sprintf("获取用户 %d 的Redis数据失败: %s", user.Id, err.Error()))
			continue
		}

		missingDates := make([]string, 0)
		for _, date := range dates {
			if _, ok := cachedData[date]; !ok {
				missingDates = append(missingDates, date)
				continue
			}
			for tokenName, item := range cachedData[date] {
				key := user.Username + ":" + tokenName
				if existing, ok := statsMap[key]; ok {
					existing.TotalTokens += item.TotalTokens
					existing.PromptTokens += item.PromptTokens
					existing.CompletionTokens += item.CompletionTokens
					existing.RequestCount += item.RequestCount
					if item.LastUsedTime > existing.LastUsedTime {
						existing.LastUsedTime = item.LastUsedTime
					}
				} else {
					statsMap[key] = &TokenUsageStat{
						Username:         user.Username,
						TokenName:        tokenName,
						TotalTokens:      item.TotalTokens,
						PromptTokens:     item.PromptTokens,
						CompletionTokens: item.CompletionTokens,
						RequestCount:     item.RequestCount,
						LastUsedTime:     item.LastUsedTime,
					}
				}
			}
		}
		if len(missingDates) > 0 {
			logger.SysLog(fmt.Sprintf("用户%d的Redis缓存缺失日期: %v", user.Id, missingDates))
		}
	}
	return statsMap, nil
}

// GetAllUserTokenStatsWithCache 使用Redis缓存+MySQL混合查询所有用户的Token统计
func GetAllUserTokenStatsWithCache(startTimestamp, endTimestamp int64, excludeModels string) ([]TokenUsageStat, error) {
	// 如果Redis未启用，直接使用MySQL
	if !common.RedisEnabled {
		return GetAllUserTokenStats(startTimestamp, endTimestamp, excludeModels)
	}

	statsMap := make(map[string]*TokenUsageStat)

	// 按数据源划分时间范围
	partition := PartitionByDataSource(startTimestamp, endTimestamp)

	// 1. 查询MySQL区间（历史冷数据 + 今天实时数据）
	for _, mysqlRange := range partition.MySQLRanges {
		mysqlStats, err := GetAllUserTokenStats(mysqlRange.Start, mysqlRange.End, excludeModels)
		if err != nil {
			return nil, fmt.Errorf("MySQL查询失败[%d-%d]: %w", mysqlRange.Start, mysqlRange.End, err)
		}
		for i := range mysqlStats {
			stat := &mysqlStats[i] // 使用索引访问，避免循环变量重用问题
			key := stat.Username + ":" + stat.TokenName
			if existing, ok := statsMap[key]; ok {
				existing.TotalTokens += stat.TotalTokens
				existing.PromptTokens += stat.PromptTokens
				existing.CompletionTokens += stat.CompletionTokens
				existing.RequestCount += stat.RequestCount
				if stat.LastUsedTime > existing.LastUsedTime {
					existing.LastUsedTime = stat.LastUsedTime
				}
			} else {
				statsMap[key] = stat
			}
		}
	}

	// 2. 查询Redis区间（昨天到179天前的缓存数据）
	if len(partition.RedisDates) > 0 {
		redisStats, err := getTokenStatsFromRedisDays(nil, partition.RedisDates, excludeModels)
		if err != nil {
			logger.SysError(fmt.Sprintf("Redis查询失败，跳过缓存数据: %s", err.Error()))
		} else {
			for _, stat := range redisStats {
				key := stat.Username + ":" + stat.TokenName
				if existing, ok := statsMap[key]; ok {
					existing.TotalTokens += stat.TotalTokens
					existing.PromptTokens += stat.PromptTokens
					existing.CompletionTokens += stat.CompletionTokens
					existing.RequestCount += stat.RequestCount
					if stat.LastUsedTime > existing.LastUsedTime {
						existing.LastUsedTime = stat.LastUsedTime
					}
				} else {
					statsMap[key] = stat
				}
			}
		}
	}

	// 3. 转换为数组并排序
	result := make([]TokenUsageStat, 0, len(statsMap))
	for _, stat := range statsMap {
		result = append(result, *stat)
	}

	// 按 total_tokens 降序排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalTokens > result[j].TotalTokens
	})

	return result, nil
}

// GetAllUserTokenStats 获取所有用户的Token使用统计
func GetAllUserTokenStats(startTimestamp, endTimestamp int64, excludeModels string) ([]TokenUsageStat, error) {
	tables := getTablesForRange(startTimestamp, endTimestamp)

	ifnull := "ifnull"
	if common.UsingPostgreSQL {
		ifnull = "COALESCE"
	}

	// 构建WHERE条件
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "type = ? AND created_at BETWEEN ? AND ?")
	args = append(args, LogTypeConsume, startTimestamp, endTimestamp)

	if excludeModels != "" {
		conditions = append(conditions, "model_name NOT IN (?)")
		args = append(args, strings.Split(excludeModels, ","))
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")

	// 构建UNION ALL查询
	sqlTemplate := fmt.Sprintf(
		"SELECT username, token_name, "+
			"%s(sum(prompt_tokens + completion_tokens),0) as total_tokens, "+
			"%s(sum(prompt_tokens),0) as prompt_tokens, "+
			"%s(sum(completion_tokens),0) as completion_tokens, "+
			"COUNT(id) as request_count, "+
			"MAX(created_at) as last_used_time "+
			"FROM {TABLE}%s "+
			"GROUP BY username, token_name",
		ifnull, ifnull, ifnull, whereClause)

	unionSQL := buildUnionSQL(tables, sqlTemplate)
	finalSQL := fmt.Sprintf("SELECT * FROM (%s) AS logs ORDER BY total_tokens DESC", unionSQL)

	// 为每个表重复参数
	var finalArgs []interface{}
	for range tables {
		finalArgs = append(finalArgs, args...)
	}

	// 执行查询
	var stats []TokenUsageStat
	err := LOG_DB.Raw(finalSQL, finalArgs...).Scan(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// GetUserTokenStatsWithCache 使用Redis缓存+MySQL混合查询特定用户的Token统计
func GetUserTokenStatsWithCache(userId int, tokenName string, startTimestamp, endTimestamp int64, excludeModels string) ([]TokenUsageStat, error) {
	// 如果Redis未启用，直接使用MySQL
	if !common.RedisEnabled {
		return GetUserTokenStats(userId, tokenName, startTimestamp, endTimestamp, excludeModels)
	}

	// 获取用户信息
	var user User
	err := DB.Where("id = ?", userId).First(&user).Error
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	statsMap := make(map[string]*TokenUsageStat)

	// 按数据源划分时间范围
	partition := PartitionByDataSource(startTimestamp, endTimestamp)

	// 1. 查询MySQL区间（历史冷数据 + 今天实时数据）
	for _, mysqlRange := range partition.MySQLRanges {
		mysqlStats, err := GetUserTokenStats(userId, tokenName, mysqlRange.Start, mysqlRange.End, excludeModels)
		if err != nil {
			return nil, fmt.Errorf("MySQL查询失败[%d-%d]: %w", mysqlRange.Start, mysqlRange.End, err)
		}
		for i := range mysqlStats {
			stat := &mysqlStats[i] // 使用索引访问，避免循环变量重用问题
			key := stat.Username + ":" + stat.TokenName
			if existing, ok := statsMap[key]; ok {
				existing.TotalTokens += stat.TotalTokens
				existing.PromptTokens += stat.PromptTokens
				existing.CompletionTokens += stat.CompletionTokens
				existing.RequestCount += stat.RequestCount
				if stat.LastUsedTime > existing.LastUsedTime {
					existing.LastUsedTime = stat.LastUsedTime
				}
			} else {
				statsMap[key] = stat
			}
		}
	}

	// 2. 查询Redis区间（昨天到179天前的缓存数据）
	if len(partition.RedisDates) > 0 {
		redisStats, err := getTokenStatsFromRedisDays(&userId, partition.RedisDates, excludeModels)
		if err != nil {
			logger.SysError(fmt.Sprintf("Redis查询失败，跳过缓存数据: %s", err.Error()))
		} else {
			for _, stat := range redisStats {
				// 如果指定了tokenName，只获取该token的数据
				if tokenName != "" && stat.TokenName != tokenName {
					continue
				}
				key := stat.Username + ":" + stat.TokenName
				if existing, ok := statsMap[key]; ok {
					existing.TotalTokens += stat.TotalTokens
					existing.PromptTokens += stat.PromptTokens
					existing.CompletionTokens += stat.CompletionTokens
					existing.RequestCount += stat.RequestCount
					if stat.LastUsedTime > existing.LastUsedTime {
						existing.LastUsedTime = stat.LastUsedTime
					}
				} else {
					statsMap[key] = stat
				}
			}
		}
	}

	// 4. 转换为数组并排序
	result := make([]TokenUsageStat, 0, len(statsMap))
	for _, stat := range statsMap {
		result = append(result, *stat)
	}

	// 按 total_tokens 降序排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalTokens > result[j].TotalTokens
	})

	return result, nil
}

// GetUserTokenStats 获取特定用户的Token使用统计
func GetUserTokenStats(userId int, tokenName string, startTimestamp, endTimestamp int64, excludeModels string) ([]TokenUsageStat, error) {
	tables := getTablesForRange(startTimestamp, endTimestamp)

	ifnull := "ifnull"
	if common.UsingPostgreSQL {
		ifnull = "COALESCE"
	}

	// 构建WHERE条件
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "user_id = ? AND type = ? AND token_name = ? AND created_at BETWEEN ? AND ?")
	args = append(args, userId, LogTypeConsume, tokenName, startTimestamp, endTimestamp)

	if excludeModels != "" {
		conditions = append(conditions, "model_name NOT IN (?)")
		args = append(args, strings.Split(excludeModels, ","))
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")

	// 构建UNION ALL查询
	sqlTemplate := fmt.Sprintf(
		"SELECT username, token_name, "+
			"%s(sum(prompt_tokens + completion_tokens),0) as total_tokens, "+
			"%s(sum(prompt_tokens),0) as prompt_tokens, "+
			"%s(sum(completion_tokens),0) as completion_tokens, "+
			"COUNT(id) as request_count, "+
			"MAX(created_at) as last_used_time "+
			"FROM {TABLE}%s "+
			"GROUP BY username, token_name",
		ifnull, ifnull, ifnull, whereClause)

	unionSQL := buildUnionSQL(tables, sqlTemplate)
	finalSQL := fmt.Sprintf("SELECT * FROM (%s) AS logs ORDER BY total_tokens DESC", unionSQL)

	// 为每个表重复参数
	var finalArgs []interface{}
	for range tables {
		finalArgs = append(finalArgs, args...)
	}

	// 执行查询
	var stats []TokenUsageStat
	err := LOG_DB.Raw(finalSQL, finalArgs...).Scan(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// GetTokenUsageByNameWithCache 使用Redis缓存+MySQL混合查询指定Token的使用统计
func GetTokenUsageByNameWithCache(startTime, endTime int64, userId int, tokenName string, excludeModels string) ([]TokenUsageStat, error) {
	// 如果Redis未启用，直接使用MySQL
	if !common.RedisEnabled {
		return GetTokenUsageByName(startTime, endTime, userId, tokenName, excludeModels)
	}

	statsMap := make(map[string]*TokenUsageStat)

	// 按数据源划分时间范围
	partition := PartitionByDataSource(startTime, endTime)

	// 1. 查询MySQL区间（历史冷数据 + 今天实时数据）
	for _, mysqlRange := range partition.MySQLRanges {
		mysqlStats, err := GetTokenUsageByName(mysqlRange.Start, mysqlRange.End, userId, tokenName, excludeModels)
		if err != nil {
			return nil, fmt.Errorf("MySQL查询失败[%d-%d]: %w", mysqlRange.Start, mysqlRange.End, err)
		}
		for i := range mysqlStats {
			stat := &mysqlStats[i] // 使用索引访问，避免循环变量重用问题
			key := stat.Username + ":" + stat.TokenName
			if existing, ok := statsMap[key]; ok {
				existing.TotalTokens += stat.TotalTokens
				existing.PromptTokens += stat.PromptTokens
				existing.CompletionTokens += stat.CompletionTokens
				existing.RequestCount += stat.RequestCount
				if stat.LastUsedTime > existing.LastUsedTime {
					existing.LastUsedTime = stat.LastUsedTime
				}
			} else {
				statsMap[key] = stat
			}
		}
	}

	// 2. 查询Redis区间（昨天到179天前的缓存数据）
	if len(partition.RedisDates) > 0 {
		var redisStats map[string]*TokenUsageStat
		var err error

		if userId != 0 {
			// 单用户模式
			redisStats, err = getTokenStatsFromRedisDays(&userId, partition.RedisDates, excludeModels)
		} else {
			// 所有用户模式
			redisStats, err = getTokenStatsFromRedisDays(nil, partition.RedisDates, excludeModels)
		}

		if err != nil {
			logger.SysError(fmt.Sprintf("Redis查询失败，跳过缓存数据: %s", err.Error()))
		} else {
			for _, stat := range redisStats {
				// 如果指定了tokenName，只获取该token的数据
				if tokenName != "" && stat.TokenName != tokenName {
					continue
				}
				key := stat.Username + ":" + stat.TokenName
				if existing, ok := statsMap[key]; ok {
					existing.TotalTokens += stat.TotalTokens
					existing.PromptTokens += stat.PromptTokens
					existing.CompletionTokens += stat.CompletionTokens
					existing.RequestCount += stat.RequestCount
					if stat.LastUsedTime > existing.LastUsedTime {
						existing.LastUsedTime = stat.LastUsedTime
					}
				} else {
					statsMap[key] = stat
				}
			}
		}
	}

	// 3. 转换为数组返回
	result := make([]TokenUsageStat, 0, len(statsMap))
	for _, stat := range statsMap {
		result = append(result, *stat)
	}

	// 按 total_tokens 降序排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalTokens > result[j].TotalTokens
	})

	return result, nil
}

// GetTokenUsageByName 获取指定时间范围内的Token使用统计
func GetTokenUsageByName(startTime, endTime int64, userId int, tokenName string, excludeModels string) ([]TokenUsageStat, error) {
	tables := getTablesForRange(startTime, endTime)

	ifnull := "ifnull"
	if common.UsingPostgreSQL {
		ifnull = "COALESCE"
	}

	// 构建WHERE条件
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "type = ? AND created_at BETWEEN ? AND ?")
	args = append(args, LogTypeConsume, startTime, endTime)

	if userId > 0 {
		conditions = append(conditions, "user_id = ?")
		args = append(args, userId)
	}
	if tokenName != "" {
		conditions = append(conditions, "token_name = ?")
		args = append(args, tokenName)
	}
	if excludeModels != "" {
		conditions = append(conditions, "model_name NOT IN (?)")
		args = append(args, strings.Split(excludeModels, ","))
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")

	// 构建UNION ALL查询
	sqlTemplate := fmt.Sprintf(
		"SELECT username, token_name, "+
			"%s(sum(prompt_tokens + completion_tokens),0) as total_tokens, "+
			"%s(sum(prompt_tokens),0) as prompt_tokens, "+
			"%s(sum(completion_tokens),0) as completion_tokens, "+
			"COUNT(id) as request_count, "+
			"MAX(created_at) as last_used_time "+
			"FROM {TABLE}%s "+
			"GROUP BY username, token_name",
		ifnull, ifnull, ifnull, whereClause)

	unionSQL := buildUnionSQL(tables, sqlTemplate)
	finalSQL := fmt.Sprintf("SELECT * FROM (%s) AS logs ORDER BY total_tokens DESC", unionSQL)

	// 为每个表重复参数
	var finalArgs []interface{}
	for range tables {
		finalArgs = append(finalArgs, args...)
	}

	var stats []TokenUsageStat
	err := LOG_DB.Raw(finalSQL, finalArgs...).Scan(&stats).Error
	return stats, err
}

// GetCombinedDailyUsageStats 获取多个token的合并日使用统计
func GetCombinedDailyUsageStats(tokenKeys []string, startTimestamp int64, endTimestamp int64) (map[string]int, bool) {
	// 构建日期格式选择
	groupSelect := "DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d') as day"
	if common.UsingPostgreSQL {
		groupSelect = "TO_CHAR(date_trunc('day', to_timestamp(created_at)), 'YYYY-MM-DD') as day"
	}
	if common.UsingSQLite {
		groupSelect = "strftime('%Y-%m-%d', datetime(created_at, 'unixepoch')) as day"
	}

	// 查询所有有效的token
	var tokens []Token
	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	err := DB.Where(keyCol+" IN (?)", tokenKeys).Find(&tokens).Error
	if err != nil {
		// 出现错误，返回空结果
		return make(map[string]int), true
	}

	// 提取有效的token名称
	var validTokenNames []string
	for _, token := range tokens {
		validTokenNames = append(validTokenNames, token.Name)
	}

	hasErrors := len(validTokenNames) != len(tokenKeys) // 如果有无效token则标记有错误

	if len(validTokenNames) == 0 {
		return make(map[string]int), true
	}

	// 查询日志统计，直接按日期聚合所有token的使用量
	var results []struct {
		Day   string `gorm:"column:day"`
		Count int    `gorm:"column:count"`
	}

	// 获取涉及的分表
	tables := getTablesForRange(startTimestamp, endTimestamp)

	// 构建UNION ALL查询
	sqlTemplate := fmt.Sprintf(`
		SELECT %s, COUNT(1) as count_per_token
		FROM {TABLE}
			WHERE type = ? 
			AND token_name IN (?)
			AND created_at BETWEEN ? AND ?
		GROUP BY token_name, day`, groupSelect)

	unionSQL := buildUnionSQL(tables, sqlTemplate)

	// 为每个表重复参数
	var args []interface{}
	for range tables {
		args = append(args, LogTypeConsume, validTokenNames, startTimestamp, endTimestamp)
	}

	// 最终查询
	finalSQL := fmt.Sprintf(`
		SELECT day, SUM(count_per_token) as count
		FROM (%s) as daily_counts
		GROUP BY day
		ORDER BY day ASC
	`, unionSQL)

	tx := LOG_DB.Raw(finalSQL, args...)

	err = tx.Scan(&results).Error
	if err != nil {
		return make(map[string]int), true
	}

	// 转换为map格式
	usageMap := make(map[string]int)
	for _, r := range results {
		usageMap[r.Day] = r.Count
	}

	return usageMap, hasErrors
}

// GetCombinedTotalUsage 获取多个token的合并总使用量
func GetCombinedTotalUsage(tokenKeys []string) (int, bool) {
	// 查询所有有效的token
	var tokens []Token
	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	err := DB.Where(keyCol+" IN (?)", tokenKeys).Find(&tokens).Error
	if err != nil {
		return 0, true
	}

	// 提取有效的token名称
	var validTokenNames []string
	for _, token := range tokens {
		validTokenNames = append(validTokenNames, token.Name)
	}

	hasErrors := len(validTokenNames) != len(tokenKeys) // 如果有无效token则标记有错误

	if len(validTokenNames) == 0 {
		return 0, true
	}

	// 获取最近1年的分表（统计总使用量通常看最近的数据）
	tables := getTablesForRecentPeriod()

	// 构建UNION ALL查询
	sqlTemplate := "SELECT COUNT(*) as count FROM {TABLE} WHERE token_name IN (?) AND type = ?"
	unionSQL := buildUnionSQL(tables, sqlTemplate)

	// 为每个表重复参数
	var args []interface{}
	for range tables {
		args = append(args, validTokenNames, LogTypeConsume)
	}

	finalSQL := fmt.Sprintf("SELECT SUM(count) as total FROM (%s) AS counts", unionSQL)

	// 查询所有token的总使用次数
	var totalCount int64
	err = LOG_DB.Raw(finalSQL, args...).Scan(&totalCount).Error

	if err != nil {
		return 0, true
	}

	return int(totalCount), hasErrors
}
