package chatcompletions

import (
	"encoding/base64"
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

const (
	kindImage kind = "image_url"
	kindAudio kind = "input_audio"
	kindFile  kind = "file"
)

type userPart struct {
	Breakpoint *compatible.ChatCacheBreakpoint `json:"prompt_cache_breakpoint"`
	Type       kind                            `json:"type"`
	Text       *string                         `json:"text"`
	Image      *struct {
		URL    string                 `json:"url"`
		Detail compatible.ImageDetail `json:"detail"`
	} `json:"image_url"`
	Audio *struct {
		Data   string `json:"data"`
		Format string `json:"format"`
	} `json:"input_audio"`
	File *struct {
		ID       *string `json:"file_id"`
		Data     *string `json:"file_data"`
		Filename *string `json:"filename"`
	} `json:"file"`
}

func userContent(raw json.RawMessage) ([]provider.ContentPart, error) {
	var parts []userPart
	if decodeStrict(raw, &parts) != nil {
		return nil, errRequest
	}
	result := make([]provider.ContentPart, 0, len(parts))
	for _, p := range parts {
		if p.Breakpoint.Validate() != nil {
			return nil, errRequest
		}
		fields := 0
		for _, set := range []bool{p.Text != nil, p.Image != nil, p.Audio != nil, p.File != nil} {
			if set {
				fields++
			}
		}
		if fields != 1 {
			return nil, errRequest
		}
		var part provider.ContentPart
		switch {
		case p.Type == kindText && p.Text != nil:
			part = provider.TextPart(*p.Text)
		case p.Type == kindImage && p.Image != nil:
			if p.Image.Detail != "" && p.Image.Detail != compatible.ImageDetailAuto && p.Image.Detail != compatible.ImageDetailLow && p.Image.Detail != compatible.ImageDetailHigh && p.Image.Detail != compatible.ImageDetailOriginal {
				return nil, errRequest
			}
			media := "image/*"
			data := provider.URLDataContent(p.Image.URL)
			if strings.HasPrefix(p.Image.URL, "data:") {
				var err error
				media, data, err = inlineData(p.Image.URL, "")
				if err != nil || !strings.HasPrefix(media, "image/") {
					return nil, errRequest
				}
			} else {
				u, err := url.Parse(p.Image.URL)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
					return nil, errRequest
				}
			}
			part = provider.FilePart(media, data)
			if p.Image.Detail != "" {
				raw, _ := json.Marshal(map[string]compatible.ImageDetail{"imageDetail": p.Image.Detail})
				part.ProviderOptions = provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: raw}}
			}
		case p.Type == kindAudio && p.Audio != nil:
			if p.Audio.Format != "wav" && p.Audio.Format != "mp3" {
				return nil, errRequest
			}
			decoded, err := base64.StdEncoding.DecodeString(p.Audio.Data)
			if err != nil || len(decoded) == 0 {
				return nil, errRequest
			}
			media := "audio/wav"
			if p.Audio.Format == "mp3" {
				media = "audio/mpeg"
			}
			part = provider.FilePart(media, provider.Base64DataContent(p.Audio.Data))
		case p.Type == kindFile && p.File != nil:
			if (p.File.ID == nil) == (p.File.Data == nil) {
				return nil, errRequest
			}
			if p.File.ID != nil {
				if *p.File.ID == "" {
					return nil, errRequest
				}
				reference, _ := json.Marshal(map[string]string{"openai": *p.File.ID})
				part = provider.FilePart("application/pdf", provider.ReferenceDataContent(reference))
			} else {
				filename := ""
				if p.File.Filename != nil {
					filename = *p.File.Filename
				}
				media, data, err := inlineData(*p.File.Data, filename)
				if err != nil {
					return nil, errRequest
				}
				part = provider.FilePart(media, data)
			}
			part.Filename = p.File.Filename
		default:
			return nil, errRequest
		}
		result = append(result, part)
	}
	return result, nil
}

func inlineData(raw, filename string) (string, provider.DataContent, error) {
	media := ""
	data := raw
	if strings.HasPrefix(raw, "data:") {
		header, encoded, ok := strings.Cut(strings.TrimPrefix(raw, "data:"), ",")
		if !ok || !strings.HasSuffix(header, ";base64") {
			return "", provider.DataContent{}, errRequest
		}
		var err error
		media, _, err = mime.ParseMediaType(strings.TrimSuffix(header, ";base64"))
		if err != nil {
			return "", provider.DataContent{}, errRequest
		}
		data = encoded
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil || len(decoded) == 0 {
		return "", provider.DataContent{}, errRequest
	}
	if media == "" {
		media = mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
		if media == "" {
			media = http.DetectContentType(decoded)
		}
		media, _, err = mime.ParseMediaType(media)
		if err != nil {
			return "", provider.DataContent{}, errRequest
		}
	}
	return media, provider.Base64DataContent(data), nil
}

func translateMedia(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
	options.Prompt = slices.Clone(options.Prompt)
	namespace := string(target.Type)
	if target.Type == execution.TargetOpenAICompatible {
		namespace, _, _ = strings.Cut(target.ProviderName, ".")
		namespace = strings.TrimSpace(namespace)
		if namespace == "" {
			namespace = "openai-compatible"
		}
	}
	for i, message := range options.Prompt {
		options.Prompt[i].Content = slices.Clone(message.Content)
		for j, part := range message.Content {
			if part.Type != provider.ContentPartTypeFile || part.Data == nil {
				continue
			}
			if part.Data.IsReference() {
				var reference map[string]string
				if json.Unmarshal(part.Data.Reference, &reference) != nil {
					return provider.CallOptions{}, errRequest
				}
				raw, _ := json.Marshal(map[string]string{namespace: reference["openai"]})
				data := provider.ReferenceDataContent(raw)
				part.Data = &data
			} else if !supportedMedia(target.Type, part.MediaType) {
				return provider.CallOptions{}, provider.ErrUnsupportedFunctionality
			}
			if target.Type == execution.TargetOpenAICompatible {
				detail, _, err := provider.ResolveOption[compatible.ImageOptions](part.ProviderOptions, "openai")
				if err != nil {
					return provider.CallOptions{}, errRequest
				}
				if detail.Detail != "" {
					part.ProviderOptions = provider.BuildProviderOptions(detail)
				}
			}
			options.Prompt[i].Content[j] = part
		}
	}
	return options, nil
}

func supportedMedia(target execution.TargetType, media string) bool {
	switch target {
	case execution.TargetOpenAICompatible:
		return strings.HasPrefix(media, "image/") || media == "application/pdf" || strings.HasPrefix(media, "text/") || media == "audio/wav" || media == "audio/mpeg"
	case execution.TargetOpenAI:
		return strings.HasPrefix(media, "image/") || media == "application/pdf"
	case execution.TargetAnthropic:
		return media == "image/*" || media == "image/jpeg" || media == "image/png" || media == "image/gif" || media == "image/webp" || media == "application/pdf" || media == "text/plain"
	}
	return false
}
