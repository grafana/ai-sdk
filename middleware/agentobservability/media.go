package agentobservability

import (
	"encoding/base64"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/provider"
)

func contentFilePartToAgento11y(part provider.ContentPart, providerType string) (agento11y.Part, bool) {
	return mediaDataToAgento11y(part.MediaType, part.Filename, part.Data, providerType)
}

func generateFilePartToAgento11y(part provider.GenerateContentPart, providerType string) (agento11y.Part, bool) {
	return mediaDataToAgento11y(part.MediaType, part.Filename, part.Data, providerType)
}

func streamFilePartToAgento11y(part provider.StreamPart, providerType string) (agento11y.Part, bool) {
	if part.Data == nil {
		return agento11y.Part{}, false
	}
	data := provider.DataContent{
		Bytes:  part.Data.Bytes,
		Base64: part.Data.Base64,
		URL:    part.Data.URL,
	}
	return mediaDataToAgento11y(part.MediaType, part.Filename, &data, providerType)
}

func mediaDataToAgento11y(mediaType, filename string, data *provider.DataContent, providerType string) (agento11y.Part, bool) {
	if data == nil || data.Validate() != nil {
		return agento11y.Part{}, false
	}

	rawURL, urlMediaType, urlPath, ok := mediaDataURL(data)
	if !ok {
		return agento11y.Part{}, false
	}

	concreteType := concreteMediaType(mediaType)
	if concreteType != "" && urlMediaType != "" && concreteType != urlMediaType {
		return agento11y.Part{}, false
	}
	if concreteType == "" {
		concreteType = urlMediaType
	}
	if concreteType == "" {
		concreteType = mediaTypeFromName(filename)
	}
	if concreteType == "" {
		concreteType = mediaTypeFromName(urlPath)
	}
	if concreteType == "" {
		concreteType = mediaTypeFromInlineData(data)
	}
	kind := mediaKind(concreteType)
	if kind == "" {
		return agento11y.Part{}, false
	}

	if rawURL == "" {
		rawURL = inlineMediaURL(data, concreteType)
		if rawURL == "" {
			return agento11y.Part{}, false
		}
	}

	part := agento11y.MediaPart(agento11y.Media{
		Kind:     kind,
		URL:      rawURL,
		MIMEType: concreteType,
		Name:     strings.TrimSpace(filename),
	})
	part.Metadata.ProviderType = providerType
	return part, true
}

func mediaDataURL(data *provider.DataContent) (rawURL, mediaType, path string, ok bool) {
	if data == nil {
		return "", "", "", false
	}
	if data.URL == "" {
		if data.Base64 != "" && !validBase64(data.Base64) {
			return "", "", "", false
		}
		return "", "", "", true
	}

	rawURL = strings.TrimSpace(data.URL)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "data":
		mediaType, ok = mediaTypeFromDataURL(rawURL)
		return rawURL, mediaType, "", ok
	case "http", "https":
		if parsed.Host == "" || parsed.User != nil {
			return "", "", "", false
		}
		return rawURL, "", parsed.Path, true
	default:
		return "", "", "", false
	}
}

func mediaTypeFromDataURL(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(trimmed), "data:") {
		return "", false
	}
	header, payload, found := strings.Cut(trimmed[len("data:"):], ",")
	if !found || payload == "" {
		return "", false
	}

	base64Encoded := false
	mediaHeader := header
	if index := strings.LastIndex(header, ";"); index >= 0 && strings.EqualFold(strings.TrimSpace(header[index+1:]), "base64") {
		base64Encoded = true
		mediaHeader = header[:index]
	}
	mediaType := concreteMediaType(mediaHeader)
	if mediaType == "" {
		return "", false
	}
	decodedPayload, err := url.PathUnescape(payload)
	if err != nil {
		return "", false
	}
	if base64Encoded {
		return mediaType, validBase64(decodedPayload)
	}
	return mediaType, true
}

func inlineMediaURL(data *provider.DataContent, mediaType string) string {
	if data == nil || mediaType == "" {
		return ""
	}
	if data.Base64 != "" {
		return "data:" + mediaType + ";base64," + data.Base64
	}
	if len(data.Bytes) > 0 {
		return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data.Bytes)
	}
	return ""
}

func validBase64(value string) bool {
	if strings.ContainsAny(value, "\r\n") {
		return false
	}
	decoder := base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(value))
	_, err := io.Copy(io.Discard, decoder)
	return err == nil
}

func concreteMediaType(value string) string {
	parsed, _, err := mime.ParseMediaType(strings.ToLower(strings.TrimSpace(value)))
	if err != nil || strings.HasSuffix(parsed, "/*") || mediaKind(parsed) == "" {
		return ""
	}
	return parsed
}

func mediaKind(mediaType string) string {
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		return "image"
	case strings.HasPrefix(mediaType, "video/"):
		return "video"
	default:
		return ""
	}
}

func mediaTypeFromInlineData(data *provider.DataContent) string {
	if data == nil {
		return ""
	}
	if len(data.Bytes) > 0 {
		return mediaTypeFromSniffedBytes(data.Bytes)
	}
	if data.Base64 == "" {
		return ""
	}

	decoder := base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(data.Base64))
	buffer := make([]byte, 512)
	n, err := io.ReadFull(decoder, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return ""
	}
	return mediaTypeFromSniffedBytes(buffer[:n])
}

func mediaTypeFromSniffedBytes(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return concreteMediaType(http.DetectContentType(data))
}

func mediaTypeFromName(name string) string {
	extension := strings.ToLower(path.Ext(strings.TrimSpace(name)))
	switch extension {
	case ".avif":
		return "image/avif"
	case ".gif":
		return "image/gif"
	case ".jpeg", ".jpg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	case ".webp":
		return "image/webp"
	case ".m4v", ".mp4":
		return "video/mp4"
	case ".mpeg", ".mpg":
		return "video/mpeg"
	case ".mov":
		return "video/quicktime"
	case ".ogv":
		return "video/ogg"
	case ".webm":
		return "video/webm"
	default:
		return ""
	}
}
