package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/message"
	"github.com/songquanpeng/one-api/middleware"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/monitor"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/controller"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func buildTestRequest(model string) *relaymodel.GeneralOpenAIRequest {
	if model == "" {
		model = "gpt-3.5-turbo"
	}
	testRequest := &relaymodel.GeneralOpenAIRequest{
		Model: model,
	}
	testMessage := relaymodel.Message{
		Role:    "user",
		Content: config.TestPrompt,
	}
	testRequest.Messages = append(testRequest.Messages, testMessage)
	return testRequest
}

func parseTestResponse(resp string) (*openai.TextResponse, string, error) {
	var response openai.TextResponse
	err := json.Unmarshal([]byte(resp), &response)
	if err != nil {
		return nil, "", err
	}
	if len(response.Choices) == 0 {
		return nil, "", errors.New("response has no choices")
	}
	stringContent, ok := response.Choices[0].Content.(string)
	if !ok {
		return nil, "", errors.New("response content is not string")
	}
	return &response, stringContent, nil
}

func testChannel(ctx context.Context, channel *model.Channel, request *relaymodel.GeneralOpenAIRequest) (responseMessage string, err error, openaiErr *relaymodel.Error) {
	startTime := time.Now()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 根据模型类型选择正确的API路径
	apiPath := "/v1/chat/completions"
	relayMode := relaymode.ChatCompletions

	// 检测是否为嵌入模型
	modelName := request.Model
	if isRerankModel(modelName) {
		// 添加对rerank模型的支持
		apiPath = "/v1/rerank"
		relayMode = relaymode.Rerank

		// 确保请求格式正确（rerank模型需要query和documents字段）
		if len(request.Messages) > 0 && request.Query == nil && request.Messages[0].Content != "" {
			request.Query = request.Messages[0].Content
		}

		// 如果没有documents字段，添加一个示例文档
		if request.Documents == nil || len(request.Documents) == 0 {
			request.Documents = []string{"This is Rerank testing."}
		}
	} else if isEmbeddingModel(modelName) {
		apiPath = "/v1/embeddings"
		relayMode = relaymode.Embeddings

		// 确保请求格式正确（嵌入模型需要input字段而非messages）
		if len(request.Messages) > 0 && request.Input == nil && request.Messages[0].Content != "" {
			request.Input = request.Messages[0].Content
		}
	}

	c.Request = &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: apiPath},
		Body:   nil,
		Header: make(http.Header),
	}

	c.Request.Header.Set("Authorization", "Bearer "+channel.Key)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(ctxkey.Channel, channel.Type)
	c.Set(ctxkey.BaseURL, channel.GetBaseURL())
	cfg, _ := channel.LoadConfig()
	c.Set(ctxkey.Config, cfg)
	middleware.SetupContextForSelectedChannel(c, channel, "")
	meta := meta.GetByContext(c)
	apiType := channeltype.ToAPIType(channel.Type)
	adaptor := relay.GetAdaptor(apiType)
	if adaptor == nil {
		return "", fmt.Errorf("invalid api type: %d, adaptor is nil", apiType), nil
	}
	adaptor.Init(meta)

	// 处理模型名称和映射...
	modelMap := channel.GetModelMapping()
	if modelName == "" || !strings.Contains(channel.Models, modelName) {
		modelNames := strings.Split(channel.Models, ",")
		if len(modelNames) > 0 {
			modelName = modelNames[0]
		}
	}
	if modelMap != nil && modelMap[modelName] != "" {
		modelName = modelMap[modelName]
	}
	meta.OriginModelName, meta.ActualModelName = request.Model, modelName
	request.Model = modelName

	// 使用正确的relayMode进行请求转换
	convertedRequest, err := adaptor.ConvertRequest(c, relayMode, request)
	if err != nil {
		return "", err, nil
	}
	jsonData, err := json.Marshal(convertedRequest)
	if err != nil {
		return "", err, nil
	}
	defer func() {
		logContent := fmt.Sprintf("渠道 %s 测试成功，响应：%s", channel.Name, responseMessage)
		if err != nil || openaiErr != nil {
			errorMessage := ""
			if err != nil {
				errorMessage = err.Error()
			} else {
				errorMessage = openaiErr.Message
			}
			logContent = fmt.Sprintf("渠道 %s 测试失败，错误：%s", channel.Name, errorMessage)
		}
		go model.RecordTestLog(ctx, &model.Log{
			ChannelId:   channel.Id,
			ModelName:   modelName,
			Content:     logContent,
			ElapsedTime: helper.CalcElapsedTime(startTime),
		})
	}()
	logger.SysLog(string(jsonData))
	requestBody := bytes.NewBuffer(jsonData)
	c.Request.Body = io.NopCloser(requestBody)
	resp, err := adaptor.DoRequest(c, meta, requestBody)
	if err != nil {
		return "", err, nil
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		err := controller.RelayErrorHandler(resp)
		errorMessage := err.Error.Message
		if errorMessage != "" {
			errorMessage = ", error message: " + errorMessage
		}
		return "", fmt.Errorf("http status code: %d%s", resp.StatusCode, errorMessage), &err.Error
	}
	usage, respErr := adaptor.DoResponse(c, resp, meta)
	if respErr != nil {
		return "", fmt.Errorf("%s", respErr.Error.Message), &respErr.Error
	}
	if usage == nil {
		return "", errors.New("usage is nil"), nil
	}
	rawResponse := w.Body.String()

	// 响应解析部分需要根据模型类型不同进行处理
	if isRerankModel(modelName) {
		// 解析rerank模型响应
		responseMessage, err = parseRerankTestResponse(rawResponse)
	} else if isEmbeddingModel(modelName) {
		// 解析嵌入模型响应
		responseMessage, err = parseEmbeddingTestResponse(rawResponse)
	} else {
		// 解析聊天模型响应
		_, responseMessage, err = parseTestResponse(rawResponse)
	}

	if err != nil {
		logger.SysError(fmt.Sprintf("failed to parse error: %s, \nresponse: %s", err.Error(), rawResponse))
		return "", err, nil
	}
	result := w.Result()
	// print result.Body
	respBody, err := io.ReadAll(result.Body)
	if err != nil {
		return "", err, nil
	}
	logger.SysLog(fmt.Sprintf("testing channel #%d, response: \n%s", channel.Id, string(respBody)))
	return responseMessage, nil, nil
}

// 判断是否为嵌入模型
func isEmbeddingModel(modelName string) bool {
	return helper.IsEmbeddingModel(modelName)
}

// 解析嵌入模型的测试响应
func parseEmbeddingTestResponse(rawResponse string) (string, error) {
	var response map[string]interface{}
	err := json.Unmarshal([]byte(rawResponse), &response)
	if err != nil {
		return "", err
	}

	// 检查是否有嵌入数据
	data, ok := response["data"].([]interface{})
	if !ok || len(data) == 0 {
		return "嵌入向量生成成功，但无数据返回", nil
	}

	// 获取第一个嵌入向量的维度
	firstEmbedding, ok := data[0].(map[string]interface{})
	if !ok {
		return "嵌入向量格式异常", nil
	}

	embedding, ok := firstEmbedding["embedding"].([]interface{})
	if !ok {
		return "嵌入向量格式异常", nil
	}

	return fmt.Sprintf("嵌入向量生成成功，维度: %d", len(embedding)), nil
}

// 判断是否为rerank模型
func isRerankModel(modelName string) bool {
	return helper.IsRerankModel(modelName)
}

// 解析rerank模型的测试响应
func parseRerankTestResponse(rawResponse string) (string, error) {
	var response map[string]interface{}
	err := json.Unmarshal([]byte(rawResponse), &response)
	if err != nil {
		return "", err
	}

	// 检查是否有结果数据
	results, ok := response["results"].([]interface{})
	if !ok || len(results) == 0 {
		return "重排序成功，但无结果返回", nil
	}

	// 获取结果数量
	return fmt.Sprintf("重排序成功，返回 %d 个结果", len(results)), nil
}

func TestChannel(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	modelName := c.Query("model")
	testRequest := buildTestRequest(modelName)
	tik := time.Now()
	responseMessage, err, _ := testChannel(ctx, channel, testRequest)
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	if err != nil {
		milliseconds = 0
	}
	go channel.UpdateResponseTime(milliseconds)
	consumedTime := float64(milliseconds) / 1000.0
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":   false,
			"message":   err.Error(),
			"time":      consumedTime,
			"modelName": modelName,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   responseMessage,
		"time":      consumedTime,
		"modelName": modelName,
	})
	return
}

var testAllChannelsLock sync.Mutex
var testAllChannelsRunning bool = false

// 定义渠道测试结果结构体
type ChannelTestResult struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message,omitempty"` // Only include if there's an error
	ResponseTime int64     `json:"response_time"`
	StartTime    time.Time `json:"start_time"` // 记录测试开始的时间
}

func testChannels(ctx context.Context, notify bool, scope string) error {
	if config.RootUserEmail == "" {
		config.RootUserEmail = model.GetRootUserEmail()
	}
	testAllChannelsLock.Lock()
	if testAllChannelsRunning {
		testAllChannelsLock.Unlock()
		return errors.New("测试已在运行中")
	}
	testAllChannelsRunning = true
	testAllChannelsLock.Unlock()
	channels, err := model.GetAllChannels(0, 0, scope)
	if err != nil {
		return err
	}
	var disableThreshold = int64(config.ChannelDisableThreshold * 1000)
	if disableThreshold == 0 {
		disableThreshold = 10000000 // a impossible value
	}

	go func() {
		// 确保 goroutine 结束后释放锁和设置标志
		defer func() {
			testAllChannelsLock.Lock()
			testAllChannelsRunning = false
			testAllChannelsLock.Unlock()
		}()

		// 用于收集所有渠道的测试结果
		var testResults []ChannelTestResult

		for _, channel := range channels {
			startTime := time.Now()
			isChannelEnabled := channel.Status == model.ChannelStatusEnabled
			tik := time.Now()
			testRequest := buildTestRequest(channel.Models)
			_, err, openaiErr := testChannel(ctx, channel, testRequest)
			tok := time.Now()
			milliseconds := tok.Sub(tik).Milliseconds()

			if isChannelEnabled && milliseconds > disableThreshold {
				err = fmt.Errorf("响应时间 %.2fs 超过阈值 %.2fs", float64(milliseconds)/1000.0, float64(disableThreshold)/1000.0)
				if config.AutomaticDisableChannelEnabled {
					monitor.DisableChannel(channel.Id, channel.Name, err.Error())
				} else {
					_ = message.Notify(message.ByAll, fmt.Sprintf("渠道 %s （%d）测试超时", channel.Name, channel.Id), "", err.Error())
				}
			}
			if isChannelEnabled && monitor.ShouldDisableChannel(openaiErr, -1) {
				monitor.DisableChannel(channel.Id, channel.Name, err.Error())
			}
			if !isChannelEnabled && monitor.ShouldEnableChannel(err, openaiErr) {
				monitor.EnableChannel(channel.Id, channel.Name)
			}
			channel.UpdateResponseTime(milliseconds)

			// 记录当前渠道的测试结果
			result := ChannelTestResult{
				ID:           channel.Id,
				Name:         channel.Name,
				Success:      err == nil && openaiErr == nil, // 成功条件
				ResponseTime: milliseconds,
				StartTime:    startTime,
			}
			if err != nil {
				result.ErrorMessage = err.Error()
			} else if openaiErr != nil {
				result.ErrorMessage = openaiErr.Message
			}
			testResults = append(testResults, result) // 将结果添加到切片中

			time.Sleep(config.RequestInterval)
		}

		// 将测试结果序列化为 JSON
		resultsJSON, marshalErr := json.Marshal(testResults)
		if marshalErr != nil {
			logger.SysError(fmt.Sprintf("failed to marshal test results: %s", marshalErr.Error()))
		} else {
			// 保存 resultsJSON 到 options 表中，使用键 "last_channel_test_results"
			saveErr := model.UpdateOption("last_channel_test_results", string(resultsJSON))
			if saveErr != nil {
				logger.SysError(fmt.Sprintf("failed to save test results to options: %s", saveErr.Error()))
			} else {
				logger.SysLog("Channel test results saved to options.")
			}
		}

		if notify {
			err := message.Notify(message.ByAll, "渠道测试完成", "", "渠道测试完成，如果没有收到禁用通知，说明所有渠道都正常")
			if err != nil {
				logger.SysError(fmt.Sprintf("failed to send email: %s", err.Error()))
			}
		}
	}()
	return nil
}

func TestChannels(c *gin.Context) {
	ctx := c.Request.Context()
	scope := c.Query("scope")
	if scope == "" {
		scope = "all"
	}
	err := testChannels(ctx, true, scope)
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
	})
	return
}

func AutomaticallyTestChannels(frequency int) {
	ctx := context.Background()
	for {
		time.Sleep(time.Duration(frequency) * time.Minute)
		logger.SysLog("testing all channels")
		_ = testChannels(ctx, false, "enabled")
		logger.SysLog("channel test finished")
	}
}

// GetLastChannelTestResults 获取上一次渠道测试结果
func GetLastChannelTestResults(c *gin.Context) {
	// 从 config.OptionMap 中读取保存的测试结果
	testResultsJSON, exists := config.OptionMap["last_channel_test_results"]
	if !exists {
		// 如果不存在，返回空数组或适当的错误
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "No previous test results found",
			"data":    []ChannelTestResult{}, // 返回空数组
		})
		return
	}

	var testResults []ChannelTestResult
	err := json.Unmarshal([]byte(testResultsJSON), &testResults)
	if err != nil {
		logger.SysError(fmt.Sprintf("failed to unmarshal test results: %s", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to parse test results",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Successfully retrieved last test results",
		"data":    testResults,
	})
}
