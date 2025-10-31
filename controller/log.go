package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
)

func GetAllLogs(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	logs, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, p*config.ItemsPerPage, config.ItemsPerPage, channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
	return
}

func GetUserLogs(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	userId := c.GetInt(ctxkey.Id)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	logs, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, p*config.ItemsPerPage, config.ItemsPerPage)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
	return
}

func SearchAllLogs(c *gin.Context) {
	keyword := c.Query("keyword")
	logs, err := model.SearchAllLogs(keyword)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
	return
}

func SearchUserLogs(c *gin.Context) {
	keyword := c.Query("keyword")
	userId := c.GetInt(ctxkey.Id)
	logs, err := model.SearchUserLogs(userId, keyword)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
	return
}

func GetLogsStat(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	excludeModels := c.Query("excludeModels")
	channel, _ := strconv.Atoi(c.Query("channel"))
	quotaNum := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, excludeModels)
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": quotaNum,
			//"token": tokenNum,
		},
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	username := c.GetString(ctxkey.Username)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	excludeModels := c.Query("excludeModels")
	quotaNum := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, excludeModels)
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": quotaNum,
			//"token": tokenNum,
		},
	})
	return
}

func DeleteHistoryLogs(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "target timestamp is required",
		})
		return
	}
	count, err := model.DeleteOldLog(targetTimestamp)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
	return
}

func GetUserTokenModelUsage(c *gin.Context) {
	logger.Info(c.Request.Context(), "GetUserTokenModelUsage method start")
	userId := c.Query("userId")
	tokenName := c.Query("tokenName")
	startTimestamp := c.Query("startTimestamp")
	endTimestamp := c.Query("endTimestamp")
	excludeModels := c.Query("excludeModels")

	// 解析参数
	var userIdInt int
	var startTimestampInt, endTimestampInt int64
	fmt.Sscanf(userId, "%d", &userIdInt)
	fmt.Sscanf(startTimestamp, "%d", &startTimestampInt)
	fmt.Sscanf(endTimestamp, "%d", &endTimestampInt)

	usageData, err := model.GetUserTokenModelUsageWithCache(userIdInt, tokenName, startTimestampInt, endTimestampInt, excludeModels)
	logger.Info(c.Request.Context(), fmt.Sprintf("Usage data for user %s with token %s: %d records", userId, tokenName, len(usageData)))

	if err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"data":    usageData,
	})
}

// GetAllUserStatsHandler 获取所有用户的Token使用统计
func GetAllUserStatsHandler(c *gin.Context) {
	// 获取查询参数
	startTimestampStr := c.Query("startTimestamp")
	endTimestampStr := c.Query("endTimestamp")
	excludeModels := c.Query("excludeModels")

	// 转换时间戳参数
	startTimestamp, err := strconv.ParseInt(startTimestampStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的开始时间戳",
		})
		return
	}

	endTimestamp, err := strconv.ParseInt(endTimestampStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的结束时间戳",
		})
		return
	}

	// 尝试使用Redis缓存+MySQL混合查询
	stats, err := model.GetAllUserTokenStatsWithCache(startTimestamp, endTimestamp, excludeModels)
	if err != nil {
		// 如果混合查询失败，降级到纯MySQL查询
		logger.SysError(fmt.Sprintf("Token统计混合查询失败，降级到MySQL: %s", err.Error()))
		stats, err = model.GetAllUserTokenStats(startTimestamp, endTimestamp, excludeModels)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "获取统计数据失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}

// GetUserTokenStatsHandler 获取特定用户的Token使用统计
func GetUserTokenStatsHandler(c *gin.Context) {
	// 获取查询参数
	userId := c.Query("userId")
	tokenName := c.Query("tokenName")
	startTimestampStr := c.Query("startTimestamp")
	endTimestampStr := c.Query("endTimestamp")
	excludeModels := c.Query("excludeModels")

	// 转换用户ID
	userIdInt, err := strconv.Atoi(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的用户ID",
		})
		return
	}

	// 转换时间戳参数
	startTimestamp, err := strconv.ParseInt(startTimestampStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的开始时间戳",
		})
		return
	}

	endTimestamp, err := strconv.ParseInt(endTimestampStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的结束时间戳",
		})
		return
	}

	// 尝试使用Redis缓存+MySQL混合查询
	stats, err := model.GetUserTokenStatsWithCache(userIdInt, tokenName, startTimestamp, endTimestamp, excludeModels)
	if err != nil {
		// 如果混合查询失败，降级到纯MySQL查询
		logger.SysError(fmt.Sprintf("用户Token统计混合查询失败，降级到MySQL: %s", err.Error()))
		stats, err = model.GetUserTokenStats(userIdInt, tokenName, startTimestamp, endTimestamp, excludeModels)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "获取用户统计数据失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}

func GetTokenUsageByNameHandler(c *gin.Context) {
	startTimestamp := c.Query("startTimestamp")
	endTimestamp := c.Query("endTimestamp")
	userId := c.Query("userId")
	tokenName := c.Query("tokenName")
	excludeModels := c.Query("excludeModels")

	startTime, _ := strconv.ParseInt(startTimestamp, 10, 64)
	endTime, _ := strconv.ParseInt(endTimestamp, 10, 64)

	var userIdInt int
	if userId != "" {
		userIdInt, _ = strconv.Atoi(userId)
	}

	// 尝试使用Redis缓存+MySQL混合查询
	stats, err := model.GetTokenUsageByNameWithCache(startTime, endTime, userIdInt, tokenName, excludeModels)
	if err != nil {
		// 如果混合查询失败，降级到纯MySQL查询
		logger.SysError(fmt.Sprintf("Token使用查询混合查询失败，降级到MySQL: %s", err.Error()))
		stats, err = model.GetTokenUsageByName(startTime, endTime, userIdInt, tokenName, excludeModels)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// GetDailyUsageStats 获取多个token在一段时间内按天统计的使用次数（合并结果）
func GetDailyUsageStats(c *gin.Context) {
	var requestData struct {
		Keys      []string `json:"keys"` // 改为支持多个key
		Key       string   `json:"key"`  // 保持向后兼容
		StartTime int64    `json:"start_time"`
		EndTime   int64    `json:"end_time"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 400,
			"msg":  "无效的请求参数",
			"data": nil,
		})
		return
	}

	// 处理keys参数，支持向后兼容
	var keys []string
	if len(requestData.Keys) > 0 {
		keys = requestData.Keys
	} else if requestData.Key != "" {
		keys = []string{requestData.Key}
	} else {
		c.JSON(http.StatusOK, gin.H{
			"code": 400,
			"msg":  "缺少token参数",
			"data": nil,
		})
		return
	}

	// 去除sk-前缀
	processedKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		processedKeys = append(processedKeys, strings.TrimPrefix(key, "sk-"))
	}

	// 验证时间范围
	if requestData.StartTime == 0 || requestData.EndTime == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code": 400,
			"msg":  "无效的时间范围",
			"data": nil,
		})
		return
	}

	// 获取合并后的使用统计
	usageData, hasErrors := model.GetCombinedDailyUsageStats(processedKeys, requestData.StartTime, requestData.EndTime)

	// 如果有无效token，返回400错误
	if hasErrors {
		c.JSON(http.StatusOK, gin.H{
			"code": 400,
			"msg":  "存在无效的token",
			"data": nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "成功",
		"data": usageData,
	})
}

// GetTotalUsageStats 获取多个token的总使用次数（合并结果）
func GetTotalUsageStats(c *gin.Context) {
	keysParam := c.Query("keys")
	keyParam := c.Query("key") // 保持向后兼容

	var keys []string
	if keysParam != "" {
		// 解析keys参数（用逗号分隔）
		keys = strings.Split(keysParam, ",")
	} else if keyParam != "" {
		keys = []string{keyParam}
	} else {
		c.JSON(http.StatusOK, gin.H{
			"code": 400,
			"msg":  "缺少token参数",
			"data": nil,
		})
		return
	}

	// 去除sk-前缀并清理空白字符
	processedKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey != "" {
			processedKeys = append(processedKeys, strings.TrimPrefix(trimmedKey, "sk-"))
		}
	}

	if len(processedKeys) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code": 400,
			"msg":  "没有有效的token参数",
			"data": nil,
		})
		return
	}

	// 获取合并后的总使用量
	totalUsage, hasErrors := model.GetCombinedTotalUsage(processedKeys)

	// 如果有无效token，返回400错误
	if hasErrors {
		c.JSON(http.StatusOK, gin.H{
			"code": 400,
			"msg":  "存在无效的token",
			"data": nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "成功",
		"data": totalUsage,
	})
}
