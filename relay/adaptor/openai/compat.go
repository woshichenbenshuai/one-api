package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/songquanpeng/one-api/relay/constant/role"
	"github.com/songquanpeng/one-api/relay/model"
)

func ConvertChatToResponsesRequest(request *model.GeneralOpenAIRequest) map[string]any {
	payload := map[string]any{
		"model":  request.Model,
		"stream": request.Stream,
	}

	if request.Store != nil {
		payload["store"] = *request.Store
	}
	if request.Metadata != nil {
		payload["metadata"] = request.Metadata
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if request.FrequencyPenalty != nil {
		payload["frequency_penalty"] = *request.FrequencyPenalty
	}
	if request.PresencePenalty != nil {
		payload["presence_penalty"] = *request.PresencePenalty
	}
	if request.User != "" {
		payload["user"] = request.User
	}
	if request.MaxCompletionTokens != nil && *request.MaxCompletionTokens > 0 {
		payload["max_output_tokens"] = *request.MaxCompletionTokens
	} else if request.MaxTokens > 0 {
		payload["max_output_tokens"] = request.MaxTokens
	}
	if request.ReasoningEffort != nil && *request.ReasoningEffort != "" {
		payload["reasoning"] = map[string]any{
			"effort": *request.ReasoningEffort,
		}
	}

	instructions := buildResponsesInstructions(request)
	if instructions != "" {
		payload["instructions"] = instructions
	}

	if len(request.Tools) > 0 {
		payload["tools"] = convertResponsesTools(request.Tools)
	}
	if request.ToolChoice != nil {
		payload["tool_choice"] = convertResponsesToolChoice(request.ToolChoice)
	}
	if request.ParallelTooCalls != nil {
		payload["parallel_tool_calls"] = *request.ParallelTooCalls
	}

	input := convertMessagesToResponsesInput(request.Messages)
	if len(input) == 1 {
		if textMessage, ok := tryCollapseResponsesTextInput(input[0]); ok {
			payload["input"] = textMessage
			return payload
		}
	}
	payload["input"] = input
	return payload
}

func buildResponsesInstructions(request *model.GeneralOpenAIRequest) string {
	parts := make([]string, 0, len(request.Messages)+1)
	if request.Instructions != "" {
		parts = append(parts, request.Instructions)
	}
	for _, message := range request.Messages {
		if message.Role != role.System && message.Role != "developer" {
			continue
		}
		content := strings.TrimSpace(message.StringContent())
		if content == "" {
			continue
		}
		parts = append(parts, content)
	}
	return strings.Join(parts, "\n\n")
}

func convertMessagesToResponsesInput(messages []model.Message) []any {
	input := make([]any, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case role.System, "developer":
			continue
		case "tool":
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": message.ToolCallId,
				"output":  messageContentAsString(message),
			})
		default:
			content := convertResponsesMessageContent(message)
			if content != nil {
				input = append(input, map[string]any{
					"role":    normalizeResponsesRole(message.Role),
					"content": content,
				})
			}
			if message.Role == role.Assistant {
				for _, toolCall := range message.ToolCalls {
					input = append(input, map[string]any{
						"type":      "function_call",
						"id":        toolCall.Id,
						"call_id":   toolCall.Id,
						"name":      toolCall.Function.Name,
						"arguments": stringifyToolArguments(toolCall.Function.Arguments),
					})
				}
			}
		}
	}
	return input
}

func convertResponsesMessageContent(message model.Message) any {
	contentParts := message.ParseContent()
	if len(contentParts) == 0 {
		text := strings.TrimSpace(message.StringContent())
		if text == "" {
			return nil
		}
		return text
	}
	if len(contentParts) == 1 && contentParts[0].Type == model.ContentTypeText {
		return contentParts[0].Text
	}
	content := make([]map[string]any, 0, len(contentParts))
	for _, part := range contentParts {
		switch part.Type {
		case model.ContentTypeText:
			content = append(content, map[string]any{
				"type": "input_text",
				"text": part.Text,
			})
		case model.ContentTypeImageURL:
			imageItem := map[string]any{
				"type":      "input_image",
				"image_url": part.ImageURL.Url,
			}
			if part.ImageURL.Detail != "" {
				imageItem["detail"] = part.ImageURL.Detail
			}
			content = append(content, imageItem)
		}
	}
	if len(content) == 0 {
		return nil
	}
	return content
}

func messageContentAsString(message model.Message) string {
	text := strings.TrimSpace(message.StringContent())
	if text != "" {
		return text
	}
	if message.Content == nil {
		return ""
	}
	raw, err := json.Marshal(message.Content)
	if err != nil {
		return fmt.Sprintf("%v", message.Content)
	}
	return string(raw)
}

func stringifyToolArguments(arguments any) string {
	switch value := arguments.(type) {
	case nil:
		return "{}"
	case string:
		if strings.TrimSpace(value) == "" {
			return "{}"
		}
		return value
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return "{}"
		}
		return string(raw)
	}
}

func normalizeResponsesRole(inputRole string) string {
	switch inputRole {
	case role.Assistant:
		return role.Assistant
	case role.System, "developer":
		return "developer"
	default:
		return "user"
	}
}

func tryCollapseResponsesTextInput(item any) (string, bool) {
	message, ok := item.(map[string]any)
	if !ok {
		return "", false
	}
	if normalizeResponsesRole(asString(message["role"])) != "user" {
		return "", false
	}
	content, ok := message["content"].(string)
	if !ok {
		return "", false
	}
	return content, true
}

func convertResponsesTools(tools []model.Tool) []any {
	converted := make([]any, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "" && tool.Type != "function" {
			continue
		}
		converted = append(converted, map[string]any{
			"type":        "function",
			"name":        tool.Function.Name,
			"description": tool.Function.Description,
			"parameters":  tool.Function.Parameters,
		})
	}
	return converted
}

func convertResponsesToolChoice(toolChoice any) any {
	choiceMap, ok := toolChoice.(map[string]any)
	if !ok {
		return toolChoice
	}
	choiceType := asString(choiceMap["type"])
	if choiceType != "function" {
		return toolChoice
	}
	if name := asString(choiceMap["name"]); name != "" {
		return map[string]any{
			"type": "function",
			"name": name,
		}
	}
	functionMap, ok := choiceMap["function"].(map[string]any)
	if !ok {
		return toolChoice
	}
	name := asString(functionMap["name"])
	if name == "" {
		return toolChoice
	}
	return map[string]any{
		"type": "function",
		"name": name,
	}
}

func asString(value any) string {
	text, _ := value.(string)
	return text
}

func responsesOutputToChatMessage(output []ResponsesOutputItem) (model.Message, string) {
	message := model.Message{Role: role.Assistant}
	var textParts strings.Builder
	toolCalls := make([]model.Tool, 0)

	for _, item := range output {
		switch item.Type {
		case "", "message":
			textParts.WriteString(responseItemText(item))
		case "function_call":
			toolCalls = append(toolCalls, model.Tool{
				Id:   coalesce(item.CallID, item.ID),
				Type: "function",
				Function: model.Function{
					Name:      item.Name,
					Arguments: normalizeToolArguments(item.Arguments),
				},
			})
		}
	}

	if textParts.Len() > 0 {
		message.Content = textParts.String()
	}
	if len(toolCalls) > 0 {
		message.ToolCalls = toolCalls
		if message.Content == nil {
			message.Content = nil
		}
		return message, "tool_calls"
	}
	if message.Content == nil {
		message.Content = ""
	}
	return message, "stop"
}

func responseItemText(item ResponsesOutputItem) string {
	var text strings.Builder
	for _, content := range item.Content {
		if content.Type == "output_text" || content.Type == "text" {
			text.WriteString(content.Text)
		}
	}
	return text.String()
}

func normalizeToolArguments(arguments string) string {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return "{}"
	}
	return arguments
}

func coalesce(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
