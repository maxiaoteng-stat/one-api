package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

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
	err := LOG_DB.Create(log).Error
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
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("type = ?", logType)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	return logs, err
}

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int) (logs []*Log, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB.Where("user_id = ?", userId)
	} else {
		tx = LOG_DB.Where("user_id = ? and type = ?", userId, logType)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Omit("id").Find(&logs).Error
	return logs, err
}

func SearchAllLogs(keyword string) (logs []*Log, err error) {
	err = LOG_DB.Where("type = ? or content LIKE ?", keyword, keyword+"%").Order("id desc").Limit(config.MaxRecentItems).Find(&logs).Error
	return logs, err
}

func SearchUserLogs(userId int, keyword string) (logs []*Log, err error) {
	err = LOG_DB.Where("user_id = ? and type = ?", userId, keyword).Order("id desc").Limit(config.MaxRecentItems).Omit("id").Find(&logs).Error
	return logs, err
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, excludeModels string) (quota int64) {
	ifnull := "ifnull"
	if common.UsingPostgreSQL {
		ifnull = "COALESCE"
	}
	tx := LOG_DB.Table("logs").Select(fmt.Sprintf("%s(sum(quota),0)", ifnull))
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	if excludeModels != "" {
		tx = tx.Where("model_name NOT IN (?)", strings.Split(excludeModels, ","))
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&quota)
	return quota
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	ifnull := "ifnull"
	if common.UsingPostgreSQL {
		ifnull = "COALESCE"
	}
	tx := LOG_DB.Table("logs").Select(fmt.Sprintf("%s(sum(prompt_tokens),0) + %s(sum(completion_tokens),0)", ifnull, ifnull))
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

func DeleteOldLog(targetTimestamp int64) (int64, error) {
	result := LOG_DB.Where("created_at < ?", targetTimestamp).Delete(&Log{})
	return result.RowsAffected, result.Error
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
	groupSelect := "DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d') as day"

	if common.UsingPostgreSQL {
		groupSelect = "TO_CHAR(date_trunc('day', to_timestamp(created_at)), 'YYYY-MM-DD') as day"
	}

	if common.UsingSQLite {
		groupSelect = "strftime('%Y-%m-%d', datetime(created_at, 'unixepoch')) as day"
	}

	var tx *gorm.DB
	if excludeModels == "" {
		tx = LOG_DB.Raw(`
			SELECT `+groupSelect+`,
			model_name, count(1) as request_count,
			sum(quota) as quota,
			sum(prompt_tokens) as prompt_tokens,
			sum(completion_tokens) as completion_tokens
			FROM logs
			WHERE type=2
			AND user_id= ?
			AND created_at BETWEEN ? AND ?
			GROUP BY day, model_name
			ORDER BY day, model_name`, userId, start, end)
	} else {
		tx = LOG_DB.Raw(`
			SELECT `+groupSelect+`,
			model_name, count(1) as request_count,
			sum(quota) as quota,
			sum(prompt_tokens) as prompt_tokens,
			sum(completion_tokens) as completion_tokens
			FROM logs
			WHERE type=2
			AND user_id= ?
			AND created_at BETWEEN ? AND ?
			AND model_name NOT IN (?)
			GROUP BY day, model_name
			ORDER BY day, model_name`, userId, start, end, strings.Split(excludeModels, ","))
	}

	err = tx.Scan(&LogStatistics).Error

	return LogStatistics, err
}

// GetUserTokenModelUsage 统计单个用户token下各个模型在不同时间的使用量
func GetUserTokenModelUsage(userId int, tokenName string, startTimestamp int64, endTimestamp int64, excludeModels string) ([]struct {
	ModelName    string `json:"model_name"`
	CreatedAt    string `json:"created_at"`
	Usage        int    `json:"usage"`
	RequestCount int    `json:"request_count"`
}, error) {
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

	tx := LOG_DB.Table("logs").Select(fmt.Sprintf(
		"model_name, date_format(from_unixtime(created_at), '%%Y-%%m-%%d %%H:%%i:%%s') as created_at, "+
			"%s(sum(prompt_tokens + completion_tokens),0) as `usage`, COUNT(1) as request_count",
		ifnull))

	// 构建查询条件
	tx = tx.Where("user_id = ? AND token_name = ?", userId, tokenName)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if excludeModels != "" {
		tx = tx.Where("model_name NOT IN (?)", strings.Split(excludeModels, ","))
	}

	// 分组并执行查询
	err := tx.Group("model_name, DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d %H:%i:%s')").
		Scan(&results).Error

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

// GetAllUserTokenStats 获取所有用户的Token使用统计
func GetAllUserTokenStats(startTimestamp, endTimestamp int64, excludeModels string) ([]TokenUsageStat, error) {
	ifnull := "ifnull"
	if common.UsingPostgreSQL {
		ifnull = "COALESCE"
	}

	// 使用链式方法构建查询
	tx := LOG_DB.Table("logs").Select(fmt.Sprintf(
		"username, token_name, "+
			"%s(sum(prompt_tokens + completion_tokens),0) as total_tokens, "+
			"%s(sum(prompt_tokens),0) as prompt_tokens, "+
			"%s(sum(completion_tokens),0) as completion_tokens, "+
			"COUNT(id) as request_count, "+
			"MAX(created_at) as last_used_time",
		ifnull, ifnull, ifnull))

	// 构建查询条件
	tx = tx.Where("type = ? AND created_at BETWEEN ? AND ?", LogTypeConsume, startTimestamp, endTimestamp)

	if excludeModels != "" {
		tx = tx.Where("model_name NOT IN (?)", strings.Split(excludeModels, ","))
	}

	// 分组并排序
	tx = tx.Group("username, token_name").Order("total_tokens DESC")

	// 执行查询
	var stats []TokenUsageStat
	err := tx.Scan(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// GetUserTokenStats 获取特定用户的Token使用统计
func GetUserTokenStats(userId int, tokenName string, startTimestamp, endTimestamp int64, excludeModels string) ([]TokenUsageStat, error) {
	ifnull := "ifnull"
	if common.UsingPostgreSQL {
		ifnull = "COALESCE"
	}

	// 使用链式方法构建查询
	tx := LOG_DB.Table("logs").Select(fmt.Sprintf(
		"username, token_name, "+
			"%s(sum(prompt_tokens + completion_tokens),0) as total_tokens, "+
			"%s(sum(prompt_tokens),0) as prompt_tokens, "+
			"%s(sum(completion_tokens),0) as completion_tokens, "+
			"COUNT(id) as request_count, "+
			"MAX(created_at) as last_used_time",
		ifnull, ifnull, ifnull))

	// 构建查询条件
	tx = tx.Where("user_id = ? AND type = ? AND token_name = ? AND created_at BETWEEN ? AND ?",
		userId, LogTypeConsume, tokenName, startTimestamp, endTimestamp)

	if excludeModels != "" {
		tx = tx.Where("model_name NOT IN (?)", strings.Split(excludeModels, ","))
	}

	// 分组并排序
	tx = tx.Group("username, token_name").Order("total_tokens DESC")

	// 执行查询
	var stats []TokenUsageStat
	err := tx.Scan(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// GetTokenUsageByName 获取指定时间范围内的Token使用统计
func GetTokenUsageByName(startTime, endTime int64, userId int, tokenName string, excludeModels string) ([]TokenUsageStat, error) {
	ifnull := "ifnull"
	if common.UsingPostgreSQL {
		ifnull = "COALESCE"
	}

	var stats []TokenUsageStat
	query := LOG_DB.Table("logs").Select(fmt.Sprintf(
		"username, token_name, "+
			"%s(sum(prompt_tokens + completion_tokens),0) as total_tokens, "+
			"%s(sum(prompt_tokens),0) as prompt_tokens, "+
			"%s(sum(completion_tokens),0) as completion_tokens, "+
			"COUNT(id) as request_count, "+
			"MAX(created_at) as last_used_time",
		ifnull, ifnull, ifnull)).
		Where("type = ? AND created_at BETWEEN ? AND ?", LogTypeConsume, startTime, endTime).
		Group("username, token_name").
		Order("total_tokens DESC")

	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if tokenName != "" {
		query = query.Where("token_name = ?", tokenName)
	}
	if excludeModels != "" {
		query = query.Where("model_name NOT IN (?)", strings.Split(excludeModels, ","))
	}

	err := query.Scan(&stats).Error
	return stats, err
}

// GetDailyUsageStats 获取特定token在一段时间内按天统计的使用次数
func GetDailyUsageStats(tokenKey string, startTimestamp int64, endTimestamp int64) (map[string]int, error) {
	// 构建日期格式选择
	groupSelect := "DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d') as day"
	if common.UsingPostgreSQL {
		groupSelect = "TO_CHAR(date_trunc('day', to_timestamp(created_at)), 'YYYY-MM-DD') as day"
	}
	if common.UsingSQLite {
		groupSelect = "strftime('%Y-%m-%d', datetime(created_at, 'unixepoch')) as day"
	}

	// 从tokens表中查询该key对应的token
	var token Token
	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	err := DB.Where(keyCol+" = ?", tokenKey).First(&token).Error
	if err != nil {
		return nil, errors.New("无效的令牌")
	}

	// 查询日志统计
	var results []struct {
		Day   string `gorm:"column:day"`
		Count int    `gorm:"column:count"`
	}

	tx := LOG_DB.Raw(`
		SELECT `+groupSelect+`, COUNT(1) as count
		FROM logs
		WHERE type = ? 
		AND token_name = ?
		AND created_at BETWEEN ? AND ?
		GROUP BY day
		ORDER BY day ASC
	`, LogTypeConsume, token.Name, startTimestamp, endTimestamp)

	err = tx.Scan(&results).Error
	if err != nil {
		return nil, err
	}

	// 转换为map格式
	usageMap := make(map[string]int)
	for _, r := range results {
		usageMap[r.Day] = r.Count
	}

	return usageMap, nil
}

// GetTotalUsage 获取指定token的总使用次数
func GetTotalUsage(tokenKey string) (int, error) {
	// 从tokens表中查询该key对应的token
	var token Token
	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	err := DB.Where(keyCol+" = ?", tokenKey).First(&token).Error
	if err != nil {
		return 0, errors.New("无效的令牌")
	}

	// 查询该token的总使用次数
	var count int64
	err = LOG_DB.Model(&Log{}).
		Where("token_name = ? AND type = ?", token.Name, LogTypeConsume).
		Count(&count).Error

	if err != nil {
		return 0, err
	}

	return int(count), nil
}
