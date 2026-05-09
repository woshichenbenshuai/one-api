package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/songquanpeng/one-api/relay/billing"
	billingratio "github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func RelayTextHelper(c *gin.Context) *model.ErrorWithStatusCode {
	ctx := c.Request.Context()
	meta := meta.GetByContext(c)
	// get & validate textRequest
	textRequest, err := getAndValidateTextRequest(c, meta.Mode)
	if err != nil {
		logger.Errorf(ctx, "getAndValidateTextRequest failed: %s", err.Error())
		return openai.ErrorWrapper(err, "invalid_text_request", http.StatusBadRequest)
	}
	meta.IsStream = textRequest.Stream

	// map model name
	meta.OriginModelName = textRequest.Model
	textRequest.Model, _ = getMappedModelName(textRequest.Model, meta.ModelMapping)
	meta.ActualModelName = textRequest.Model
	configureOpenAICompatMode(meta, textRequest)
	// set system prompt if not empty
	systemPromptReset := setSystemPrompt(ctx, textRequest, meta.ForcedSystemPrompt)
	// get model ratio & group ratio
	modelRatio := billingratio.GetModelRatio(textRequest.Model, meta.ChannelType)
	groupRatio := billingratio.GetGroupRatio(meta.Group)
	ratio := modelRatio * groupRatio
	// pre-consume quota
	promptTokens := getPromptTokens(textRequest, meta.Mode)
	meta.PromptTokens = promptTokens
	preConsumedQuota, bizErr := preConsumeQuota(ctx, textRequest, promptTokens, ratio, meta)
	if bizErr != nil {
		logger.Warnf(ctx, "preConsumeQuota failed: %+v", *bizErr)
		return bizErr
	}

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(fmt.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	// get request body
	requestBody, err := getRequestBody(c, meta, textRequest, adaptor)
	if err != nil {
		return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
	}

	// do request
	resp, err := adaptor.DoRequest(c, meta, requestBody)
	if err != nil {
		logger.Errorf(ctx, "DoRequest failed: %s", err.Error())
		if isUpstreamTimeoutError(err) {
			return openai.ErrorWrapper(
				fmt.Errorf("upstream request timeout after %s", getUpstreamTimeoutLabel(meta)),
				"upstream_request_timeout",
				http.StatusGatewayTimeout,
			)
		}
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if isErrorHappened(meta, resp) {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
		return RelayErrorHandler(resp)
	}

	// do response
	usage, respErr := adaptor.DoResponse(c, resp, meta)
	if respErr != nil {
		logger.Errorf(ctx, "respErr is not nil: %+v", respErr)
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
		return respErr
	}
	// post-consume quota
	go postConsumeQuota(ctx, usage, meta, textRequest, ratio, preConsumedQuota, modelRatio, groupRatio, systemPromptReset)
	return nil
}

func getRequestBody(c *gin.Context, meta *meta.Meta, textRequest *model.GeneralOpenAIRequest, adaptor adaptor.Adaptor) (io.Reader, error) {
	if meta.APIType == apitype.OpenAI && meta.ChannelType != channeltype.Baichuan {
		if meta.UseResponsesCompat {
			return getResponsesCompatRequestBody(c, meta, textRequest)
		}
		if !config.EnforceIncludeUsage &&
			meta.OriginModelName == meta.ActualModelName &&
			meta.ForcedSystemPrompt == "" {
			// no need to convert request for openai
			return c.Request.Body, nil
		}
		return getPatchedOpenAIRequestBody(c, meta, textRequest)
	}

	// get request body
	return getConvertedRequestBody(c, meta, textRequest, adaptor)
}

func getResponsesCompatRequestBody(c *gin.Context, meta *meta.Meta, textRequest *model.GeneralOpenAIRequest) (io.Reader, error) {
	convertedRequest := openai.ConvertChatToResponsesRequest(textRequest)
	jsonData, err := json.Marshal(convertedRequest)
	if err != nil {
		logger.Debugf(c.Request.Context(), "responses compat request json_marshal_failed: %s\n", err.Error())
		return nil, err
	}
	logger.Infof(
		c.Request.Context(),
		"responses compat request body prepared: bytes=%d stream=%t prompt_tokens=%d",
		len(jsonData),
		textRequest.Stream,
		meta.PromptTokens,
	)
	logger.Infof(c.Request.Context(), "responses compat request preview: %s", truncateForLog(string(jsonData), 1200))
	return bytes.NewBuffer(jsonData), nil
}

func getPatchedOpenAIRequestBody(c *gin.Context, meta *meta.Meta, textRequest *model.GeneralOpenAIRequest) (io.Reader, error) {
	requestBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, err
	}
	payload := make(map[string]any)
	if err = json.Unmarshal(requestBody, &payload); err != nil {
		return nil, err
	}
	if meta.OriginModelName != meta.ActualModelName {
		payload["model"] = textRequest.Model
	}
	if meta.ForcedSystemPrompt != "" {
		if len(textRequest.Messages) > 0 {
			payload["messages"] = textRequest.Messages
		} else if textRequest.Instructions != "" {
			payload["instructions"] = textRequest.Instructions
		}
	}
	if config.EnforceIncludeUsage && textRequest.Stream && meta.Mode != relaymode.Responses {
		streamOptions, ok := payload["stream_options"].(map[string]any)
		if !ok || streamOptions == nil {
			streamOptions = make(map[string]any)
		}
		streamOptions["include_usage"] = true
		payload["stream_options"] = streamOptions
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	logger.Debugf(c.Request.Context(), "patched openai request: \n%s", string(jsonData))
	return bytes.NewBuffer(jsonData), nil
}

func getConvertedRequestBody(c *gin.Context, meta *meta.Meta, textRequest *model.GeneralOpenAIRequest, adaptor adaptor.Adaptor) (io.Reader, error) {
	convertedRequest, err := adaptor.ConvertRequest(c, meta.Mode, textRequest)
	if err != nil {
		logger.Debugf(c.Request.Context(), "converted request failed: %s\n", err.Error())
		return nil, err
	}
	jsonData, err := json.Marshal(convertedRequest)
	if err != nil {
		logger.Debugf(c.Request.Context(), "converted request json_marshal_failed: %s\n", err.Error())
		return nil, err
	}
	if meta.UseResponsesCompat {
		logger.Infof(
			c.Request.Context(),
			"responses compat request body prepared: bytes=%d stream=%t prompt_tokens=%d",
			len(jsonData),
			textRequest.Stream,
			meta.PromptTokens,
		)
		logger.Infof(c.Request.Context(), "responses compat request preview: %s", truncateForLog(string(jsonData), 1200))
	}
	logger.Debugf(c.Request.Context(), "converted request: \n%s", string(jsonData))
	return bytes.NewBuffer(jsonData), nil
}

func configureOpenAICompatMode(meta *meta.Meta, textRequest *model.GeneralOpenAIRequest) {
	meta.UseResponsesCompat = false
	if meta.APIType != apitype.OpenAI || meta.ChannelType == channeltype.Azure {
		return
	}
	if meta.Config.ResponsesCompat {
		meta.UseResponsesCompat = true
		meta.RequestURLPath = "/v1/responses"
		logger.SysLogf(
			"responses compat enabled: channel_config=true origin_model=%s actual_model=%s client_mode=%d rewritten_path=%s",
			meta.OriginModelName,
			textRequest.Model,
			meta.Mode,
			meta.RequestURLPath,
		)
	}
}

func truncateForLog(input string, limit int) string {
	if len(input) <= limit {
		return input
	}
	return input[:limit] + "...(truncated)"
}

func isUpstreamTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func getUpstreamTimeoutLabel(meta *meta.Meta) string {
	if meta.Config.RequestTimeout > 0 {
		return fmt.Sprintf("%ds", meta.Config.RequestTimeout)
	}
	if config.RelayTimeout > 0 {
		return fmt.Sprintf("%ds", config.RelayTimeout)
	}
	return "the configured limit"
}
