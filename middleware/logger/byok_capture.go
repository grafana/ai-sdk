package logger

func redactBYOKOptions(value any) {
	options, ok := value.(map[string]any)
	if !ok {
		return
	}
	gateway, ok := options["gateway"].(map[string]any)
	if !ok {
		if options["gateway"] != nil {
			options["gateway"] = redactedValue
		}
		return
	}
	if _, ok := gateway["byok"]; ok {
		gateway["byok"] = redactedValue
	}
}

func redactNestedBYOKOptions(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			if key == "providerOptions" {
				if _, ok := item.(map[string]any); !ok && item != nil {
					value[key] = redactedValue
					continue
				}
				redactBYOKOptions(item)
			}
			redactNestedBYOKOptions(item)
		}
	case []any:
		for _, item := range value {
			redactNestedBYOKOptions(item)
		}
	}
}
