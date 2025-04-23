package common

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// IncrAndGetQPSCounter 递增并获取QPS计数器值
func IncrAndGetQPSCounter(key string, expireSeconds int) (int64, error) {
	if !RedisEnabled {
		return 0, fmt.Errorf("redis not enabled")
	}

	ctx := context.Background()
	redisKey := "qps_counter:" + key

	// 使用INCR命令原子递增计数器
	val, err := RDB.Incr(ctx, redisKey).Result()
	if err != nil {
		return 0, err
	}

	// 如果是新的计数器，设置过期时间
	if val == 1 {
		RDB.Expire(ctx, redisKey, time.Duration(expireSeconds)*time.Second)
	}

	return val, nil
}

// IncrAndGetDailyCounter 递增并获取日配额计数器值
func IncrAndGetDailyCounter(key string) (int64, error) {
	if !RedisEnabled {
		return 0, fmt.Errorf("redis not enabled")
	}

	ctx := context.Background()
	now := time.Now()
	date := now.Format("20060102")
	redisKey := fmt.Sprintf("daily_counter:%s:%s", key, date)

	// 使用INCR命令原子递增计数器
	val, err := RDB.Incr(ctx, redisKey).Result()
	if err != nil {
		return 0, err
	}

	// 如果是新的计数器，设置过期时间到当天结束
	if val == 1 {
		// 计算当天剩余的秒数
		tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		ttl := tomorrow.Sub(now)
		RDB.Expire(ctx, redisKey, ttl)
	}

	return val, nil
}

// GetQueueLength 获取当前队列长度
func GetQueueLength(key string) (int64, error) {
	if !RedisEnabled {
		return 0, fmt.Errorf("redis not enabled")
	}

	ctx := context.Background()
	redisKey := "request_queue:" + key

	return RDB.LLen(ctx, redisKey).Result()
}

// EnqueueRequest 将请求加入队列
func EnqueueRequest(key string, requestId string, timeout int) error {
	if !RedisEnabled {
		return fmt.Errorf("redis not enabled")
	}

	ctx := context.Background()
	redisKey := "request_queue:" + key

	// 将请求ID放入队列
	err := RDB.RPush(ctx, redisKey, requestId).Err()
	if err != nil {
		return err
	}

	// 设置请求超时
	timeoutKey := "request_timeout:" + requestId
	err = RDB.Set(ctx, timeoutKey, time.Now().Unix(), time.Duration(timeout)*time.Second).Err()

	return err
}

// DequeueRequest 从队列取出请求
func DequeueRequest(key string) (string, error) {
	if !RedisEnabled {
		return "", fmt.Errorf("redis not enabled")
	}

	ctx := context.Background()
	redisKey := "request_queue:" + key

	// 取出队列头部的请求ID
	requestId, err := RDB.LPop(ctx, redisKey).Result()
	if err != nil {
		return "", err
	}

	return requestId, nil
}

// IsRequestTimedOut 检查请求是否已超时
func IsRequestTimedOut(requestId string) (bool, error) {
	if !RedisEnabled {
		return false, fmt.Errorf("redis not enabled")
	}

	ctx := context.Background()
	timeoutKey := "request_timeout:" + requestId

	// 检查超时键是否存在
	exists, err := RDB.Exists(ctx, timeoutKey).Result()
	if err != nil {
		return false, err
	}

	// 如果键不存在，表示请求已超时
	return exists == 0, nil
}

// GetDailyQuotaUsage 获取当日配额使用情况
func GetDailyQuotaUsage(prefix string) (int64, error) {
	if !RedisEnabled {
		return 0, fmt.Errorf("redis not enabled")
	}

	ctx := context.Background()
	date := time.Now().Format("20060102")
	redisKey := fmt.Sprintf("daily_counter:%s:%s", prefix, date)

	// 获取当前计数
	val, err := RDB.Get(ctx, redisKey).Result()
	if err != nil {
		if err.Error() == "redis: nil" {
			return 0, nil // 键不存在，表示今天还没有请求
		}
		return 0, err
	}

	count, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, err
	}

	return count, nil
}
