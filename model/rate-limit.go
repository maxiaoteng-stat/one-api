package model

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"gorm.io/gorm"
)

// GlobalRateLimit 全局限流配置
type GlobalRateLimit struct {
	ID               int    `json:"id" gorm:"primaryKey"`
	Enabled          bool   `json:"enabled" gorm:"default:false"` // 是否启用全局限流
	MaxQPS           int    `json:"max_qps"`                      // 全局最大QPS
	QueueCapacity    int    `json:"queue_capacity"`               // 队列容量
	QueueTimeout     int    `json:"queue_timeout"`                // 队列超时时间(秒)
	DailyQuota       int    `json:"daily_quota"`                  // 单日配额
	WarningThreshold int    `json:"warning_threshold"`            // 预警阈值(百分比)
	DingTalkWebhook  string `json:"dingtalk_webhook"`             // 钉钉通知Webhook
	UpdatedAt        int64  `json:"updated_at" gorm:"bigint"`     // 更新时间（毫秒时间戳）
}

// TokenRateLimit Token级别限流配置
type TokenRateLimit struct {
	ID         int   `json:"id" gorm:"primaryKey"`
	TokenId    int   `json:"token_id" gorm:"index"`        // 关联的Token ID
	Enabled    bool  `json:"enabled" gorm:"default:false"` // 是否启用Token限流
	MaxQPS     int   `json:"max_qps"`                      // Token最大QPS
	DailyQuota int   `json:"daily_quota"`                  // Token单日配额
	UpdatedAt  int64 `json:"updated_at" gorm:"bigint"`     // 更新时间（毫秒时间戳）
}

// 缓存相关常量
const (
	GlobalRateLimitCacheKey = "global_rate_limit"
	TokenRateLimitKeyPrefix = "token_rate_limit:"
	NOT_EXIST               = "NOT_EXIST"
	// RateLimitCacheTTL       = 600 // 缓存10分钟
)

// 获取全局限流配置
func GetGlobalRateLimit() (*GlobalRateLimit, error) {
	var rateLimit GlobalRateLimit

	// 先尝试从缓存获取
	if common.RedisEnabled {
		cacheData, err := common.RedisGet(GlobalRateLimitCacheKey)
		if err == nil {
			err = json.Unmarshal([]byte(cacheData), &rateLimit)
			if err == nil {
				return &rateLimit, nil
			}
		}
	}

	// 从数据库获取
	if err := DB.First(&rateLimit).Error; err != nil {
		// 如果不存在，直接返回错误
		// 注意：在初始化阶段，会在InitRateLimitCache中创建默认配置
		return nil, err
	}

	// 存入缓存
	if common.RedisEnabled {
		jsonData, err := json.Marshal(rateLimit)
		if err != nil {
			logger.SysError("error marshaling global rate limit: " + err.Error())
		} else {
			err = common.RedisSet(GlobalRateLimitCacheKey, string(jsonData), 0) // 使用0表示永不过期
			if err != nil {
				logger.SysError("error caching global rate limit: " + err.Error())
			}
		}
	}

	return &rateLimit, nil
}

// 更新全局限流配置
func UpdateGlobalRateLimit(rateLimit *GlobalRateLimit) error {
	// 设置更新时间
	rateLimit.UpdatedAt = helper.GetTimestamp()

	// 更新数据库
	err := DB.Save(rateLimit).Error
	if err != nil {
		return err
	}

	// 更新缓存
	if common.RedisEnabled {
		jsonData, err := json.Marshal(rateLimit)
		if err != nil {
			logger.SysError("error marshaling global rate limit: " + err.Error())
		} else {
			err = common.RedisSet(GlobalRateLimitCacheKey, string(jsonData), 0) // 不设置过期时间
			if err != nil {
				logger.SysError("error updating global rate limit cache: " + err.Error())
			}
		}
	}

	return nil
}

// 获取Token限流配置
func GetTokenRateLimit(tokenId int) (*TokenRateLimit, error) {
	var rateLimit TokenRateLimit
	cacheKey := TokenRateLimitKeyPrefix + string(rune(tokenId))

	// 先尝试从缓存获取
	if common.RedisEnabled {
		cacheData, err := common.RedisGet(cacheKey)
		if err == nil {
			// 检查是否是特殊标记值表示"记录不存在"
			if cacheData == NOT_EXIST {
				return nil, gorm.ErrRecordNotFound
			}

			// 正常数据，解析JSON
			err = json.Unmarshal([]byte(cacheData), &rateLimit)
			if err == nil {
				return &rateLimit, nil
			}
		}
	}

	// 从数据库获取
	if err := DB.Where("token_id = ?", tokenId).First(&rateLimit).Error; err != nil {
		// 如果不存在，缓存这个"不存在"的状态
		if errors.Is(err, gorm.ErrRecordNotFound) && common.RedisEnabled {
			// 使用特殊值标记"记录不存在"
			cacheErr := common.RedisSet(cacheKey, NOT_EXIST, 0)
			if cacheErr != nil {
				logger.SysError("error caching token rate limit not-exist state: " + cacheErr.Error())
			}
			return nil, gorm.ErrRecordNotFound
		} else {
			return nil, err
		}
	}

	// 记录存在，存入缓存
	if common.RedisEnabled {
		jsonData, err := json.Marshal(rateLimit)
		if err != nil {
			logger.SysError("error marshaling token rate limit: " + err.Error())
		} else {
			err = common.RedisSet(cacheKey, string(jsonData), 0)
			if err != nil {
				logger.SysError("error caching token rate limit: " + err.Error())
			}
		}
	}

	return &rateLimit, nil
}

// 更新Token限流配置
func UpdateTokenRateLimit(rateLimit *TokenRateLimit) error {
	// 设置更新时间
	rateLimit.UpdatedAt = helper.GetTimestamp()

	// 先检查记录是否存在
	var count int64
	err := DB.Model(&TokenRateLimit{}).Where("token_id = ?", rateLimit.TokenId).Count(&count).Error
	if err != nil {
		return err
	}

	// 根据记录是否存在选择更新或创建
	if count > 0 {
		// 记录存在，直接更新
		err = DB.Model(&TokenRateLimit{}).
			Where("token_id = ?", rateLimit.TokenId).
			Updates(map[string]interface{}{
				"enabled":     rateLimit.Enabled,
				"max_qps":     rateLimit.MaxQPS,
				"daily_quota": rateLimit.DailyQuota,
				"updated_at":  rateLimit.UpdatedAt,
			}).Error
	} else {
		// 记录不存在，创建新记录
		err = DB.Create(&TokenRateLimit{
			TokenId:    rateLimit.TokenId,
			Enabled:    rateLimit.Enabled,
			MaxQPS:     rateLimit.MaxQPS,
			DailyQuota: rateLimit.DailyQuota,
			UpdatedAt:  rateLimit.UpdatedAt,
		}).Error
	}

	if err != nil {
		return err
	}

	// 更新缓存
	if common.RedisEnabled {
		cacheKey := TokenRateLimitKeyPrefix + string(rune(rateLimit.TokenId))
		// 删除可能存在的"NOT_EXIST"缓存条目
		_ = common.RedisDel(cacheKey)

		// 缓存新的数据
		jsonData, err := json.Marshal(rateLimit)
		if err != nil {
			logger.SysError("error marshaling token rate limit: " + err.Error())
		} else {
			err = common.RedisSet(cacheKey, string(jsonData), 0)
			if err != nil {
				logger.SysError("error updating token rate limit cache: " + err.Error())
			}
		}
	}

	return nil
}

// InitRateLimitCache 在项目启动时加载所有的TokenRateLimit和GlobalRateLimit，不设置过期时间
func InitRateLimitCache() {
	if !common.RedisEnabled {
		logger.SysLog("Redis未启用，跳过加载限流配置")
		return
	}

	logger.SysLog("开始加载限流配置到Redis")

	// 使用First方法查询
	var globalRateLimit GlobalRateLimit
	if err := DB.First(&globalRateLimit).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 没有设置全局限流，创建默认配置
			globalRateLimit = GlobalRateLimit{
				Enabled:          false, // 默认不启用
				MaxQPS:           100,
				QueueCapacity:    200,
				QueueTimeout:     10,
				DailyQuota:       100000000,
				WarningThreshold: 80,
				UpdatedAt:        helper.GetTimestamp(),
			}

			// 保存到数据库
			if err := DB.Create(&globalRateLimit).Error; err != nil {
				logger.SysError("创建默认全局限流配置失败: " + err.Error())
			} else {
				logger.SysLog("已创建默认全局限流配置")

				// 缓存到Redis
				if common.RedisEnabled {
					jsonData, err := json.Marshal(globalRateLimit)
					if err != nil {
						logger.SysError("序列化全局限流配置失败: " + err.Error())
					} else {
						err = common.RedisSet(GlobalRateLimitCacheKey, string(jsonData), 0)
						if err != nil {
							logger.SysError("缓存全局限流配置到Redis失败: " + err.Error())
						} else {
							logger.SysLog("全局限流配置已加载到Redis")
						}
					}
				}
			}
		} else {
			// 其他查询错误
			logger.SysError("查询全局限流配置出错: " + err.Error())
		}
	} else {
		// 找到记录，缓存到Redis
		if common.RedisEnabled {
			jsonData, err := json.Marshal(globalRateLimit)
			if err != nil {
				logger.SysError("序列化全局限流配置失败: " + err.Error())
			} else {
				err = common.RedisSet(GlobalRateLimitCacheKey, string(jsonData), 0)
				if err != nil {
					logger.SysError("缓存全局限流配置到Redis失败: " + err.Error())
				} else {
					logger.SysLog("全局限流配置已加载到Redis")
				}
			}
		}
	}

	// 加载所有Token限流配置
	var tokenRateLimits []*TokenRateLimit
	if err := DB.Find(&tokenRateLimits).Error; err != nil {
		logger.SysError("加载Token限流配置失败: " + err.Error())
		return
	}

	// 获取所有token
	tokens, err := GetAllTokensMap()
	if err != nil {
		logger.SysError("加载Token列表失败: " + err.Error())
		return
	}

	logger.SysLogf("找到 %d 个Token限流配置", len(tokenRateLimits))

	// 将所有Token限流配置持久化到Redis（不设置过期时间）
	for _, rateLimit := range tokenRateLimits {
		// 只缓存存在的Token的限流配置
		if _, exists := tokens[rateLimit.TokenId]; !exists {
			continue
		}

		jsonData, err := json.Marshal(rateLimit)
		if err != nil {
			logger.SysError(fmt.Sprintf("序列化Token %d 限流配置失败: %s", rateLimit.TokenId, err.Error()))
			continue
		}

		cacheKey := TokenRateLimitKeyPrefix + string(rune(rateLimit.TokenId))
		err = common.RedisSet(cacheKey, string(jsonData), 0) // 0表示不过期
		if err != nil {
			logger.SysError(fmt.Sprintf("缓存Token %d 限流配置到Redis失败: %s", rateLimit.TokenId, err.Error()))
			continue
		}

		logger.SysLog(fmt.Sprintf("Token %d 限流配置已加载到Redis", rateLimit.TokenId))
	}

	logger.SysLog("限流配置加载完成")
}
