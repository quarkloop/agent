package gatewaysvc

import (
	"strconv"
	"strings"
)

const (
	optionMaxOutputTokens     = "max_output_tokens"
	optionMaxCompletionTokens = "max_completion_tokens"
	optionMaxTokens           = "max_tokens"
)

func maxOutputTokensOption(options map[string]string) (int, bool) {
	for _, key := range []string{optionMaxOutputTokens, optionMaxCompletionTokens, optionMaxTokens} {
		value := strings.TrimSpace(options[key])
		if value == "" {
			continue
		}
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			continue
		}
		return n, true
	}
	return 0, false
}
