// Package chatcompletions adapts the Gateway's bounded OpenAI Chat Completions
// subset to and from the canonical provider domain.
package chatcompletions

import (
	"errors"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
)

// Limits bounds one request; aggregate admission remains the host's responsibility.
type Limits struct {
	RequestBytes, ResponseBytes, FrameBytes    int64
	StreamParts                                int
	ModelDuration, IdleDuration, DrainDuration time.Duration
}

// Config supplies the shared catalog and request limits.
type Config struct {
	Resolver catalog.ModelResolver
	Limits   Limits
}

type handler struct {
	resolver catalog.ModelResolver
	limits   Limits
}

// New constructs a Chat Completions adapter without importing another protocol's codecs.
func New(c Config) (*handler, error) {
	l := c.Limits
	if c.Resolver == nil || l.RequestBytes <= 0 || l.RequestBytes >= 1<<62 || l.ResponseBytes < 512 || l.ResponseBytes >= 1<<62 || l.FrameBytes < 512 || l.FrameBytes >= 1<<62 || l.StreamParts <= 0 || l.ModelDuration <= 0 || l.IdleDuration <= 0 || l.IdleDuration > l.ModelDuration || l.DrainDuration <= 0 {
		return nil, errors.New("chat completions: invalid configuration")
	}
	return &handler{resolver: c.Resolver, limits: l}, nil
}
