package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/songquanpeng/one-api/common/render"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/conv"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

const (
	dataPrefix       = "data: "
	done             = "[DONE]"
	dataPrefixLength = len(dataPrefix)
)

func StreamHandler(c *gin.Context, resp *http.Response, relayMode int) (*model.ErrorWithStatusCode, string, *model.Usage) {
	responseText := ""
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	var usage *model.Usage

	common.SetEventStreamHeaders(c)

	doneRendered := false
	for scanner.Scan() {
		data := scanner.Text()
		if len(data) < dataPrefixLength { // ignore blank line or wrong format
			continue
		}
		if data[:dataPrefixLength] != dataPrefix && data[:dataPrefixLength] != done {
			continue
		}
		if strings.HasPrefix(data[dataPrefixLength:], done) {
			render.StringData(c, data)
			doneRendered = true
			continue
		}
		switch relayMode {
		case relaymode.ChatCompletions:
			var streamResponse ChatCompletionsStreamResponse
			err := json.Unmarshal([]byte(data[dataPrefixLength:]), &streamResponse)
			if err != nil {
				logger.SysError("error unmarshalling stream response: " + err.Error())
				render.StringData(c, data) // if error happened, pass the data to client
				continue                   // just ignore the error
			}
			if len(streamResponse.Choices) == 0 && streamResponse.Usage == nil {
				// but for empty choice and no usage, we should not pass it to client, this is for azure
				continue // just ignore empty choice
			}
			render.StringData(c, data)
			for _, choice := range streamResponse.Choices {
				responseText += conv.AsString(choice.Delta.Content)
			}
			if streamResponse.Usage != nil {
				usage = streamResponse.Usage
			}
		case relaymode.Completions:
			render.StringData(c, data)
			var streamResponse CompletionsStreamResponse
			err := json.Unmarshal([]byte(data[dataPrefixLength:]), &streamResponse)
			if err != nil {
				logger.SysError("error unmarshalling stream response: " + err.Error())
				continue
			}
			for _, choice := range streamResponse.Choices {
				responseText += choice.Text
			}
		}
	}

	if err := scanner.Err(); err != nil {
		logger.SysError("error reading stream: " + err.Error())
	}

	if !doneRendered {
		render.Done(c)
	}

	err := resp.Body.Close()
	if err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), "", nil
	}

	return nil, responseText, usage
}

func Handler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	var textResponse SlimTextResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &textResponse)
	if err != nil {
		return ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if textResponse.Error.Type != "" {
		return &model.ErrorWithStatusCode{
			Error:      textResponse.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}
	// Reset response body
	resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))

	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the HTTPClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	for k, v := range resp.Header {
		c.Writer.Header().Set(k, v[0])
	}
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		return ErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	if textResponse.Usage.TotalTokens == 0 || (textResponse.Usage.PromptTokens == 0 && textResponse.Usage.CompletionTokens == 0) {
		completionTokens := 0
		for _, choice := range textResponse.Choices {
			completionTokens += CountTokenText(choice.Message.StringContent(), modelName)
		}
		textResponse.Usage = model.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		}
	}
	return nil, &textResponse.Usage
}

// RerankHandler 处理 Rerank 模型的响应
// 为了保持向后兼容性，只在必要时才修改响应体，否则透传原始响应
func RerankHandler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	var rerankResponse RerankResponse
	var results []RerankResult
	var needsTransform bool // 标记是否需要转换响应格式

	// 首先尝试解析为简单数组格式: [{"index": 0, "score": 0.016}, ...]
	err = json.Unmarshal(responseBody, &results)
	if err == nil && len(results) > 0 {
		// 成功解析为数组格式，需要转换为标准响应
		needsTransform = true
		rerankResponse.Results = results
		rerankResponse.Object = "list"
		rerankResponse.Model = modelName

		// 计算 token 使用量

		rerankResponse.Usage = RerankUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: 0,
			TotalTokens:      promptTokens,
		}
	} else {
		// 尝试解析为标准格式（带 results、usage 等字段）
		err = json.Unmarshal(responseBody, &rerankResponse)
		if err != nil {
			return ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
		}

		// 检查是否有错误
		if rerankResponse.Error != nil && rerankResponse.Error.Type != "" {
			return &model.ErrorWithStatusCode{
				Error:      *rerankResponse.Error,
				StatusCode: resp.StatusCode,
			}, nil
		}

		// 标准格式不需要转换，但需要补充 usage（如果缺失）
		needsTransform = false

		// 如果响应中没有 usage 信息，计算一个
		if rerankResponse.Usage.TotalTokens == 0 {
			completionTokens := len(rerankResponse.Results)
			rerankResponse.Usage = RerankUsage{
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      promptTokens + completionTokens,
			}
			// 如果我们补充了 usage，需要重新序列化响应
			needsTransform = true
		} else if rerankResponse.Usage.PromptTokens == 0 && rerankResponse.Usage.CompletionTokens == 0 {
			// 如果只有 total_tokens，将 PromptTokens 设置为 total_tokens
			rerankResponse.Usage.PromptTokens = rerankResponse.Usage.TotalTokens
			rerankResponse.Usage.CompletionTokens = 0
			needsTransform = true
		} else if rerankResponse.Usage.PromptTokens == 0 && promptTokens > 0 {
			// 如果有 usage 但缺少某些字段，补充计算
			rerankResponse.Usage.PromptTokens = promptTokens
			if rerankResponse.Usage.CompletionTokens == 0 {
				rerankResponse.Usage.CompletionTokens = rerankResponse.Usage.TotalTokens - promptTokens
			}
			// 补充了 usage 字段，需要重新序列化
			needsTransform = true

		}
	}

	// 准备返回的 usage 信息
	usage := &model.Usage{
		PromptTokens:     rerankResponse.Usage.PromptTokens,
		CompletionTokens: rerankResponse.Usage.CompletionTokens,
		TotalTokens:      rerankResponse.Usage.TotalTokens,
	}

	// 如果不需要转换，像 Handler 一样透传原始响应（保持向后兼容）
	if !needsTransform {
		// Reset response body
		resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))

		// 复制响应头
		for k, v := range resp.Header {
			c.Writer.Header().Set(k, v[0])
		}
		c.Writer.WriteHeader(resp.StatusCode)

		// 透传原始响应体
		_, err = io.Copy(c.Writer, resp.Body)
		if err != nil {
			return ErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError), nil
		}
		err = resp.Body.Close()
		if err != nil {
			return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
		}

		return nil, usage
	}

	// 需要转换：将标准格式的响应序列化并发送给客户端
	// 如果没有错误，确保 Error 字段为 nil
	if rerankResponse.Error != nil && rerankResponse.Error.Type == "" {
		rerankResponse.Error = nil
	}
	jsonResponse, err := json.Marshal(rerankResponse)
	if err != nil {
		return ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}

	// 设置响应头（不复制原始响应头，因为内容已改变）
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)

	// 写入转换后的响应
	_, err = c.Writer.Write(jsonResponse)
	if err != nil {
		return ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), nil
	}

	return nil, usage
}
