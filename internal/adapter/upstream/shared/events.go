package shared

func CreateAnthropicFinalEvents(outputTokens, inputTokens int, stopReason string) []map[string]any {
	usage := map[string]any{
		"output_tokens": outputTokens,
		"input_tokens":  inputTokens,
	}

	events := []map[string]any{
		{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason":   stopReason,
				"stop_sequence": nil,
			},
			"usage": usage,
		},
		{
			"type": "message_stop",
		},
	}

	return events
}
