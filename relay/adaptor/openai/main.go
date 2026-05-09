package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/songquanpeng/one-api/common/render"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/common/conv"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

const (
	dataPrefix       = "data: "
	eventPrefix      = "event: "
	done             = "[DONE]"
	dataPrefixLength = len(dataPrefix)
)

func StreamHandler(c *gin.Context, resp *http.Response, relayMode int) (*model.ErrorWithStatusCode, string, *model.Usage) {
	responseText := ""
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
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

func ResponsesStreamHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, string, *model.Usage) {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for k, v := range resp.Header {
		c.Writer.Header().Set(k, v[0])
	}
	common.SetEventStreamHeaders(c)

	var (
		responseText strings.Builder
		usage        *model.Usage
		eventType    string
	)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, eventPrefix) {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, eventPrefix))
		} else if strings.HasPrefix(line, dataPrefix) {
			payload := strings.TrimPrefix(line, dataPrefix)
			if payload != done {
				eventUsage, eventText := parseResponsesStreamEvent(payload)
				if eventUsage != nil {
					usage = eventUsage
				}
				if eventText != "" {
					if strings.HasSuffix(eventType, ".delta") || responseText.Len() == 0 {
						responseText.WriteString(eventText)
					}
				}
			}
		} else if line == "" {
			eventType = ""
		}
		_, err := c.Writer.Write([]byte(line + "\n"))
		if err != nil {
			_ = resp.Body.Close()
			return ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), "", nil
		}
		c.Writer.Flush()
	}

	if err := scanner.Err(); err != nil {
		logger.SysError("error reading responses stream: " + err.Error())
	}

	err := resp.Body.Close()
	if err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), "", nil
	}

	return nil, responseText.String(), usage
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

func ResponsesHandler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	var response ResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &response)
	if err != nil {
		return ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if response.Error.Type != "" {
		return &model.ErrorWithStatusCode{
			Error:      response.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}

	resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))
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

	usage := response.Usage.ToUsage()
	if usage == nil || usage.TotalTokens == 0 {
		responseText := response.OutputText()
		usage = ResponseText2Usage(responseText, modelName, promptTokens)
	}
	return nil, usage
}

func parseResponsesStreamEvent(payload string) (*model.Usage, string) {
	var event struct {
		Delta    string             `json:"delta,omitempty"`
		Text     string             `json:"text,omitempty"`
		Output   []ResponsesOutputItem `json:"output,omitempty"`
		Response *ResponsesResponse `json:"response,omitempty"`
		Usage    *ResponsesUsage    `json:"usage,omitempty"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		logger.SysError("error unmarshalling responses stream event: " + err.Error())
		return nil, ""
	}
	if event.Response != nil {
		if usage := event.Response.Usage.ToUsage(); usage != nil {
			return usage, event.Response.OutputText()
		}
		return nil, event.Response.OutputText()
	}
	if event.Usage != nil {
		if event.Delta != "" {
			return event.Usage.ToUsage(), event.Delta
		}
		if event.Text != "" {
			return event.Usage.ToUsage(), event.Text
		}
		return event.Usage.ToUsage(), outputItemsText(event.Output)
	}
	if event.Delta != "" {
		return nil, event.Delta
	}
	if event.Text != "" {
		return nil, event.Text
	}
	return nil, outputItemsText(event.Output)
}

func outputItemsText(output []ResponsesOutputItem) string {
	var text strings.Builder
	for _, item := range output {
		for _, content := range item.Content {
			if content.Type == "output_text" {
				text.WriteString(content.Text)
			}
		}
	}
	return text.String()
}

func ResponsesToChatHandler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	var response ResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &response)
	if err != nil {
		return ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if response.Error.Type != "" {
		return &model.ErrorWithStatusCode{
			Error:      response.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}

	usage := response.Usage.ToUsage()
	message, finishReason := responsesOutputToChatMessage(response.Output)
	outputText := response.OutputText()
	if usage == nil || usage.TotalTokens == 0 {
		usage = ResponseText2Usage(outputText, modelName, promptTokens)
	}
	chatResponse := TextResponse{
		Id:      response.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   modelName,
		Choices: []TextResponseChoice{{
			Index:        0,
			Message:      message,
			FinishReason: finishReason,
		}},
		Usage: *usage,
	}
	if chatResponse.Id == "" {
		chatResponse.Id = fmt.Sprintf("chatcmpl-%s", random.GetUUID())
	}
	responseJSON, err := json.Marshal(chatResponse)
	if err != nil {
		return ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	for k, v := range resp.Header {
		c.Writer.Header().Set(k, v[0])
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(responseJSON)
	if err != nil {
		return ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), nil
	}
	return nil, usage
}

func ResponsesToChatStreamHandler(c *gin.Context, resp *http.Response, modelName string, promptTokens int) (*model.ErrorWithStatusCode, string, *model.Usage) {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	common.SetEventStreamHeaders(c)

	responseID := fmt.Sprintf("chatcmpl-%s", random.GetUUID())
	createdAt := time.Now().Unix()
	var (
		usage          *model.Usage
		responseText   strings.Builder
		eventType      string
		sawToolCall    bool
		sentRoleChunk  bool
		toolCallStates = map[int]*responsesToolCallState{}
	)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, eventPrefix) {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, eventPrefix))
			continue
		}
		if line == "" {
			eventType = ""
			continue
		}
		if !strings.HasPrefix(line, dataPrefix) {
			continue
		}
		payload := strings.TrimPrefix(line, dataPrefix)
		if payload == done {
			continue
		}
		event, err := parseResponsesStreamPayload(payload)
		if err != nil {
			logger.SysError("error unmarshalling responses stream event: " + err.Error())
			continue
		}
		if eventType == "" {
			eventType = event.EventType
		}
		if event.ResponseID != "" {
			responseID = event.ResponseID
		}
		if event.Usage != nil {
			usage = event.Usage
		}
		if !sentRoleChunk && shouldEmitCompatRoleChunk(eventType, event) {
			err = render.ObjectData(c, ChatCompletionsStreamResponse{
				Id:      responseID,
				Object:  "chat.completion.chunk",
				Created: createdAt,
				Model:   modelName,
				Choices: []ChatCompletionsStreamResponseChoice{{
					Index: 0,
					Delta: model.Message{
						Role: "assistant",
					},
				}},
			})
			if err != nil {
				_ = resp.Body.Close()
				return ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), "", nil
			}
			sentRoleChunk = true
		}

		switch eventType {
		case "response.output_text.delta", "response.refusal.delta":
			if event.Delta == "" {
				continue
			}
			responseText.WriteString(event.Delta)
			err = render.ObjectData(c, ChatCompletionsStreamResponse{
				Id:      responseID,
				Object:  "chat.completion.chunk",
				Created: createdAt,
				Model:   modelName,
				Choices: []ChatCompletionsStreamResponseChoice{{
					Index: 0,
					Delta: model.Message{
						Content: event.Delta,
					},
				}},
			})
		case "response.output_item.added", "response.output_item.done":
			if event.Item == nil || event.Item.Type != "function_call" {
				continue
			}
			sawToolCall = true
			state := getOrCreateToolCallState(toolCallStates, event)
			if eventType == "response.output_item.added" {
				err = render.ObjectData(c, ChatCompletionsStreamResponse{
					Id:      responseID,
					Object:  "chat.completion.chunk",
					Created: createdAt,
					Model:   modelName,
					Choices: []ChatCompletionsStreamResponseChoice{{
						Index: 0,
						Delta: model.Message{
							ToolCalls: []model.Tool{{
								Id:   state.ID,
								Type: "function",
								Function: model.Function{
									Name:      state.Name,
									Arguments: "",
								},
							}},
						},
					}},
				})
				break
			}
			if state.Arguments.Len() > 0 || strings.TrimSpace(event.Item.Arguments) == "" {
				continue
			}
			state.Arguments.WriteString(event.Item.Arguments)
			err = render.ObjectData(c, ChatCompletionsStreamResponse{
				Id:      responseID,
				Object:  "chat.completion.chunk",
				Created: createdAt,
				Model:   modelName,
				Choices: []ChatCompletionsStreamResponseChoice{{
					Index: 0,
					Delta: model.Message{
						ToolCalls: []model.Tool{{
							Id:   state.ID,
							Type: "function",
							Function: model.Function{
								Name:      state.Name,
								Arguments: event.Item.Arguments,
							},
						}},
					},
				}},
			})
		case "response.function_call_arguments.delta", "response.function_call_arguments.done":
			argumentDelta := event.Delta
			if eventType == "response.function_call_arguments.done" && strings.TrimSpace(argumentDelta) == "" && event.Item != nil {
				argumentDelta = event.Item.Arguments
			}
			if argumentDelta == "" {
				argumentDelta = event.Text()
			}
			if argumentDelta == "" {
				if event.Item != nil {
					argumentDelta = event.Item.Arguments
				}
			}
			if argumentDelta == "" {
				continue
			}
			sawToolCall = true
			state := getOrCreateToolCallState(toolCallStates, event)
			state.Arguments.WriteString(argumentDelta)
			err = render.ObjectData(c, ChatCompletionsStreamResponse{
				Id:      responseID,
				Object:  "chat.completion.chunk",
				Created: createdAt,
				Model:   modelName,
				Choices: []ChatCompletionsStreamResponseChoice{{
					Index: 0,
					Delta: model.Message{
						ToolCalls: []model.Tool{{
							Id:   state.ID,
							Type: "function",
							Function: model.Function{
								Name:      state.Name,
								Arguments: argumentDelta,
							},
						}},
					},
				}},
			})
		case "response.completed":
			if event.RawResponse != nil {
				if event.RawResponse.ID != "" {
					responseID = event.RawResponse.ID
				}
				if event.RawResponse.Usage != nil {
					usage = event.RawResponse.Usage.ToUsage()
				}
			}
			continue
		default:
			continue
		}
		if err != nil {
			_ = resp.Body.Close()
			return ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), "", nil
		}
	}

	if err := scanner.Err(); err != nil {
		logger.SysError("error reading responses stream: " + err.Error())
	}

	finishReason := "stop"
	if sawToolCall {
		finishReason = "tool_calls"
	}
	err := render.ObjectData(c, ChatCompletionsStreamResponse{
		Id:      responseID,
		Object:  "chat.completion.chunk",
		Created: createdAt,
		Model:   modelName,
		Choices: []ChatCompletionsStreamResponseChoice{{
			Index:        0,
			Delta:        model.Message{},
			FinishReason: &finishReason,
		}},
	})
	if err != nil {
		_ = resp.Body.Close()
		return ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), "", nil
	}

	if usage == nil || usage.TotalTokens == 0 {
		usage = ResponseText2Usage(responseText.String(), modelName, promptTokens)
	}
	err = render.ObjectData(c, ChatCompletionsStreamResponse{
		Id:      responseID,
		Object:  "chat.completion.chunk",
		Created: createdAt,
		Model:   modelName,
		Choices: []ChatCompletionsStreamResponseChoice{},
		Usage:   usage,
	})
	if err != nil {
		_ = resp.Body.Close()
		return ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), "", nil
	}

	render.Done(c)
	err = resp.Body.Close()
	if err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), "", nil
	}
	return nil, responseText.String(), usage
}

type responsesStreamPayload struct {
	Type        string                `json:"type,omitempty"`
	ID          string                `json:"id,omitempty"`
	ResponseID  string                `json:"response_id,omitempty"`
	ItemID      string                `json:"item_id,omitempty"`
	OutputIndex int                   `json:"output_index,omitempty"`
	Delta       string                `json:"delta,omitempty"`
	TextBody    string                `json:"text,omitempty"`
	Item        *ResponsesOutputItem  `json:"item,omitempty"`
	Output      []ResponsesOutputItem `json:"output,omitempty"`
	Response    *ResponsesResponse    `json:"response,omitempty"`
	UsageRaw    *ResponsesUsage       `json:"usage,omitempty"`
}

func parseResponsesStreamPayload(payload string) (*responsesStreamPayloadCompat, error) {
	var event responsesStreamPayload
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return nil, err
	}
	compat := &responsesStreamPayloadCompat{
		ResponseID:  coalesce(event.ResponseID, event.ID),
		EventType:   event.Type,
		OutputIndex: event.OutputIndex,
		ItemID:      event.ItemID,
		Item:        event.Item,
		RawResponse: event.Response,
	}
	if event.Response != nil {
		compat.ResponseID = event.Response.ID
		compat.Usage = event.Response.Usage.ToUsage()
		compat.TextValue = event.Response.OutputText()
		return compat, nil
	}
	if event.UsageRaw != nil {
		compat.Usage = event.UsageRaw.ToUsage()
	}
	if event.Delta != "" {
		compat.Delta = event.Delta
		compat.TextValue = event.Delta
		return compat, nil
	}
	if event.TextBody != "" {
		compat.TextValue = event.TextBody
		return compat, nil
	}
	compat.TextValue = outputItemsText(event.Output)
	return compat, nil
}

type responsesStreamPayloadCompat struct {
	ResponseID  string
	EventType   string
	ItemID      string
	OutputIndex int
	TextValue   string
	Delta       string
	Usage       *model.Usage
	Item        *ResponsesOutputItem
	RawResponse *ResponsesResponse
}

func (p *responsesStreamPayloadCompat) Text() string {
	if p == nil {
		return ""
	}
	return p.TextValue
}

type responsesToolCallState struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

func shouldEmitCompatRoleChunk(eventType string, event *responsesStreamPayloadCompat) bool {
	if event == nil {
		return false
	}
	switch eventType {
	case "response.output_text.delta", "response.refusal.delta", "response.output_item.added", "response.function_call_arguments.delta":
		return true
	}
	return event.Text() != ""
}

func getOrCreateToolCallState(states map[int]*responsesToolCallState, event *responsesStreamPayloadCompat) *responsesToolCallState {
	if state, ok := states[event.OutputIndex]; ok {
		if event.Item != nil {
			state.ID = coalesce(state.ID, event.Item.CallID, event.Item.ID, event.ItemID)
			state.Name = coalesce(state.Name, event.Item.Name)
		}
		return state
	}
	state := &responsesToolCallState{}
	if event.Item != nil {
		state.ID = coalesce(event.Item.CallID, event.Item.ID, event.ItemID)
		state.Name = event.Item.Name
	} else {
		state.ID = event.ItemID
	}
	states[event.OutputIndex] = state
	return state
}
