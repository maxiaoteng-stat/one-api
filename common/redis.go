package common

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/songquanpeng/one-api/common/logger"
)

var RDB redis.Cmdable
var RedisEnabled = true

// InitRedisClient This function is called after init()
func InitRedisClient() (err error) {
	if os.Getenv("REDIS_CONN_STRING") == "" {
		RedisEnabled = false
		logger.SysLog("REDIS_CONN_STRING not set, Redis is not enabled")
		return nil
	}
	if os.Getenv("SYNC_FREQUENCY") == "" {
		RedisEnabled = false
		logger.SysLog("SYNC_FREQUENCY not set, Redis is disabled")
		return nil
	}
	redisConnString := os.Getenv("REDIS_CONN_STRING")
	redisMode := strings.ToLower(os.Getenv("REDIS_MODE"))

	// 根据不同模式连接Redis
	switch redisMode {
	case "cluster":
		// 集群模式
		logger.SysLog("Redis cluster mode enabled")
		RDB = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    strings.Split(redisConnString, ","),
			Password: os.Getenv("REDIS_PASSWORD"),
		})
	case "sentinel":
		// 哨兵模式
		logger.SysLog("Redis sentinel mode enabled")
		if os.Getenv("REDIS_MASTER_NAME") == "" {
			logger.FatalLog("REDIS_MASTER_NAME not set, required for sentinel mode")
		}
		RDB = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    os.Getenv("REDIS_MASTER_NAME"),
			SentinelAddrs: strings.Split(redisConnString, ","),
			Password:      os.Getenv("REDIS_PASSWORD"),
			DB:            0,
		})
	default:
		// 兼容以前的代码逻辑
		if os.Getenv("REDIS_MASTER_NAME") != "" {
			// 保持向后兼容，使用哨兵模式
			logger.SysLog("Redis sentinel mode enabled (legacy config)")
			RDB = redis.NewUniversalClient(&redis.UniversalOptions{
				Addrs:      strings.Split(redisConnString, ","),
				Password:   os.Getenv("REDIS_PASSWORD"),
				MasterName: os.Getenv("REDIS_MASTER_NAME"),
			})
		} else {
			// 单实例模式
			logger.SysLog("Redis single instance mode enabled")
			opt, err := redis.ParseURL(redisConnString)
			if err != nil {
				logger.FatalLog("failed to parse Redis connection string: " + err.Error())
			}
			RDB = redis.NewClient(opt)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	_, err = RDB.Ping(ctx).Result()
	if err != nil {
		logger.FatalLog("Redis ping test failed: " + err.Error())
	}
	return err
}

func ParseRedisOption() *redis.Options {
	opt, err := redis.ParseURL(os.Getenv("REDIS_CONN_STRING"))
	if err != nil {
		logger.FatalLog("failed to parse Redis connection string: " + err.Error())
	}
	return opt
}

func RedisSet(key string, value string, expiration time.Duration) error {
	ctx := context.Background()
	return RDB.Set(ctx, key, value, expiration).Err()
}

func RedisGet(key string) (string, error) {
	ctx := context.Background()
	return RDB.Get(ctx, key).Result()
}

func RedisDel(key string) error {
	ctx := context.Background()
	return RDB.Del(ctx, key).Err()
}

func RedisDecrease(key string, value int64) error {
	ctx := context.Background()
	return RDB.DecrBy(ctx, key, value).Err()
}
