package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/relay/constant/role"

	"github.com/gin-gonic/gin"

	"encoding/json"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/kafka"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	billingratio "github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/controller/validator"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func getAndValidateTextRequest(c *gin.Context, relayMode int) (*relaymodel.GeneralOpenAIRequest, error) {
	textRequest := &relaymodel.GeneralOpenAIRequest{}
	err := common.UnmarshalBodyReusable(c, textRequest)
	if err != nil {
		return nil, err
	}
	if relayMode == relaymode.Moderations && textRequest.Model == "" {
		textRequest.Model = "text-moderation-latest"
	}
	if relayMode == relaymode.Embeddings && textRequest.Model == "" {
		textRequest.Model = c.Param("model")
	}
	err = validator.ValidateTextRequest(textRequest, relayMode)
	if err != nil {
		return nil, err
	}
	return textRequest, nil
}

func getPromptTokens(textRequest *relaymodel.GeneralOpenAIRequest, relayMode int) int {
	switch relayMode {
	case relaymode.ChatCompletions:
		return openai.CountTokenMessages(textRequest.Messages, textRequest.Model)
	case relaymode.Completions:
		return openai.CountTokenInput(textRequest.Prompt, textRequest.Model)
	case relaymode.Moderations:
		return openai.CountTokenInput(textRequest.Input, textRequest.Model)
	}
	return 0
}

func getPreConsumedQuota(textRequest *relaymodel.GeneralOpenAIRequest, promptTokens int, ratio float64) int64 {
	preConsumedTokens := config.PreConsumedQuota + int64(promptTokens)
	if textRequest.MaxTokens != 0 {
		preConsumedTokens += int64(textRequest.MaxTokens)
	}
	return int64(float64(preConsumedTokens) * ratio)
}

func preConsumeQuota(ctx context.Context, textRequest *relaymodel.GeneralOpenAIRequest, promptTokens int, ratio float64, meta *meta.Meta) (int64, *relaymodel.ErrorWithStatusCode) {
	preConsumedQuota := getPreConsumedQuota(textRequest, promptTokens, ratio)

	userQuota, err := model.CacheGetUserQuota(ctx, meta.UserId)
	if err != nil {
		return preConsumedQuota, openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota-preConsumedQuota < 0 {
		return preConsumedQuota, openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}
	err = model.CacheDecreaseUserQuota(meta.UserId, preConsumedQuota)
	if err != nil {
		return preConsumedQuota, openai.ErrorWrapper(err, "decrease_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota > 100*preConsumedQuota {
		// in this case, we do not pre-consume quota
		// because the user has enough quota
		preConsumedQuota = 0
		logger.Info(ctx, fmt.Sprintf("user %d has enough quota %d, trusted and no need to pre-consume", meta.UserId, userQuota))
	}
	if preConsumedQuota > 0 {
		err := model.PreConsumeTokenQuota(meta.TokenId, preConsumedQuota)
		if err != nil {
			return preConsumedQuota, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
		}
	}
	return preConsumedQuota, nil
}

func postConsumeQuota(ctx context.Context, usage *relaymodel.Usage, meta *meta.Meta, textRequest *relaymodel.GeneralOpenAIRequest, ratio float64, preConsumedQuota int64, modelRatio float64, groupRatio float64, systemPromptReset bool, test bool, responseBody io.ReadCloser) {
	if usage == nil {
		logger.Error(ctx, "usage is nil, which is unexpected")
		return
	}
	var quota int64
	completionRatio := billingratio.GetCompletionRatio(textRequest.Model, meta.ChannelType)
	promptTokens := usage.PromptTokens
	completionTokens := usage.CompletionTokens
	quota = int64(math.Ceil((float64(promptTokens) + float64(completionTokens)*completionRatio) * ratio))
	if ratio != 0 && quota <= 0 {
		quota = 1
	}
	totalTokens := promptTokens + completionTokens
	if totalTokens == 0 {
		quota = 0
	}
	quotaDelta := quota - preConsumedQuota
	err := model.PostConsumeTokenQuota(meta.TokenId, quotaDelta)
	if err != nil {
		logger.Error(ctx, "error consuming token remain quota: "+err.Error())
	}
	if test {
		return
	}
	err = model.CacheUpdateUserQuota(ctx, meta.UserId)
	if err != nil {
		logger.Error(ctx, "error update user quota cache: "+err.Error())
	}
	logContent := fmt.Sprintf("倍率：%.2f × %.2f × %.2f", modelRatio, groupRatio, completionRatio)
	model.RecordConsumeLog(ctx, &model.Log{
		UserId:            meta.UserId,
		ChannelId:         meta.ChannelId,
		PromptTokens:      promptTokens,
		CompletionTokens:  completionTokens,
		ModelName:         textRequest.Model,
		TokenName:         meta.TokenName,
		Quota:             int(quota),
		Content:           logContent,
		IsStream:          meta.IsStream,
		ElapsedTime:       helper.CalcElapsedTime(meta.StartTime),
		SystemPromptReset: systemPromptReset,
	})

	// 发送token使用数据到Kafka
	go func() {
		if !config.KafkaEnabled {
			return // 如果未启用，静默返回
		}
		// 如果模型是embedding模型或rerank模型，不发送token使用数据到Kafka
		if helper.IsRerankModel(textRequest.Model) || helper.IsEmbeddingModel(textRequest.Model) {
			return
		}
		// if textRequest == nil {
		// 	return //空文本请求，或者不是语言模型的请求，返回
		// }

		// 提取输入文本内容
		inputText := extractInputText(textRequest)
		respBody, _ := io.ReadAll(responseBody)
		// 考虑是否需要重置响应体以便后续使用

		// 提取输出文本内容
		outputText := extractResponseText(respBody)

		kafkaData := &kafka.TokenUsageData{
			UserId:       meta.UserId,
			Timestamp:    time.Now().Unix(),
			ModelName:    textRequest.Model,
			TokenName:    meta.TokenName,
			InputTokens:  promptTokens,
			OutputTokens: completionTokens,
			InputText:    inputText,
			OutputText:   outputText, // 使用提取的输出文本
			RequestId:    helper.GetRequestID(ctx),
			ChannelId:    meta.ChannelId,
		}
		err := kafka.SendTokenUsageToKafka(ctx, kafkaData)
		if err != nil {
			logger.Errorf(ctx, "发送token使用数据到Kafka失败: %v", err)
		}
	}()

	model.UpdateUserUsedQuotaAndRequestCount(meta.UserId, quota)
	model.UpdateChannelUsedQuota(meta.ChannelId, quota)
}

// 提取输入文本内容
func extractInputText(textRequest *relaymodel.GeneralOpenAIRequest) string {
	if textRequest == nil {
		return ""
	}

	var inputTexts []string

	// 从Messages中提取文本
	for _, message := range textRequest.Messages {
		if message.Content != nil {
			if content, ok := message.Content.(string); ok {
				inputTexts = append(inputTexts, fmt.Sprintf("[%s]: %s", message.Role, content))
			}
		}
	}

	// 如果没有Messages，检查Prompt字段
	if len(inputTexts) == 0 && textRequest.Prompt != nil {
		if prompt, ok := textRequest.Prompt.(string); ok {
			inputTexts = append(inputTexts, prompt)
		}
	}

	return strings.Join(inputTexts, "\n")
}

// 提取输出文本内容
func extractResponseText(responseData []byte) string {
	if responseData == nil || len(responseData) == 0 {
		return ""
	}

	var responseBody ChatCompletionResponse
	if err := json.Unmarshal(responseData, &responseBody); err != nil {
		return ""
	}

	if len(responseBody.Choices) > 0 {
		return responseBody.Choices[0].Message.Content
	}

	return ""
}

func getMappedModelName(modelName string, mapping map[string]string) (string, bool) {
	if mapping == nil {
		return modelName, false
	}
	mappedModelName := mapping[modelName]
	if mappedModelName != "" {
		return mappedModelName, true
	}
	return modelName, false
}

func isErrorHappened(meta *meta.Meta, resp *http.Response) bool {
	if resp == nil {
		if meta.ChannelType == channeltype.AwsClaude {
			return false
		}
		return true
	}
	if resp.StatusCode != http.StatusOK &&
		// replicate return 201 to create a task
		resp.StatusCode != http.StatusCreated {
		return true
	}
	if meta.ChannelType == channeltype.DeepL {
		// skip stream check for deepl
		return false
	}

	if meta.IsStream && strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") &&
		// Even if stream mode is enabled, replicate will first return a task info in JSON format,
		// requiring the client to request the stream endpoint in the task info
		meta.ChannelType != channeltype.Replicate {
		return true
	}
	return false
}

func setSystemPrompt(ctx context.Context, request *relaymodel.GeneralOpenAIRequest, prompt string) (reset bool) {
	if prompt == "" {
		return false
	}
	if len(request.Messages) == 0 {
		return false
	}
	if request.Messages[0].Role == role.System {
		request.Messages[0].Content = prompt
		logger.Infof(ctx, "rewrite system prompt")
		return true
	}
	request.Messages = append([]relaymodel.Message{{
		Role:    role.System,
		Content: prompt,
	}}, request.Messages...)
	logger.Infof(ctx, "add system prompt")
	return true
}

type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
