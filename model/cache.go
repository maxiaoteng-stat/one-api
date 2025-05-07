package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/random"
)

var (
	TokenCacheSeconds         = config.SyncFrequency
	UserId2GroupCacheSeconds  = config.SyncFrequency
	UserId2QuotaCacheSeconds  = config.SyncFrequency
	UserId2StatusCacheSeconds = config.SyncFrequency
	GroupModelsCacheSeconds   = config.SyncFrequency
)

// 负载均衡策略类型
const (
	BalanceStrategyRandom     = "random"     // 随机策略
	BalanceStrategyRoundRobin = "roundrobin" // 轮询策略
)

// 全局配置
var (
	// 默认使用随机策略
	defaultBalanceStrategy = BalanceStrategyRandom

	// 轮询计数器
	roundRobinCounters     = make(map[string]map[string]*atomic.Uint64)
	roundRobinCountersLock sync.RWMutex
)

func CacheGetTokenByKey(key string) (*Token, error) {
	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	var token Token
	if !common.RedisEnabled {
		err := DB.Where(keyCol+" = ?", key).First(&token).Error
		return &token, err
	}
	tokenObjectString, err := common.RedisGet(fmt.Sprintf("token:%s", key))
	if err != nil {
		err := DB.Where(keyCol+" = ?", key).First(&token).Error
		if err != nil {
			return nil, err
		}
		jsonBytes, err := json.Marshal(token)
		if err != nil {
			return nil, err
		}
		err = common.RedisSet(fmt.Sprintf("token:%s", key), string(jsonBytes), time.Duration(TokenCacheSeconds)*time.Second)
		if err != nil {
			logger.SysError("Redis set token error: " + err.Error())
		}
		return &token, nil
	}
	err = json.Unmarshal([]byte(tokenObjectString), &token)
	return &token, err
}

func CacheGetUserGroup(id int) (group string, err error) {
	if !common.RedisEnabled {
		return GetUserGroup(id)
	}
	group, err = common.RedisGet(fmt.Sprintf("user_group:%d", id))
	if err != nil {
		group, err = GetUserGroup(id)
		if err != nil {
			return "", err
		}
		err = common.RedisSet(fmt.Sprintf("user_group:%d", id), group, time.Duration(UserId2GroupCacheSeconds)*time.Second)
		if err != nil {
			logger.SysError("Redis set user group error: " + err.Error())
		}
	}
	return group, err
}

func fetchAndUpdateUserQuota(ctx context.Context, id int) (quota int64, err error) {
	quota, err = GetUserQuota(id)
	if err != nil {
		return 0, err
	}
	err = common.RedisSet(fmt.Sprintf("user_quota:%d", id), fmt.Sprintf("%d", quota), time.Duration(UserId2QuotaCacheSeconds)*time.Second)
	if err != nil {
		logger.Error(ctx, "Redis set user quota error: "+err.Error())
	}
	return
}

func CacheGetUserQuota(ctx context.Context, id int) (quota int64, err error) {
	if !common.RedisEnabled {
		return GetUserQuota(id)
	}
	quotaString, err := common.RedisGet(fmt.Sprintf("user_quota:%d", id))
	if err != nil {
		return fetchAndUpdateUserQuota(ctx, id)
	}
	quota, err = strconv.ParseInt(quotaString, 10, 64)
	if err != nil {
		return 0, nil
	}
	if quota <= config.PreConsumedQuota { // when user's quota is less than pre-consumed quota, we need to fetch from db
		logger.Infof(ctx, "user %d's cached quota is too low: %d, refreshing from db", quota, id)
		return fetchAndUpdateUserQuota(ctx, id)
	}
	return quota, nil
}

func CacheUpdateUserQuota(ctx context.Context, id int) error {
	if !common.RedisEnabled {
		return nil
	}
	quota, err := CacheGetUserQuota(ctx, id)
	if err != nil {
		return err
	}
	err = common.RedisSet(fmt.Sprintf("user_quota:%d", id), fmt.Sprintf("%d", quota), time.Duration(UserId2QuotaCacheSeconds)*time.Second)
	return err
}

func CacheDecreaseUserQuota(id int, quota int64) error {
	if !common.RedisEnabled {
		return nil
	}
	err := common.RedisDecrease(fmt.Sprintf("user_quota:%d", id), int64(quota))
	return err
}

func CacheIsUserEnabled(userId int) (bool, error) {
	if !common.RedisEnabled {
		return IsUserEnabled(userId)
	}
	enabled, err := common.RedisGet(fmt.Sprintf("user_enabled:%d", userId))
	if err == nil {
		return enabled == "1", nil
	}

	userEnabled, err := IsUserEnabled(userId)
	if err != nil {
		return false, err
	}
	enabled = "0"
	if userEnabled {
		enabled = "1"
	}
	err = common.RedisSet(fmt.Sprintf("user_enabled:%d", userId), enabled, time.Duration(UserId2StatusCacheSeconds)*time.Second)
	if err != nil {
		logger.SysError("Redis set user enabled error: " + err.Error())
	}
	return userEnabled, err
}

func CacheGetGroupModels(ctx context.Context, group string) ([]string, error) {
	if !common.RedisEnabled {
		return GetGroupModels(ctx, group)
	}
	modelsStr, err := common.RedisGet(fmt.Sprintf("group_models:%s", group))
	if err == nil {
		return strings.Split(modelsStr, ","), nil
	}
	models, err := GetGroupModels(ctx, group)
	if err != nil {
		return nil, err
	}
	err = common.RedisSet(fmt.Sprintf("group_models:%s", group), strings.Join(models, ","), time.Duration(GroupModelsCacheSeconds)*time.Second)
	if err != nil {
		logger.SysError("Redis set group models error: " + err.Error())
	}
	return models, nil
}

var group2model2channels map[string]map[string][]*Channel
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	newChannelId2channel := make(map[int]*Channel)
	var channels []*Channel
	DB.Where("status = ?", ChannelStatusEnabled).Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]*Channel)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]*Channel)
	}
	for _, channel := range channels {
		groups := strings.Split(channel.Group, ",")
		for _, group := range groups {
			models := strings.Split(channel.Models, ",")
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]*Channel, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel)
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return channels[i].GetPriority() > channels[j].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	channelSyncLock.Unlock()
	logger.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		logger.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func CacheGetRandomSatisfiedChannel(group string, model string, ignoreFirstPriority bool) (*Channel, error) {
	if !config.MemoryCacheEnabled {
		return GetRandomSatisfiedChannel(group, model, ignoreFirstPriority)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	channels := group2model2channels[group][model]
	if len(channels) == 0 {
		return nil, errors.New("channel not found")
	}
	endIdx := len(channels)
	// choose by priority
	firstChannel := channels[0]
	if firstChannel.GetPriority() > 0 {
		for i := range channels {
			if channels[i].GetPriority() != firstChannel.GetPriority() {
				endIdx = i
				break
			}
		}
	}
	idx := rand.Intn(endIdx)
	if ignoreFirstPriority {
		if endIdx < len(channels) { // which means there are more than one priority
			idx = random.RandRange(endIdx, len(channels))
		}
	}
	return channels[idx], nil
}

// SetBalanceStrategy 设置负载均衡策略
func SetBalanceStrategy(strategy string) {
	if strategy == BalanceStrategyRandom || strategy == BalanceStrategyRoundRobin {
		defaultBalanceStrategy = strategy
		logger.SysLogf("Balance strategy set to: %s", strategy)
	} else {
		logger.SysLogf("Invalid balance strategy: %s, using default: %s", strategy, defaultBalanceStrategy)
	}
}

// GetBalanceStrategy 获取当前负载均衡策略
func GetBalanceStrategy() string {
	return defaultBalanceStrategy
}

// 初始化轮询计数器
func initRoundRobinCounter(group, model string) {
	roundRobinCountersLock.Lock()
	defer roundRobinCountersLock.Unlock()

	if _, ok := roundRobinCounters[group]; !ok {
		roundRobinCounters[group] = make(map[string]*atomic.Uint64)
	}

	if _, ok := roundRobinCounters[group][model]; !ok {
		roundRobinCounters[group][model] = &atomic.Uint64{}
	}
}

// 获取并递增计数器
func getAndIncrementCounter(group, model string) uint64 {
	roundRobinCountersLock.RLock()
	counter, ok := roundRobinCounters[group][model]
	roundRobinCountersLock.RUnlock()

	if !ok {
		initRoundRobinCounter(group, model)
		roundRobinCountersLock.RLock()
		counter = roundRobinCounters[group][model]
		roundRobinCountersLock.RUnlock()
	}

	return counter.Add(1) - 1 // 返回递增前的值
}

// 获取指定优先级的渠道列表
func getChannelsByPriority(channels []*Channel) map[int][]*Channel {
	priorityGroups := make(map[int][]*Channel)

	for _, channel := range channels {
		priority := int(channel.GetPriority())
		priorityGroups[priority] = append(priorityGroups[priority], channel)
	}

	return priorityGroups
}

// CacheGetRoundRobinChannel 实现轮询+优先级策略获取渠道
func cacheGetRoundRobinChannel(group string, model string, ignoreFirstPriority bool) (*Channel, error) {
	if !config.MemoryCacheEnabled {
		return nil, errors.New("memory cache is not enabled")
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	channels := group2model2channels[group][model]
	if len(channels) == 0 {
		return nil, errors.New("no available channel found")
	}
	// 按优先级分组
	priorityGroups := getChannelsByPriority(channels)

	// 获取所有优先级，并按从高到低排序
	priorities := make([]int, 0, len(priorityGroups))
	for p := range priorityGroups {
		priorities = append(priorities, p)
	}
	sortPrioritiesDesc(priorities)

	// 如果需要忽略第一优先级
	if ignoreFirstPriority && len(priorities) > 1 {
		priorities = priorities[1:] // 移除最高优先级
	}

	// 基于优先级加权选择
	var selectedPriority int
	if len(priorities) == 0 {
		return nil, errors.New("no suitable priority channels found")
	} else if len(priorities) == 1 {
		selectedPriority = priorities[0]
	} else {
		selectedPriority = weightedPrioritySelection(priorities, priorityGroups)
	}

	// 在选定优先级的渠道中进行轮询
	channelsInPriority := priorityGroups[selectedPriority]
	counter := getAndIncrementCounter(group, model)
	index := int(counter % uint64(len(channelsInPriority)))

	return channelsInPriority[index], nil
}

// 优先级权重选择，使用真实数字比例
func weightedPrioritySelection(priorities []int, priorityGroups map[int][]*Channel) int {
	totalWeight := 0
	weights := make(map[int]int)

	// 优先级数字直接作为权重
	for _, priority := range priorities {
		// 确保权重为正数
		weight := priority
		if weight <= 0 {
			weight = 1 // 处理特殊情况，确保最小权重为1
		}
		weights[priority] = weight
		totalWeight += weight
	}

	// 如果总权重为0（极端情况），则直接返回最高优先级
	if totalWeight <= 0 {
		return priorities[0]
	}

	// 随机生成数
	r := random.RandRange(1, totalWeight+1)

	// 按权重选择
	cumulativeWeight := 0
	for _, priority := range priorities {
		cumulativeWeight += weights[priority]
		if r <= cumulativeWeight {
			return priority
		}
	}

	// 保底返回最高优先级
	return priorities[0]
}

// 按从高到低排序优先级
func sortPrioritiesDesc(priorities []int) {
	for i := 0; i < len(priorities); i++ {
		for j := i + 1; j < len(priorities); j++ {
			if priorities[i] < priorities[j] {
				priorities[i], priorities[j] = priorities[j], priorities[i]
			}
		}
	}
}

// CacheGetSatisfiedChannel 根据配置的负载均衡策略选择合适的渠道
func CacheGetSatisfiedChannel(group string, model string, ignoreFirstPriority bool) (*Channel, error) {
	if defaultBalanceStrategy == BalanceStrategyRoundRobin {
		return cacheGetRoundRobinChannel(group, model, ignoreFirstPriority)
	}
	// 默认使用原有的随机策略
	return CacheGetRandomSatisfiedChannel(group, model, ignoreFirstPriority)
}

// StartRoundRobinCounterCleaner 启动计数器清理器
func StartRoundRobinCounterCleaner() {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			cleanRoundRobinCounters()
		}
	}()

	logger.SysLog("Round robin counter cleaner started")
}

// 清理轮询计数器
func cleanRoundRobinCounters() {
	roundRobinCountersLock.Lock()
	defer roundRobinCountersLock.Unlock()

	// 重置所有计数器
	for group := range roundRobinCounters {
		for model := range roundRobinCounters[group] {
			roundRobinCounters[group][model].Store(0)
		}
	}

	logger.SysLog("Round robin counters have been reset")
}

// InitBalanceStrategies 初始化负载均衡策略
func InitBalanceStrategies() {
	// 从配置或环境变量中读取策略设置
	strategyFromConfig := config.BalanceStrategy
	if strategyFromConfig != "" {
		SetBalanceStrategy(strategyFromConfig)
	}

	// 如果使用轮询策略，启动计数器清理器
	if defaultBalanceStrategy == BalanceStrategyRoundRobin {
		StartRoundRobinCounterCleaner()
	}
}
