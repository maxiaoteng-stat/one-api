package common

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
)

// 内存滑动窗口限流器
type InMemorySlidingWindow struct {
	store              map[string][]int64
	mutex              sync.Mutex
	expirationDuration time.Duration
}

func (l *InMemorySlidingWindow) Init(expirationDuration time.Duration) {
	if l.store == nil {
		l.mutex.Lock()
		if l.store == nil {
			l.store = make(map[string][]int64)
			l.expirationDuration = expirationDuration
			if expirationDuration > 0 {
				go l.clearExpiredItems()
			}
		}
		l.mutex.Unlock()
	}
}

func (l *InMemorySlidingWindow) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.mutex.Lock()
		now := time.Now().UnixMilli()
		for key := range l.store {
			timestamps := l.store[key]
			if len(timestamps) == 0 {
				delete(l.store, key)
				continue
			}

			// 清理窗口外的时间戳
			var i int
			for i = 0; i < len(timestamps); i++ {
				if now-timestamps[i] <= int64(l.expirationDuration.Milliseconds()) {
					break
				}
			}

			if i > 0 {
				if i >= len(timestamps) {
					delete(l.store, key)
				} else {
					l.store[key] = timestamps[i:]
				}
			}
		}
		l.mutex.Unlock()
	}
}

// AllowRequest 检查请求是否允许通过
// windowSize 时间窗口大小，单位毫秒
// maxRequests 窗口内最大请求数
func (l *InMemorySlidingWindow) AllowRequest(key string, windowSize int64, maxRequests int) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	now := time.Now().UnixMilli()
	timestamps, exists := l.store[key]

	// 如果键不存在，创建新的时间戳数组
	if !exists {
		l.store[key] = []int64{now}
		return true
	}

	// 移除窗口外的时间戳
	windowStart := now - windowSize
	var validIndex int
	for validIndex = 0; validIndex < len(timestamps); validIndex++ {
		if timestamps[validIndex] > windowStart {
			break
		}
	}

	if validIndex > 0 {
		timestamps = timestamps[validIndex:]
		l.store[key] = timestamps
	}

	// 检查当前窗口中的请求数
	if len(timestamps) < maxRequests {
		l.store[key] = append(timestamps, now)
		return true
	}

	return false
}

// GetQPS 获取当前每秒请求数并递增
func (l *InMemorySlidingWindow) GetQPS(key string) int {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	now := time.Now().UnixMilli()
	timestamps, exists := l.store[key]

	// 如果键不存在，创建新的时间戳数组
	if !exists {
		l.store[key] = []int64{now}
		return 1
	}

	// 移除1秒窗口外的时间戳
	windowStart := now - 1000
	var validIndex int
	for validIndex = 0; validIndex < len(timestamps); validIndex++ {
		if timestamps[validIndex] > windowStart {
			break
		}
	}

	if validIndex > 0 {
		timestamps = timestamps[validIndex:]
	}

	// 添加当前时间戳并返回窗口内请求数
	timestamps = append(timestamps, now)
	l.store[key] = timestamps
	return len(timestamps)
}

// Redis滑动窗口限流器
type RedisSlidingWindow struct{}

// NewRedisSlidingWindow 创建Redis滑动窗口限流器
func NewRedisSlidingWindow() *RedisSlidingWindow {
	return &RedisSlidingWindow{}
}

// QPS计数Lua脚本
const qpsCountScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local expireTime = tonumber(ARGV[2])

-- 清理1秒前的时间戳
local windowStart = now - 1000
redis.call('ZREMRANGEBYSCORE', key, 0, windowStart)

-- 添加当前时间戳
redis.call('ZADD', key, now, now .. ':' .. math.random())
redis.call('EXPIRE', key, expireTime)

-- 获取当前1秒内的请求数
local count = redis.call('ZCARD', key)
return count
`

// GetQPS 获取当前QPS并递增计数
func (r *RedisSlidingWindow) GetQPS(key string, expireSeconds int) (int64, error) {
	if !RedisEnabled {
		return 0, fmt.Errorf("redis not enabled")
	}

	ctx := context.Background()
	now := time.Now().UnixMilli()
	redisKey := "qps:" + key

	result, err := RDB.Eval(
		ctx,
		qpsCountScript,
		[]string{redisKey},
		now,
		expireSeconds,
	).Int64()

	if err != nil {
		logger.SysError("执行Redis QPS脚本失败: " + err.Error())
		return 0, err
	}

	return result, nil
}

// 添加一个新方法，将检查和计数结合在一起
func (r *RedisSlidingWindow) CheckAndIncrease(key string, windowSize int64, maxRequests int, expireSeconds int) (allowed bool, count int64, err error) {
	if !RedisEnabled {
		return false, 0, fmt.Errorf("redis not enabled")
	}

	ctx := context.Background()
	now := time.Now().UnixMilli()

	// 使用一个Lua脚本完成检查和计数
	const checkAndIncreaseScript = `
	local key = KEYS[1]
	local now = tonumber(ARGV[1])
	local windowSize = tonumber(ARGV[2])
	local maxRequests = tonumber(ARGV[3])
	local expireTime = tonumber(ARGV[4])

	-- 清理过期的时间戳（仅清理超过限流窗口的数据，保留历史数据用于图表显示）
	local windowStart = now - windowSize
	redis.call('ZREMRANGEBYSCORE', key, 0, windowStart - 3600000) -- 仅删除1小时前的数据

	-- 获取当前窗口中的请求数
	local count = redis.call('ZCOUNT', key, windowStart, now)

	-- 是否允许请求 + 添加时间戳
	local allowed = 0
	if count < maxRequests then
		redis.call('ZADD', key, now, now .. ':' .. math.random())
		redis.call('EXPIRE', key, expireTime)
		allowed = 1
	end

	-- 返回允许状态和当前计数
	return {allowed, count + (allowed == 1 and 1 or 0)}
	`

	redisKey := "sliding_window:" + key
	result, err := RDB.Eval(
		ctx,
		checkAndIncreaseScript,
		[]string{redisKey},
		now,
		windowSize,
		maxRequests,
		expireSeconds,
	).Result()

	if err != nil {
		logger.SysError("执行Redis检查和计数脚本失败: " + err.Error())
		return false, 0, err
	}

	// 解析结果
	resultArray := result.([]interface{})
	allowedResult := resultArray[0].(int64)
	countResult := resultArray[1].(int64)

	return allowedResult == 1, countResult, nil
}

// 同样为内存实现添加一个类似的方法
func (l *InMemorySlidingWindow) CheckAndIncrease(key string, windowSize int64, maxRequests int) (bool, int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	now := time.Now().UnixMilli()
	timestamps, exists := l.store[key]

	// 如果键不存在，创建新的时间戳数组
	if !exists {
		l.store[key] = []int64{now}
		return true, 1
	}

	// 移除窗口外的时间戳
	windowStart := now - windowSize
	var validIndex int
	for validIndex = 0; validIndex < len(timestamps); validIndex++ {
		if timestamps[validIndex] > windowStart {
			break
		}
	}

	if validIndex > 0 {
		timestamps = timestamps[validIndex:]
	}

	// 检查当前窗口中的请求数并决定是否允许
	count := len(timestamps)
	if count < maxRequests {
		timestamps = append(timestamps, now)
		l.store[key] = timestamps
		return true, count + 1
	}

	return false, count
}

// 内存滑动窗口实例
var InMemorySlidingWindowLimiter InMemorySlidingWindow

// 全局Redis滑动窗口实例
var RedisSlidingWindowLimiter *RedisSlidingWindow

// 初始化滑动窗口限流器
func InitSlidingWindowLimiter() {
	logger.SysLog("初始化滑动窗口限流器")
	// 初始化Redis滑动窗口
	if RedisEnabled {
		RedisSlidingWindowLimiter = NewRedisSlidingWindow()
	} else {
		// 初始化内存滑动窗口
		InMemorySlidingWindowLimiter.Init(config.RateLimitKeyExpirationDuration)
	}
}

// IncrAndGetQPS 递增并获取当前QPS（滑动窗口实现）
func IncrAndGetQPS(key string, expireSeconds int) (int64, error) {
	if !RedisEnabled {
		// 使用内存滑动窗口
		count := InMemorySlidingWindowLimiter.GetQPS(key)
		return int64(count), nil
	}

	// 使用Redis滑动窗口
	return RedisSlidingWindowLimiter.GetQPS(key, expireSeconds)
}

// 获取QPS历史数据的Lua脚本
const getQPSHistoryScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local duration = tonumber(ARGV[2]) -- 单位：毫秒
local interval = tonumber(ARGV[3]) -- 单位：毫秒

local startTime = now - duration
local result = {}

-- 获取所有时间戳数据
local allData = redis.call('ZRANGEBYSCORE', key, startTime, now, 'WITHSCORES')
local timeMap = {}

-- 将数据按时间间隔分组
for i = 1, #allData, 2 do
    local member = allData[i]
    local score = tonumber(allData[i+1])
    
    -- 计算该时间戳属于哪个时间间隔
    local intervalStart = math.floor((score - startTime) / interval) * interval + startTime
    
    -- 初始化该时间间隔的计数
    if not timeMap[intervalStart] then
        timeMap[intervalStart] = 0
    end
    
    -- 增加该时间间隔的计数
    timeMap[intervalStart] = timeMap[intervalStart] + 1
end

-- 按照指定间隔生成完整的时间序列（包括没有数据的时间点）
local currentInterval = startTime
while currentInterval <= now do
    local count = timeMap[currentInterval] or 0
    
    -- 添加到结果数组: [时间戳1, 计数1, 时间戳2, 计数2, ...]
    table.insert(result, currentInterval)
    table.insert(result, count)
    
    currentInterval = currentInterval + interval
end

return result
`

// GetQPSHistory 获取指定时间范围内的QPS历史数据
// timeUnit: 时间单位，"second" 或 "minute"
// count: 获取多少个时间单位的数据
func (r *RedisSlidingWindow) GetQPSHistory(timeUnit string, count int) ([]map[string]interface{}, error) {
	if !RedisEnabled {
		return nil, fmt.Errorf("redis not enabled")
	}

	ctx := context.Background()
	now := time.Now().UnixMilli()

	var duration, interval int64
	if timeUnit == "second" {
		duration = int64(count) * 1000 // count秒
		interval = 1000                // 1秒间隔
	} else {
		duration = int64(count) * 60 * 1000 // count分钟
		interval = 60 * 1000                // 1分钟间隔
	}

	// 使用全局限流的key
	redisKey := "sliding_window:global"

	// 如果Redis中没有足够的历史数据，我们需要从其他地方获取或生成模拟数据
	// 这里我们先尝试从Redis获取现有数据
	result, err := RDB.Eval(
		ctx,
		getQPSHistoryScript,
		[]string{redisKey},
		now,
		duration,
		interval,
	).Result()

	if err != nil {
		logger.SysError("执行Redis获取QPS历史数据脚本失败: " + err.Error())
		return nil, err
	}

	// 处理结果
	records := result.([]interface{})
	data := make([]map[string]interface{}, 0, len(records)/2)

	// 解析时间戳和计数
	for i := 0; i < len(records); i += 2 {
		ts, _ := strconv.ParseInt(fmt.Sprint(records[i]), 10, 64)
		count, _ := strconv.Atoi(fmt.Sprint(records[i+1]))

		// 格式化时间
		t := time.Unix(0, ts*int64(time.Millisecond))
		var formattedTime string
		if timeUnit == "second" {
			formattedTime = fmt.Sprintf("%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second())
		} else {
			formattedTime = fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute())
		}

		data = append(data, map[string]interface{}{
			"timestamp": ts,
			"time":      formattedTime,
			"value":     count,
		})
	}

	return data, nil
}
