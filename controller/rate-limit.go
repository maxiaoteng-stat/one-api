package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/model"
)

// GetGlobalRateLimit 获取全局限流设置
func GetGlobalRateLimit(c *gin.Context) {
	rateLimit, err := model.GetGlobalRateLimit()
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
		"data":    rateLimit,
	})
}

// UpdateGlobalRateLimit 更新全局限流设置
func UpdateGlobalRateLimit(c *gin.Context) {
	var rateLimit model.GlobalRateLimit
	err := c.ShouldBindJSON(&rateLimit)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数解析失败: " + err.Error(),
		})
		return
	}

	// 字段验证
	if rateLimit.MaxQPS < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "最大QPS不能为负数",
		})
		return
	}

	if rateLimit.QueueCapacity < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "队列容量不能为负数",
		})
		return
	}

	if rateLimit.QueueTimeout < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "队列超时时间不能为负数",
		})
		return
	}

	if rateLimit.DailyQuota < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "单日配额不能为负数",
		})
		return
	}

	if rateLimit.WarningThreshold < 0 || rateLimit.WarningThreshold > 100 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "预警阈值必须在0-100之间",
		})
		return
	}

	err = model.UpdateGlobalRateLimit(&rateLimit)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "更新失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新成功",
	})
}

// GetTokenRateLimit 获取Token限流设置
func GetTokenRateLimit(c *gin.Context) {
	tokenIdStr := c.Param("id")
	tokenId, err := strconv.Atoi(tokenIdStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的Token ID",
		})
		return
	}

	// 检查Token是否存在
	token, err := model.GetTokenById(tokenId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Token不存在: " + err.Error(),
		})
		return
	}

	rateLimit, err := model.GetTokenRateLimit(tokenId)
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
		"data": gin.H{
			"rate_limit": rateLimit,
			"token_name": token.Name,
		},
	})
}

// UpdateTokenRateLimit 更新Token限流设置
func UpdateTokenRateLimit(c *gin.Context) {
	tokenIdStr := c.Param("id")
	tokenId, err := strconv.Atoi(tokenIdStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的Token ID",
		})
		return
	}

	// 检查Token是否存在
	_, err = model.GetTokenById(tokenId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Token不存在: " + err.Error(),
		})
		return
	}

	var rateLimit model.TokenRateLimit
	err = c.ShouldBindJSON(&rateLimit)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数解析失败: " + err.Error(),
		})
		return
	}

	// 字段验证
	if rateLimit.MaxQPS < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "最大QPS不能为负数",
		})
		return
	}

	if rateLimit.DailyQuota < 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "单日配额不能为负数",
		})
		return
	}

	// 确保TokenId匹配
	rateLimit.TokenId = tokenId

	err = model.UpdateTokenRateLimit(&rateLimit)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "更新失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新成功",
	})
}
