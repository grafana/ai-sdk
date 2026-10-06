package mediatype

import "encoding/base64"

const (
	defaultSniffBytes = 18
	maxSignatureBytes = 12
	maxID3TagBytes    = 128 * 1024
	id3ScanBytes      = maxID3TagBytes + maxSignatureBytes
)

type signature struct {
	mediaType string
	prefix    []int
}

// Detect identifies the media type from a bounded prefix of raw or base64 data.
func Detect(data []byte, encoded, topLevel string) string {
	bytes := decodePrefix(data, encoded, defaultSniffBytes)
	if hasID3(bytes) {
		bytes = stripID3(decodePrefix(data, encoded, id3ScanBytes))
	}
	for _, candidate := range signatures(topLevel) {
		if hasSignature(bytes, candidate.prefix) {
			return candidate.mediaType
		}
	}
	return ""
}

func decodePrefix(data []byte, encoded string, maxBytes int) []byte {
	if len(data) > 0 {
		if len(data) > maxBytes {
			return data[:maxBytes]
		}
		return data
	}
	if encoded == "" {
		return nil
	}
	maxChars := ((maxBytes + 2) / 3) * 4
	if len(encoded) > maxChars {
		encoded = encoded[:maxChars]
	}
	bytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		bytes, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return nil
		}
	}
	if len(bytes) > maxBytes {
		return bytes[:maxBytes]
	}
	return bytes
}

func hasID3(data []byte) bool {
	return len(data) > 10 && data[0] == 0x49 && data[1] == 0x44 && data[2] == 0x33
}

func stripID3(data []byte) []byte {
	size := (int(data[6]&0x7f) << 21) |
		(int(data[7]&0x7f) << 14) |
		(int(data[8]&0x7f) << 7) |
		int(data[9]&0x7f)
	offset := size + 10
	if offset > len(data) {
		return nil
	}
	return data[offset:]
}

func signatures(topLevel string) []signature {
	switch topLevel {
	case "image":
		return []signature{
			{mediaType: "image/gif", prefix: []int{0x47, 0x49, 0x46}},
			{mediaType: "image/png", prefix: []int{0x89, 0x50, 0x4e, 0x47}},
			{mediaType: "image/jpeg", prefix: []int{0xff, 0xd8}},
			{mediaType: "image/webp", prefix: []int{0x52, 0x49, 0x46, 0x46, -1, -1, -1, -1, 0x57, 0x45, 0x42, 0x50}},
			{mediaType: "image/bmp", prefix: []int{0x42, 0x4d}},
			{mediaType: "image/tiff", prefix: []int{0x49, 0x49, 0x2a, 0x00}},
			{mediaType: "image/tiff", prefix: []int{0x4d, 0x4d, 0x00, 0x2a}},
			{mediaType: "image/avif", prefix: []int{0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70, 0x61, 0x76, 0x69, 0x66}},
			{mediaType: "image/heic", prefix: []int{0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70, 0x68, 0x65, 0x69, 0x63}},
		}
	case "application":
		return []signature{{mediaType: "application/pdf", prefix: []int{0x25, 0x50, 0x44, 0x46}}}
	case "audio":
		return []signature{
			{mediaType: "audio/mpeg", prefix: []int{0xff, 0xfb}},
			{mediaType: "audio/mpeg", prefix: []int{0xff, 0xfa}},
			{mediaType: "audio/mpeg", prefix: []int{0xff, 0xf3}},
			{mediaType: "audio/mpeg", prefix: []int{0xff, 0xf2}},
			{mediaType: "audio/mpeg", prefix: []int{0xff, 0xe3}},
			{mediaType: "audio/mpeg", prefix: []int{0xff, 0xe2}},
			{mediaType: "audio/wav", prefix: []int{0x52, 0x49, 0x46, 0x46, -1, -1, -1, -1, 0x57, 0x41, 0x56, 0x45}},
			{mediaType: "audio/ogg", prefix: []int{0x4f, 0x67, 0x67, 0x53}},
			{mediaType: "audio/flac", prefix: []int{0x66, 0x4c, 0x61, 0x43}},
			{mediaType: "audio/aac", prefix: []int{0x40, 0x15, 0x00, 0x00}},
			{mediaType: "audio/webm", prefix: []int{0x1a, 0x45, 0xdf, 0xa3}},
			{mediaType: "audio/mp4", prefix: []int{0x00, 0x00, 0x00, -1, 0x66, 0x74, 0x79, 0x70}},
		}
	case "video":
		return []signature{
			{mediaType: "video/mp4", prefix: []int{0x00, 0x00, 0x00, -1, 0x66, 0x74, 0x79, 0x70}},
			{mediaType: "video/webm", prefix: []int{0x1a, 0x45, 0xdf, 0xa3}},
			{mediaType: "video/quicktime", prefix: []int{0x00, 0x00, 0x00, 0x14, 0x66, 0x74, 0x79, 0x70, 0x71, 0x74}},
			{mediaType: "video/x-msvideo", prefix: []int{0x52, 0x49, 0x46, 0x46}},
		}
	default:
		return nil
	}
}

func hasSignature(data []byte, prefix []int) bool {
	if len(data) < len(prefix) {
		return false
	}
	for i, value := range prefix {
		if value >= 0 && data[i] != byte(value) {
			return false
		}
	}
	return true
}
