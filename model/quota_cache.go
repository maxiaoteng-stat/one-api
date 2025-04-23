package model

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/logger"
)

const (
	DailyUsageKeyPrefix = "daily_usage:"
	DailyUsageTTL       = 24 * time.Hour // 一天过期
)

// InitDailyUsageCache 服务启动时，预加载当天所有token的使用量到Redis
func InitDailyUsageCache() {
	if !common.RedisEnabled {
		logger.SysLog("Redis未启用，跳过加载日使用量")
		return
	}

	logger.SysLog("开始加载当天token使用量到Redis")
	now := time.Now()
	date := now.Format("20060102")

	// 获取当天的开始时间戳
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startTimestamp := startOfDay.Unix()
	endTimestamp := now.Unix()

	// 计算全局使用量并设置
	globalUsage := SumUsedQuota(
		LogTypeConsume,
		startTimestamp,
		endTimestamp,
		"", // 不限模型
		"", // 不限用户
		"", // 不限token
		0,  // 不限渠道
		"", // 不排除任何模型
	)
	globalKey := fmt.Sprintf("%sglobal:%s", DailyUsageKeyPrefix, date)
	common.RedisSet(globalKey, strconv.FormatInt(globalUsage, 10), DailyUsageTTL)

	// 获取所有token并加载各自使用量
	var tokens []*Token
	err := DB.Find(&tokens).Error
	if err != nil {
		logger.SysError("加载token列表失败: " + err.Error())
		return
	}

	// 为每个token加载日使用量
	for _, token := range tokens {
		// 从日志表中计算当天已使用的配额
		usedQuota := SumUsedQuota(
			LogTypeConsume,
			startTimestamp,
			endTimestamp,
			"",         // 不限模型
			"",         // 不限用户
			token.Name, // 按token名称
			0,          // 不限渠道
			"",         // 不排除任何模型
		)

		// 将使用量保存到Redis
		key := fmt.Sprintf("%s%s:%s", DailyUsageKeyPrefix, token.Name, date)
		err := common.RedisSet(key, strconv.FormatInt(usedQuota, 10), DailyUsageTTL)
		if err != nil {
			logger.SysError(fmt.Sprintf("保存token %s 日使用量到Redis失败: %s", token.Name, err.Error()))
			continue
		}

		logger.SysLog(fmt.Sprintf("已加载token %s (ID:%d) 日使用量: %d", token.Name, token.Id, usedQuota))
	}

	logger.SysLog(fmt.Sprintf("当天使用量加载完成，全局使用量: %d", globalUsage))
}

// GetTokenDailyUsage 获取token的日使用量(优先从Redis获取)
func GetTokenDailyUsage(tokenName string) (int64, error) {
	if !common.RedisEnabled {
		// Redis未启用，从数据库计算
		return getTokenDailyUsageFromDB(tokenName)
	}

	// 从Redis获取
	date := time.Now().Format("20060102")
	key := fmt.Sprintf("%s%s:%s", DailyUsageKeyPrefix, tokenName, date)

	usageStr, err := common.RedisGet(key)
	if err != nil {
		// Redis中不存在，从数据库获取
		return getTokenDailyUsageFromDB(tokenName)
	}

	usage, err := strconv.ParseInt(usageStr, 10, 64)
	if err != nil {
		logger.SysError("解析Redis中的日使用量失败: " + err.Error())
		return getTokenDailyUsageFromDB(tokenName)
	}

	return usage, nil
}

// IncrTokenDailyUsage 增加token的日使用量
func IncrTokenDailyUsage(tokenName string, usage int64) error {
	if !common.RedisEnabled {
		return nil
	}

	date := time.Now().Format("20060102")
	key := fmt.Sprintf("%s%s:%s", DailyUsageKeyPrefix, tokenName, date)

	ctx := context.Background()
	// 使用Redis的INCRBY命令增加使用量
	_, err := common.RDB.IncrBy(ctx, key, usage).Result()
	if err != nil {
		logger.SysError(fmt.Sprintf("增加token %s 日使用量失败: %s", tokenName, err.Error()))
		return err
	}

	// 确保设置过期时间
	common.RDB.Expire(ctx, key, DailyUsageTTL)

	// 同时增加全局使用量
	globalKey := fmt.Sprintf("%sglobal:%s", DailyUsageKeyPrefix, date)
	_, err = common.RDB.IncrBy(ctx, globalKey, usage).Result()
	if err != nil {
		logger.SysError("增加全局日使用量失败: " + err.Error())
	} else {
		common.RDB.Expire(ctx, globalKey, DailyUsageTTL)
	}

	return nil
}

// GetGlobalDailyUsage 获取全局日使用量
func GetGlobalDailyUsage() (int64, error) {
	if !common.RedisEnabled {
		return getGlobalDailyUsageFromDB()
	}

	date := time.Now().Format("20060102")
	key := fmt.Sprintf("%sglobal:%s", DailyUsageKeyPrefix, date)

	usageStr, err := common.RedisGet(key)
	if err != nil {
		// 从数据库获取数据
		usage, dbErr := getGlobalDailyUsageFromDB()
		if dbErr != nil {
			return 0, dbErr
		}

		// 获取成功后，将数据写回Redis缓存
		// 设置过期时间为当天剩余时间
		expiration := getEndOfDayDuration()
		cacheErr := common.RedisSet(key, strconv.FormatInt(usage, 10), expiration)
		if cacheErr != nil {
			logger.SysWarn("缓存全局日使用量到Redis失败: " + cacheErr.Error())
			// 继续处理，不因缓存写入失败而中断流程
		}

		return usage, nil
	}

	usage, err := strconv.ParseInt(usageStr, 10, 64)
	if err != nil {
		logger.SysError("解析Redis中的全局日使用量失败: " + err.Error())

		// 从数据库获取数据
		usage, dbErr := getGlobalDailyUsageFromDB()
		if dbErr != nil {
			return 0, dbErr
		}

		// 获取成功后，将正确的数据写回Redis缓存
		expiration := getEndOfDayDuration()
		cacheErr := common.RedisSet(key, strconv.FormatInt(usage, 10), expiration)
		if cacheErr != nil {
			logger.SysWarn("缓存全局日使用量到Redis失败: " + cacheErr.Error())
		}

		return usage, nil
	}

	return usage, nil
}

// 获取到当天结束的剩余时间
func getEndOfDayDuration() time.Duration {
	now := time.Now()
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, now.Location())
	return endOfDay.Sub(now) + time.Second // 添加1秒确保覆盖整天
}

// 从数据库获取token的日使用量
func getTokenDailyUsageFromDB(tokenName string) (int64, error) {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startTimestamp := startOfDay.Unix()
	endTimestamp := now.Unix()

	usage := SumUsedQuota(
		LogTypeConsume,
		startTimestamp,
		endTimestamp,
		"",        // 不限模型
		"",        // 不限用户
		tokenName, // 按token名称
		0,         // 不限渠道
		"",        // 不排除任何模型
	)

	return usage, nil
}

// 从数据库获取全局日使用量
func getGlobalDailyUsageFromDB() (int64, error) {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startTimestamp := startOfDay.Unix()
	endTimestamp := now.Unix()

	usage := SumUsedQuota(
		LogTypeConsume,
		startTimestamp,
		endTimestamp,
		"", // 不限模型
		"", // 不限用户
		"", // 不限token
		0,  // 不限渠道
		"", // 不排除任何模型
	)

	return usage, nil
}
