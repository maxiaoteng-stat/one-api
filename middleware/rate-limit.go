package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
	"gorm.io/gorm"
)

var timeFormat = "2006-01-02T15:04:05.000Z"

var inMemoryRateLimiter common.InMemoryRateLimiter

func redisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	ctx := context.Background()
	rdb := common.RDB
	key := "rateLimit:" + mark + c.ClientIP()
	listLength, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if listLength < int64(maxRequestNum) {
		rdb.LPush(ctx, key, time.Now().Format(timeFormat))
		rdb.Expire(ctx, key, config.RateLimitKeyExpirationDuration)
	} else {
		oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
		oldTime, err := time.Parse(timeFormat, oldTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		nowTimeStr := time.Now().Format(timeFormat)
		nowTime, err := time.Parse(timeFormat, nowTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		// time.Since will return negative number!
		// See: https://stackoverflow.com/questions/50970900/why-is-time-since-returning-negative-durations-on-windows
		if int64(nowTime.Sub(oldTime).Seconds()) < duration {
			rdb.Expire(ctx, key, config.RateLimitKeyExpirationDuration)
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		} else {
			rdb.LPush(ctx, key, time.Now().Format(timeFormat))
			rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
			rdb.Expire(ctx, key, config.RateLimitKeyExpirationDuration)
		}
	}
}

func memoryRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	key := mark + c.ClientIP()
	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}
}

func rateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	if maxRequestNum == 0 || config.DebugEnabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	if common.RedisEnabled {
		return func(c *gin.Context) {
			redisRateLimiter(c, maxRequestNum, duration, mark)
		}
	} else {
		// It's safe to call multi times.
		inMemoryRateLimiter.Init(config.RateLimitKeyExpirationDuration)
		return func(c *gin.Context) {
			memoryRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
}

func GlobalWebRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.GlobalWebRateLimitNum, config.GlobalWebRateLimitDuration, "GW")
}

func GlobalAPIRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.GlobalApiRateLimitNum, config.GlobalApiRateLimitDuration, "GA")
}

func CriticalRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.CriticalRateLimitNum, config.CriticalRateLimitDuration, "CT")
}

func DownloadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.DownloadRateLimitNum, config.DownloadRateLimitDuration, "DW")
}

func UploadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.UploadRateLimitNum, config.UploadRateLimitDuration, "UP")
}

// TokenRateLimiter Token级别限流中间件
func TokenRateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		// 跳过对非API请求的限流
		if !strings.HasPrefix(c.Request.URL.Path, "/v1/") {
			c.Next()
			return
		}

		if !common.RedisEnabled {
			c.Next()
			return
		}

		// 获取Token ID
		tokenId := c.GetInt(ctxkey.TokenId)
		if tokenId == 0 {
			c.Next() // 没有Token，不做限流
			return
		}

		// 获取Token限流配置
		tokenLimit, err := model.GetTokenRateLimit(tokenId)
		if err != nil {
			// 如果是记录不存在的错误，则跳过限流检查
			if errors.Is(err, gorm.ErrRecordNotFound) {
				logger.Debug(ctx, fmt.Sprintf("Token %d 没有限流配置，跳过限流检查", tokenId))
				c.Next()
				return
			}

			// 其他错误
			logger.Error(ctx, fmt.Sprintf("获取Token限流配置失败: %s", err.Error()))
			c.Next() // 出错时不做限流，继续处理请求
			return
		}

		// 检查是否启用了限流
		if !tokenLimit.Enabled {
			c.Next()
			return
		}

		// 获取配置参数
		maxQPS := tokenLimit.MaxQPS
		dailyQuota := tokenLimit.DailyQuota

		// 如果都没有设置限制，直接通过
		if maxQPS <= 0 && dailyQuota <= 0 {
			c.Next()
			return
		}

		// 如果设置了日配额(dailyQuota > 0)，检查今日使用量
		if dailyQuota > 0 {
			// 从Redis获取今日使用的token量
			dailyUsage, err := model.GetTokenDailyUsage(c.GetString(ctxkey.TokenName))
			if err != nil {
				logger.Error(ctx, fmt.Sprintf("获取Token日配额使用情况失败: %s", err.Error()))
				// 出错时不做限流，继续处理请求
			} else if dailyUsage >= int64(dailyQuota) {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": gin.H{
						"message": "该Token日配额已用完，请明天再试或增加配额",
						"type":    "rate_limit_error",
						"code":    "daily_quota_exceeded",
					},
				})
				c.Abort()
				return
			}
		}

		// 如果设置了QPS限制，执行QPS限流检查
		if maxQPS > 0 {
			qpsKey := fmt.Sprintf("token:%d", tokenId)
			allowed := false

			// 使用滑动窗口检查当前QPS
			if common.RedisEnabled {
				windowSize := int64(1000) // 1秒窗口，单位毫秒
				allowed, _, err = common.RedisSlidingWindowLimiter.CheckAndIncrease(
					qpsKey,
					windowSize,
					maxQPS,
					60, // 过期时间设为60秒
				)

				if err != nil {
					logger.Error(ctx, fmt.Sprintf("检查Token QPS限流失败: %s", err.Error()))
					c.Next() // 出错时不做限流，继续处理请求
					return
				}
			} else {
				// 内存实现
				allowed, _ = common.InMemorySlidingWindowLimiter.CheckAndIncrease(
					qpsKey,
					1000, // 1秒窗口，单位毫秒
					maxQPS,
				)
			}

			// 如果QPS超限，拒绝请求
			if !allowed {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": gin.H{
						"message": "当前Token请求过多，请降低请求频率",
						"type":    "rate_limit_error",
						"code":    "token_limit_exceeded",
					},
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// GlobalRateLimiter 全局限流中间件 - 仅当Token限流未执行时生效
func GlobalRateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// 跳过对非API请求的限流
		if !strings.HasPrefix(c.Request.URL.Path, "/v1/") {
			c.Next()
			return
		}

		if !common.RedisEnabled {
			c.Next()
			return
		}

		// 获取全局限流配置
		globalLimit, err := model.GetGlobalRateLimit()
		if err != nil {
			logger.Error(ctx, fmt.Sprintf("failed to get global rate limit: %s", err.Error()))
			c.Next() // 出错时不做限流，继续处理请求
			return
		}

		// 检查是否启用了全局限流
		if !globalLimit.Enabled {
			c.Next()
			return
		}

		// 如果设置了日配额(dailyQuota > 0)，检查今日全局使用量
		if globalLimit.DailyQuota > 0 {
			// 从Redis获取今日使用的全局配额
			dailyUsage, err := model.GetGlobalDailyUsage()
			if err != nil {
				logger.Error(ctx, fmt.Sprintf("获取全局日配额使用情况失败: %s", err.Error()))
				// 出错时不做限流，继续处理请求
			} else if dailyUsage >= int64(globalLimit.DailyQuota) {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": gin.H{
						"message": "系统今日配额已用完，请明天再试",
						"type":    "rate_limit_error",
						"code":    "daily_quota_exceeded",
					},
				})
				c.Abort()
				return
			}
		}

		// 如果设置了QPS限制，执行QPS限流检查
		if globalLimit.MaxQPS > 0 {
			qpsKey := "global"
			allowed := false

			// 使用滑动窗口检查当前QPS
			if common.RedisEnabled {
				windowSize := int64(1000) // 1秒窗口，单位毫秒
				allowed, _, err = common.RedisSlidingWindowLimiter.CheckAndIncrease(
					qpsKey,
					windowSize,
					globalLimit.MaxQPS,
					60, // 过期时间设为60秒
				)

				if err != nil {
					logger.Error(ctx, fmt.Sprintf("failed to check rate limit: %s", err.Error()))
					c.Next() // 出错时不做限流，继续处理请求
					return
				}
			} else {
				// 内存实现
				allowed, _ = common.InMemorySlidingWindowLimiter.CheckAndIncrease(
					qpsKey,
					1000, // 1秒窗口，单位毫秒
					globalLimit.MaxQPS,
				)
			}

			// 如果请求不被允许或当前QPS超过限制，处理队列或拒绝
			if !allowed {
				// 如果没有队列容量或者请求超过队列容量，直接拒绝
				if globalLimit.QueueCapacity <= 0 {
					logger.Warn(ctx, "超过QPS限制，没有设置队列容量,拒绝请求！")
					c.JSON(http.StatusTooManyRequests, gin.H{
						"error": gin.H{
							"message": "系统当前请求过多，请稍后再试",
							"type":    "rate_limit_error",
							"code":    "too_many_requests",
						},
					})
					c.Abort()
					return
				}

				// 处理队列逻辑
				err = handleRequestQueue(c, qpsKey, globalLimit)
				if err != nil {
					// 队列处理失败，继续处理请求
					logger.Error(ctx, fmt.Sprintf("queue handling failed: %s", err.Error()))
					c.Next()
					return
				}

				// 如果处理结果为nil且没有调用Next()，表示请求已被处理或拒绝
				return
			}
		}

		c.Next()
	}
}

// handleRequestQueue 处理请求队列逻辑
func handleRequestQueue(c *gin.Context, qpsKey string, globalLimit *model.GlobalRateLimit) error {
	// 将检查队列长度和入队操作合并为原子操作
	enqueueScript := `
	local queueLen = redis.call('LLEN', KEYS[1])
	if queueLen >= tonumber(ARGV[2]) then
		return 0  -- 队列已满
	end
	
	redis.call('RPUSH', KEYS[1], ARGV[1])
	redis.call('EXPIRE', KEYS[1], tonumber(ARGV[3]))
	redis.call('SETEX', 'request_timeout:'..ARGV[1], tonumber(ARGV[3]), ARGV[4])
	return 1  -- 成功入队
	`

	// 生成请求ID
	requestId := uuid.New().String()
	ctx := context.Background()
	currentTime := fmt.Sprintf("%d", time.Now().Unix())

	// 执行入队脚本
	result, err := common.RDB.Eval(
		ctx,
		enqueueScript,
		[]string{qpsKey},
		requestId,
		globalLimit.QueueCapacity,
		globalLimit.QueueTimeout,
		currentTime,
	).Int64()

	if err != nil {
		return fmt.Errorf("failed to check queue and enqueue: %s", err.Error())
	}

	// 如果队列已满，拒绝请求
	if result == 0 {
		logger.Warn(c.Request.Context(), "超过QPS限制，队列已满，拒绝请求！")
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": gin.H{
				"message": "系统当前排队请求过多，请稍后再试",
				"type":    "rate_limit_error",
				"code":    "queue_full",
			},
		})
		c.Abort()
		return nil
	}

	// 等待轮到本请求处理 - 使用简化的轮询逻辑
	startTime := time.Now()
	for {
		// 检查是否超时
		if time.Since(startTime).Seconds() > float64(globalLimit.QueueTimeout) {
			// 超时后尝试从队列中移除该请求（非关键操作，失败也不影响）
			common.RDB.LRem(ctx, qpsKey, 0, requestId)

			c.JSON(http.StatusRequestTimeout, gin.H{
				"error": gin.H{
					"message": "请求等待超时，请稍后再试",
					"type":    "rate_limit_error",
					"code":    "queue_timeout",
				},
			})
			c.Abort()
			return nil
		}

		// 获取队列头部（但不移除）
		headId, err := common.RDB.LIndex(ctx, qpsKey, 0).Result()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// 如果是当前请求，原子地移除并处理
		if headId == requestId {
			// 使用LPOP原子地移除队列头
			popResult, err := common.RDB.LPop(ctx, qpsKey).Result()
			if err != nil || popResult != requestId {
				// 可能在我们检查和移除之间，队列头发生了变化
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// 成功移除，继续处理请求
			c.Next()
			return nil
		}

		// 不是当前请求，检查头部请求是否超时
		timeoutExists, _ := common.RDB.Exists(ctx, "request_timeout:"+headId).Result()
		if timeoutExists == 0 {
			// 头部请求已超时，尝试移除
			common.RDB.LPop(ctx, qpsKey)
			continue
		}

		// 等待一段时间再重试
		time.Sleep(100 * time.Millisecond)
	}
}
