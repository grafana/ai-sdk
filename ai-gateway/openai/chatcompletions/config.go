// Package chatcompletions implements the Gateway's bounded native Chat Completions subset.
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

// Backend is the immutable native mapping policy selected during host composition.
type Backend string

const (
	BackendAnthropic  Backend = "anthropic"
	BackendResponses  Backend = "responses"
	BackendReasoning  Backend = "responses-reasoning"
	BackendCompatible Backend = "compatible"
	BackendFallback   Backend = "fallback-text"
)

// Config supplies the shared catalog and canonical-route policies, copied by New.
type Config struct {
	Resolver catalog.ModelResolver
	Policies map[string]Backend
	Limits   Limits
}

type handler struct {
	resolver catalog.ModelResolver
	policies map[string]Backend
	limits   Limits
}

// New constructs a native adapter without importing another protocol's codecs.
func New(c Config) (*handler, error) {
	l := c.Limits
	if c.Resolver == nil || l.RequestBytes <= 0 || l.RequestBytes >= 1<<62 || l.ResponseBytes < 512 || l.ResponseBytes >= 1<<62 || l.FrameBytes < 512 || l.FrameBytes >= 1<<62 || l.StreamParts <= 0 || l.ModelDuration <= 0 || l.IdleDuration <= 0 || l.IdleDuration > l.ModelDuration || l.DrainDuration <= 0 {
		return nil, errors.New("chat completions: invalid configuration")
	}
	p := make(map[string]Backend, len(c.Policies))
	for id, policy := range c.Policies {
		switch policy {
		case BackendAnthropic, BackendResponses, BackendReasoning, BackendCompatible, BackendFallback:
		default:
			return nil, errors.New("chat completions: invalid policy")
		}
		p[id] = policy
	}
	return &handler{resolver: c.Resolver, policies: p, limits: l}, nil
}
