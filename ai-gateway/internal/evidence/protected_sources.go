package evidence

import "strings"

func containsProtectedText(value string, sources []string) bool {
	for _, source := range sources {
		if source != "" && strings.Contains(value, source) {
			return true
		}
	}
	return false
}
