package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
)

// 定义钉钉消息结构
type DingTalkMessage struct {
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

// 预警记录，用于防止短时间内重复发送
var alertRecords = struct {
	sync.RWMutex
	records map[string]time.Time
}{
	records: make(map[string]time.Time),
}

// 记录预警
func recordAlert(key string) {
	alertRecords.Lock()
	defer alertRecords.Unlock()
	alertRecords.records[key] = time.Now()
}

// 检查预警是否在冷却期
func isAlertInCooldown(key string, cooldownMinutes int) bool {
	alertRecords.RLock()
	defer alertRecords.RUnlock()
	lastTime, exists := alertRecords.records[key]
	if !exists {
		return false
	}

	return time.Since(lastTime) < time.Duration(cooldownMinutes)*time.Minute
}

// SendDingTalkAlert 发送钉钉通知
func SendDingTalkAlert(webhook string, content string) error {
	message := DingTalkMessage{
		MsgType: "text",
		Text: struct {
			Content string `json:"content"`
		}{
			Content: content,
		},
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	resp, err := http.Post(webhook, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("DingTalk responded with error code: %d", resp.StatusCode)
	}

	return nil
}

// CheckGlobalQuota 检查全局配额使用情况并发送预警
func CheckGlobalQuota() {
	// 获取全局限流配置
	globalLimit, err := model.GetGlobalRateLimit()
	if err != nil {
		logger.SysError(fmt.Sprintf("failed to get global rate limit: %s", err.Error()))
		return
	}

	// 如果没有设置钉钉webhook或预警阈值，跳过
	if globalLimit.DingTalkWebhook == "" || globalLimit.WarningThreshold <= 0 || globalLimit.DailyQuota <= 0 {
		return
	}

	// 获取当日全局请求数
	dailyUsage, err := common.GetDailyQuotaUsage("global")
	if err != nil {
		logger.SysError(fmt.Sprintf("failed to get global daily quota usage: %s", err.Error()))
		return
	}

	// 计算使用比例
	usagePercent := float64(dailyUsage) / float64(globalLimit.DailyQuota) * 100

	// 如果使用比例超过预警阈值，发送预警
	if usagePercent >= float64(globalLimit.WarningThreshold) {
		// 检查是否在冷却期（避免频繁发送）
		alertKey := "global_quota_" + strconv.Itoa(globalLimit.WarningThreshold)
		if isAlertInCooldown(alertKey, 60) { // 冷却期1小时
			return
		}

		// 构造预警消息
		message := fmt.Sprintf("【One API Global Quota Alert】\nGlobal daily requests reached %d, %.2f%% of total quota, exceeding warning threshold %d%%\nCurrent time: %s",
			dailyUsage, usagePercent, globalLimit.WarningThreshold, time.Now().Format("2006-01-02 15:04:05"))

		// 发送预警
		err = SendDingTalkAlert(globalLimit.DingTalkWebhook, message)
		if err != nil {
			logger.SysError(fmt.Sprintf("failed to send global quota alert: %s", err.Error()))
			return
		}

		// 记录预警，避免重复发送
		recordAlert(alertKey)
	}
}

// CheckTokenQuotas 检查各个Token的配额使用情况并发送预警
func CheckTokenQuotas() {
	// 获取全局限流配置（用于获取钉钉webhook）
	globalLimit, err := model.GetGlobalRateLimit()
	if err != nil {
		logger.SysError(fmt.Sprintf("failed to get global rate limit: %s", err.Error()))
		return
	}

	// 如果没有设置钉钉webhook，跳过
	if globalLimit.DingTalkWebhook == "" {
		return
	}

	// 获取所有配置了限流的Token
	var tokenLimits []model.TokenRateLimit
	err = model.DB.Where("daily_quota > 0").Find(&tokenLimits).Error
	if err != nil {
		logger.SysError(fmt.Sprintf("failed to get token rate limit list: %s", err.Error()))
		return
	}

	// 检查每个Token的配额使用情况
	for _, tokenLimit := range tokenLimits {
		// 获取Token信息
		token, err := model.GetTokenById(tokenLimit.TokenId)
		if err != nil {
			logger.SysError(fmt.Sprintf("failed to get token info, TokenId=%d: %s", tokenLimit.TokenId, err.Error()))
			continue
		}

		// 获取当日Token请求数
		qpsKey := fmt.Sprintf("token:%d", tokenLimit.TokenId)
		dailyUsage, err := common.GetDailyQuotaUsage(qpsKey)
		if err != nil {
			logger.SysError(fmt.Sprintf("failed to get token daily quota usage, TokenId=%d: %s", tokenLimit.TokenId, err.Error()))
			continue
		}

		// 计算使用比例
		usagePercent := float64(dailyUsage) / float64(tokenLimit.DailyQuota) * 100

		// 如果使用比例超过预警阈值，发送预警
		if usagePercent >= float64(globalLimit.WarningThreshold) {
			// 检查是否在冷却期（避免频繁发送）
			alertKey := fmt.Sprintf("token_quota_%d_%d", tokenLimit.TokenId, globalLimit.WarningThreshold)
			if isAlertInCooldown(alertKey, 60) { // 冷却期1小时
				continue
			}

			// 构造预警消息
			message := fmt.Sprintf("【One API Token Quota Alert】\nToken name: %s\nToken ID: %d\nDaily requests reached %d, %.2f%% of total quota, exceeding warning threshold %d%%\nCurrent time: %s",
				token.Name, tokenLimit.TokenId, dailyUsage, usagePercent, globalLimit.WarningThreshold, time.Now().Format("2006-01-02 15:04:05"))

			// 发送预警
			err = SendDingTalkAlert(globalLimit.DingTalkWebhook, message)
			if err != nil {
				logger.SysError(fmt.Sprintf("failed to send token quota alert, TokenId=%d: %s", tokenLimit.TokenId, err.Error()))
				continue
			}

			// 记录预警，避免重复发送
			recordAlert(alertKey)
		}
	}
}

// CheckHighQPS 检查高QPS情况并发送预警
func CheckHighQPS() {
	// 获取全局限流配置
	globalLimit, err := model.GetGlobalRateLimit()
	if err != nil {
		logger.SysError(fmt.Sprintf("failed to get global rate limit: %s", err.Error()))
		return
	}

	// 如果没有设置钉钉webhook或最大QPS，跳过
	if globalLimit.DingTalkWebhook == "" || globalLimit.MaxQPS <= 0 {
		return
	}

	// 获取当前全局QPS
	qpsKey := "global"
	currentQPS, err := common.IncrAndGetQPS(qpsKey, 60) // 使用滑动窗口
	if err != nil {
		logger.SysError(fmt.Sprintf("failed to get QPS counter: %s", err.Error()))
		return
	}

	// 计算QPS比例
	qpsPercent := float64(currentQPS) / float64(globalLimit.MaxQPS) * 100

	// 如果QPS达到最大QPS的90%以上，发送预警
	if qpsPercent >= 90 {
		// 检查是否在冷却期（避免频繁发送）
		alertKey := "high_qps_90"
		if isAlertInCooldown(alertKey, 10) { // 冷却期10分钟
			return
		}

		// 构造预警消息
		message := fmt.Sprintf("【One API High QPS Alert】\nCurrent QPS is %d, %.2f%% of maximum QPS (%d)\nCurrent time: %s",
			currentQPS, qpsPercent, globalLimit.MaxQPS, time.Now().Format("2006-01-02 15:04:05"))

		// 发送预警
		err = SendDingTalkAlert(globalLimit.DingTalkWebhook, message)
		if err != nil {
			logger.SysError(fmt.Sprintf("failed to send high QPS alert: %s", err.Error()))
			return
		}

		// 记录预警，避免重复发送
		recordAlert(alertKey)
	}
}

// 启动定时检查任务
var alertChecker *time.Ticker
var stopChan chan struct{}

// StartAlertChecker 启动定时检查任务
func StartAlertChecker(checkIntervalSeconds int) {
	if checkIntervalSeconds <= 0 {
		checkIntervalSeconds = 1800 // 默认半个小时检查一次
	}

	stopChan = make(chan struct{})
	alertChecker = time.NewTicker(time.Duration(checkIntervalSeconds) * time.Second)

	go func() {
		for {
			select {
			case <-alertChecker.C:
				// 检查全局配额
				CheckGlobalQuota()
				// 检查Token配额
				CheckTokenQuotas()
				// 检查高QPS情况
				CheckHighQPS()
				//缺少一个token QPS预警，考虑后续增加
			case <-stopChan:
				alertChecker.Stop()
				return
			}
		}
	}()
}

// StopAlertChecker 停止定时检查任务
func StopAlertChecker() {
	if alertChecker != nil {
		close(stopChan)
	}
}
