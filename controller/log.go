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

	usageData, err := model.GetUserTokenModelUsage(userIdInt, tokenName, startTimestampInt, endTimestampInt, excludeModels)
	logger.Info(c.Request.Context(), fmt.Sprintf("Usage data for user %s with token %s: %v", userId, tokenName, usageData))

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

	// 获取统计数据
	stats, err := model.GetAllUserTokenStats(startTimestamp, endTimestamp, excludeModels)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取统计数据失败: " + err.Error(),
		})
		return
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

	// 获取用户的统计数据
	stats, err := model.GetUserTokenStats(userIdInt, tokenName, startTimestamp, endTimestamp, excludeModels)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取用户统计数据失败: " + err.Error(),
		})
		return
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

	stats, err := model.GetTokenUsageByName(startTime, endTime, userIdInt, tokenName, excludeModels)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// GetDailyUsageStats 获取特定token在一段时间内按天统计的使用次数
func GetDailyUsageStats(c *gin.Context) {
	var requestData struct {
		Key       string `json:"key"`
		StartTime int64  `json:"start_time"`
		EndTime   int64  `json:"end_time"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 400,
			"msg":  "无效的请求参数",
			"data": nil,
		})
		return
	}

	// 验证并提取token
	if requestData.Key == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": 400,
			"msg":  "缺少token参数",
			"data": nil,
		})
		return
	}

	// 去除sk-前缀
	key := strings.TrimPrefix(requestData.Key, "sk-")

	// 验证时间范围
	if requestData.StartTime == 0 || requestData.EndTime == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code": 400,
			"msg":  "无效的时间范围",
			"data": nil,
		})
		return
	}

	usageData, err := model.GetDailyUsageStats(key, requestData.StartTime, requestData.EndTime)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 500,
			"msg":  "获取使用统计失败: " + err.Error(),
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

// GetTotalTokenUsageStats 获取token的总使用次数
func GetTotalUsageStats(c *gin.Context) {
	key := c.Query("key")

	if key == "" {
		c.JSON(http.StatusOK, gin.H{
			"code": 400,
			"msg":  "缺少token参数",
			"data": nil,
		})
		return
	}

	// 去除sk-前缀
	accessToken := strings.TrimPrefix(key, "sk-")

	totalUsage, err := model.GetTotalUsage(accessToken)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 500,
			"msg":  "获取使用统计失败: " + err.Error(),
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
