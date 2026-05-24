package gatewaysvc

import "encoding/json"

func estimateMessagesTokens(messages []message, tools []toolSchema) int64 {
	var chars int64
	for _, msg := range messages {
		chars += int64(len(msg.Role) + len(msg.ToolCallID))
		for _, part := range msg.Content {
			chars += int64(len(part.Text) + len(part.ImageURL) + len(part.ImageData) + len(part.MIMEType))
		}
		for _, call := range msg.ToolCalls {
			chars += int64(len(call.ID) + len(call.Type) + len(call.Name) + len(call.ArgumentsJSON))
		}
	}
	for _, tool := range tools {
		chars += int64(len(tool.Name) + len(tool.Description) + len(tool.ParametersJSON))
	}
	return estimateTokensFromChars(chars)
}

func estimateTextTokens(values ...string) int64 {
	var chars int64
	for _, value := range values {
		chars += int64(len(value))
	}
	return estimateTokensFromChars(chars)
}

func estimateTokensFromChars(chars int64) int64 {
	if chars <= 0 {
		return 0
	}
	tokens := chars / 4
	if chars%4 != 0 {
		tokens++
	}
	if tokens == 0 {
		return 1
	}
	return tokens
}

func estimateToolTokens(tools []toolSchema) int64 {
	var chars int64
	for _, tool := range tools {
		encoded, _ := json.Marshal(parametersMap(tool))
		chars += int64(len(tool.Name) + len(tool.Description) + len(encoded))
	}
	return estimateTokensFromChars(chars)
}
