package bedrock

import "unicode/utf16"

const mistralIDCharacters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const mistralIDSpace uint64 = 13_537_086_546_263_552

func normalizeToolCallID(toolCallID string, isMistral bool) string {
	if !isMistral {
		return toolCallID
	}
	if len(toolCallID) == 9 {
		valid := true
		for i := range 9 {
			c := toolCallID[i]
			if (c < '0' || c > '9') && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
				valid = false
				break
			}
		}
		if valid {
			return toolCallID
		}
	}

	var hash uint64 = 14695981039346656037
	for _, unit := range utf16.Encode([]rune(toolCallID)) {
		hash ^= uint64(unit)
		hash *= 1099511628211
	}
	value := hash % mistralIDSpace
	var normalized [9]byte
	for i := 8; i >= 0; i-- {
		normalized[i] = mistralIDCharacters[value%62]
		value /= 62
	}
	return string(normalized[:])
}
