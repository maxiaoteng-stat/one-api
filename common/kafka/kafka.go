package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
)

// TokenUsageData 表示要发送到kafka的token使用数据
type TokenUsageData struct {
	UserId       int    `json:"user_id"`       // 用户id
	Timestamp    int64  `json:"timestamp"`     // 时间戳
	ModelName    string `json:"model_name"`    // 模型名称
	TokenName    string `json:"token_name"`    // token名称
	InputTokens  int    `json:"input_tokens"`  // 输入token数
	OutputTokens int    `json:"output_tokens"` // 输出token数
	InputText    string `json:"input_text"`    // 输入文本内容
	OutputText   string `json:"output_text"`   // 输出文本内容
	RequestId    string `json:"request_id"`    // 请求ID
	ChannelId    int    `json:"channel_id"`    // 渠道ID
}

var producer sarama.SyncProducer

// InitKafkaProducer 初始化Kafka生产者
func InitKafkaProducer() error {
	if !config.KafkaEnabled {
		logger.SysLog("Kafka未启用，跳过初始化")
		return nil
	}

	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Producer.Return.Successes = true
	kafkaConfig.Producer.Retry.Max = 3
	kafkaConfig.Producer.RequiredAcks = sarama.WaitForLocal
	kafkaConfig.Producer.Compression = sarama.CompressionSnappy

	// 设置认证信息（如果需要）
	if config.KafkaUsername != "" && config.KafkaPassword != "" {
		kafkaConfig.Net.SASL.Enable = true
		kafkaConfig.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		kafkaConfig.Net.SASL.User = config.KafkaUsername
		kafkaConfig.Net.SASL.Password = config.KafkaPassword
	}

	var err error
	producer, err = sarama.NewSyncProducer(config.KafkaBrokers, kafkaConfig)
	if err != nil {
		return fmt.Errorf("初始化Kafka生产者失败: %v", err)
	}

	logger.SysLog("Kafka生产者初始化成功")
	return nil
}

// SendTokenUsageToKafka 发送token使用数据到Kafka
func SendTokenUsageToKafka(ctx context.Context, data *TokenUsageData) error {
	if !config.KafkaEnabled || producer == nil {
		return nil // 如果未启用或未初始化，静默返回
	}

	// 设置时间戳
	if data.Timestamp == 0 {
		data.Timestamp = time.Now().Unix()
	}

	// 序列化为JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化token使用数据失败: %v", err)
	}

	// 创建Kafka消息
	message := &sarama.ProducerMessage{
		Topic: config.KafkaTokenUsageTopic,
		Value: sarama.StringEncoder(jsonData),
		Key:   sarama.StringEncoder(fmt.Sprintf("%d_%s", data.UserId, data.TokenName)),
	}

	// 发送消息
	partition, offset, err := producer.SendMessage(message)
	if err != nil {
		logger.Errorf(ctx, "发送token使用数据到Kafka失败: %v", err)
		return err
	}

	logger.Debugf(ctx, "成功发送token使用数据到Kafka (partition: %d, offset: %d)", partition, offset)
	return nil
}

// CloseKafkaProducer 关闭Kafka生产者
func CloseKafkaProducer() error {
	if producer != nil {
		return producer.Close()
	}
	return nil
}
