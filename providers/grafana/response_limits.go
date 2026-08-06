package grafana

import (
	"errors"
	"io"
)

var errResponseTooLarge = errors.New("grafana: response body exceeds configured limit")

func readResponseWithinLimit(reader io.Reader, limit int64) ([]byte, error) {
	limited := &io.LimitedReader{R: reader, N: limit}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	var extra [1]byte
	count, extraErr := io.ReadFull(reader, extra[:])
	if count > 0 {
		return data, errResponseTooLarge
	}
	if extraErr != nil && !errors.Is(extraErr, io.EOF) && !errors.Is(extraErr, io.ErrUnexpectedEOF) {
		return data, extraErr
	}
	return data, nil
}
