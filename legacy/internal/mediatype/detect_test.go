package mediatype

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetect(t *testing.T) {
	t.Run("M4A", func(t *testing.T) {
		data := []byte{0x00, 0x00, 0x00, 0x1c, 0x66, 0x74, 0x79, 0x70, 0x4d, 0x34, 0x41, 0x20}
		assert.Equal(t, "audio/mp4", Detect(data, "", "audio"))
		assert.Equal(t, "audio/mp4", Detect(nil, base64.StdEncoding.EncodeToString(data), "audio"))
	})

	t.Run("ID3 scan limit", func(t *testing.T) {
		atLimit := id3MP3(maxID3TagBytes)
		assert.Equal(t, "audio/mpeg", Detect(atLimit, "", "audio"))
		assert.Equal(t, "audio/mpeg", Detect(nil, base64.StdEncoding.EncodeToString(atLimit), "audio"))
	})

	t.Run("ID3 over scan limit", func(t *testing.T) {
		overLimit := id3MP3(maxID3TagBytes + 1)
		assert.Empty(t, Detect(overLimit, "", "audio"))
		assert.Empty(t, Detect(nil, base64.StdEncoding.EncodeToString(overLimit), "audio"))
	})
}

func id3MP3(tagBody int) []byte {
	data := make([]byte, 10+tagBody+2)
	data[0] = 0x49
	data[1] = 0x44
	data[2] = 0x33
	data[6] = byte((tagBody >> 21) & 0x7f)
	data[7] = byte((tagBody >> 14) & 0x7f)
	data[8] = byte((tagBody >> 7) & 0x7f)
	data[9] = byte(tagBody & 0x7f)
	data[10+tagBody] = 0xff
	data[10+tagBody+1] = 0xfb
	return data
}
