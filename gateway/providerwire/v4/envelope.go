package providerwirev4

import (
	"mime"
	"regexp"
	"strconv"
	"strings"
)

var (
	qvaluePattern     = regexp.MustCompile(`^(?:0(?:\.[0-9]{0,3})?|1(?:\.0{0,3})?)$`)
	rawQMarkerPattern = regexp.MustCompile(`(?i)(?:^|;)[\t ]*q[\t ]*=`)
	rawQvaluePattern  = regexp.MustCompile(`(?i)(?:^|;)[\t ]*q=(?:0(?:\.[0-9]{0,3})?|1(?:\.0{0,3})?)[\t ]*(?:;|$)`)
)

func acceptsRepresentation(header, target string) (compatible, valid bool) {
	targetType, _, ok := strings.Cut(target, "/")
	if !ok {
		return false, false
	}
	for _, entry := range strings.Split(header, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return false, false
		}
		if rawQMarkerPattern.MatchString(entry) && !rawQvaluePattern.MatchString(entry) {
			return false, false
		}
		mediaType, params, err := mime.ParseMediaType(entry)
		if err != nil {
			return false, false
		}
		quality := 1.0
		if rawQuality, present := params["q"]; present {
			if !qvaluePattern.MatchString(rawQuality) {
				return false, false
			}
			quality, err = strconv.ParseFloat(rawQuality, 64)
			if err != nil {
				return false, false
			}
		}
		if quality > 0 && (mediaType == target || mediaType == targetType+"/*" || mediaType == "*/*") {
			compatible = true
		}
	}
	return compatible, true
}
