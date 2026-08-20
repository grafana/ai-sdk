package providerwirev4

import (
	"mime"
	"regexp"
	"strconv"
	"strings"
)

var qvaluePattern = regexp.MustCompile(`^(?:0(?:\.[0-9]{0,3})?|1(?:\.0{0,3})?)$`)

func acceptsRepresentation(header, target string) (compatible, valid bool) {
	targetType, _, ok := strings.Cut(target, "/")
	if !ok {
		return false, false
	}
	entries, ok := splitHeaderList(header, ',')
	if !ok {
		return false, false
	}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return false, false
		}
		mediaType, _, err := mime.ParseMediaType(entry)
		if err != nil {
			return false, false
		}
		quality := 1.0
		parameters, ok := splitHeaderList(entry, ';')
		if !ok {
			return false, false
		}
		for _, parameter := range parameters[1:] {
			parameter = strings.TrimLeft(parameter, "\t ")
			name, value, present := strings.Cut(parameter, "=")
			if present && strings.EqualFold(strings.TrimSpace(name), "q") {
				if !strings.EqualFold(name, "q") {
					return false, false
				}
				value = strings.TrimRight(value, "\t ")
				if !qvaluePattern.MatchString(value) {
					return false, false
				}
				quality, err = strconv.ParseFloat(value, 64)
				if err != nil {
					return false, false
				}
			}
		}
		if quality > 0 && (mediaType == target || mediaType == targetType+"/*" || mediaType == "*/*") {
			compatible = true
		}
	}
	return compatible, true
}

func splitHeaderList(value string, separator byte) ([]string, bool) {
	var result []string
	start := 0
	quoted := false
	escaped := false
	for index := 0; index < len(value); index++ {
		switch {
		case escaped:
			escaped = false
		case quoted && value[index] == '\\':
			escaped = true
		case value[index] == '"':
			quoted = !quoted
		case !quoted && value[index] == separator:
			result = append(result, value[start:index])
			start = index + 1
		}
	}
	if quoted || escaped {
		return nil, false
	}
	return append(result, value[start:]), true
}
