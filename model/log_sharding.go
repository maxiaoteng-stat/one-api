package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/logger"
)

var (
	shardingMutex      sync.Mutex
	existingTableCache sync.Map // 缓存已存在的表名，key: tableName, value: bool
	tableCacheInited   bool
)

// InitLogSharding 初始化日志分表
func InitLogSharding() error {
	if !common.UsingMySQL {
		logger.SysLog("非MySQL数据库，跳过分表初始化")
		return nil
	}

	logger.SysLog("开始初始化日志分表...")

	// 创建当前和未来的分表
	now := time.Now()
	years := []int{now.Year(), now.Year() + 1}
	halves := []string{"h1", "h2"}

	for _, year := range years {
		for _, half := range halves {
			tableName := fmt.Sprintf("logs_%d%s", year, half)
			if err := createTableIfNotExists(tableName); err != nil {
				logger.SysError(fmt.Sprintf("创建表 %s 失败: %s", tableName, err.Error()))
			} else {
				logger.SysLog(fmt.Sprintf("表 %s 已就绪", tableName))
			}
		}
	}

	// 初始化表缓存
	initTableCache()

	logger.SysLog("日志分表初始化完成")
	return nil
}

// initTableCache 初始化表缓存（从数据库加载一次）
func initTableCache() {
	shardingMutex.Lock()
	defer shardingMutex.Unlock()

	var existingTables []string
	err := LOG_DB.Raw(`
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = DATABASE() 
		AND table_name LIKE 'logs_%'
	`).Scan(&existingTables).Error

	if err != nil {
		logger.SysError(fmt.Sprintf("初始化表缓存失败: %s", err.Error()))
		return
	}

	// 清空并重新填充缓存
	existingTableCache = sync.Map{}
	for _, table := range existingTables {
		existingTableCache.Store(table, true)
	}

	tableCacheInited = true
	logger.SysLog(fmt.Sprintf("表缓存初始化完成，共 %d 个表", len(existingTables)))
}

// addTableToCache 添加表到缓存
func addTableToCache(tableName string) {
	existingTableCache.Store(tableName, true)
}

// IsTableInCache 检查表是否在缓存中
func IsTableInCache(tableName string) bool {
	if !tableCacheInited {
		return false
	}
	_, exists := existingTableCache.Load(tableName)
	return exists
}

// createTableIfNotExists 如果表不存在则创建
func createTableIfNotExists(tableName string) error {
	shardingMutex.Lock()
	defer shardingMutex.Unlock()

	// 先检查缓存
	if IsTableInCache(tableName) {
		return nil
	}

	// 检查表是否存在
	var count int64
	err := LOG_DB.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", tableName).Scan(&count).Error
	if err != nil {
		return err
	}

	if count > 0 {
		// 表存在，添加到缓存
		addTableToCache(tableName)
		return nil
	}

	// 创建新表（复制 logs 表结构）
	createSQL := fmt.Sprintf("CREATE TABLE %s LIKE logs", tableName)
	if err := LOG_DB.Exec(createSQL).Error; err != nil {
		return err
	}

	// 创建成功后添加到缓存
	addTableToCache(tableName)
	return nil
}

// StartTableMaintenanceTask 启动定时维护任务（每月检查一次，确保下个半年的表已创建）
func StartTableMaintenanceTask() {
	if !common.UsingMySQL {
		return
	}

	go func() {
		ticker := time.NewTicker(30 * 24 * time.Hour) // 每月检查一次
		defer ticker.Stop()

		for range ticker.C {
			maintainTables()
		}
	}()

	logger.SysLog("日志分表维护任务已启动")
}

// maintainTables 维护分表（确保未来半年的表已创建）
func maintainTables() {
	logger.SysLog("开始检查分表...")

	now := time.Now()
	nextHalfYear := now.AddDate(0, 6, 0)

	// 确保下个半年的表存在
	tableName := GetTableNameByDate(nextHalfYear)
	if err := createTableIfNotExists(tableName); err != nil {
		logger.SysError(fmt.Sprintf("创建表 %s 失败: %s", tableName, err.Error()))
	} else {
		logger.SysLog(fmt.Sprintf("表 %s 已就绪", tableName))
	}

	// 刷新表缓存
	RefreshTableCache()
}

// RefreshTableCache 刷新表缓存（可手动调用或定期调用）
func RefreshTableCache() {
	logger.SysLog("刷新表缓存...")
	initTableCache()
}
