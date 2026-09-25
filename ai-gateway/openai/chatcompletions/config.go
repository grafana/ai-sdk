// Package chatcompletions adapts the Gateway's bounded OpenAI Chat Completions
// subset to and from the canonical provider domain.
package chatcompletions

import (
	"errors"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

// Limits bounds one request; aggregate admission remains the host's responsibility.
type Limits struct {
	RequestBytes, ResponseBytes, FrameBytes    int64
	StreamParts                                int
	ModelDuration, IdleDuration, DrainDuration time.Duration
}

// Requirements describes canonical behavior requested by the public protocol
// that is not represented directly by provider.CallOptions.
type Requirements struct {
	History           bool
	JSONOutput        bool
	StrictJSONOutput  bool
	HasParallelTools  bool
	ParallelToolCalls bool
}

// RequestPolicy validates a canonical request against a resolved route's
// capabilities and may add provider-owned transport options. The adapter never
// inspects provider or backend identity.
type RequestPolicy func(*provider.CallOptions, Requirements) error

// Config supplies the shared catalog and canonical-route policies, copied by New.
type Config struct {
	Resolver catalog.ModelResolver
	Policies map[string]RequestPolicy
	Limits   Limits
}

type handler struct {
	resolver catalog.ModelResolver
	policies map[string]RequestPolicy
	limits   Limits
}

// New constructs a Chat Completions adapter without importing another protocol's codecs.
func New(c Config) (*handler, error) {
	l := c.Limits
	if c.Resolver == nil || l.RequestBytes <= 0 || l.RequestBytes >= 1<<62 || l.ResponseBytes < 512 || l.ResponseBytes >= 1<<62 || l.FrameBytes < 512 || l.FrameBytes >= 1<<62 || l.StreamParts <= 0 || l.ModelDuration <= 0 || l.IdleDuration <= 0 || l.IdleDuration > l.ModelDuration || l.DrainDuration <= 0 {
		return nil, errors.New("chat completions: invalid configuration")
	}
	p := make(map[string]RequestPolicy, len(c.Policies))
	for id, policy := range c.Policies {
		if policy == nil {
			return nil, errors.New("chat completions: invalid policy")
		}
		p[id] = policy
	}
	return &handler{resolver: c.Resolver, policies: p, limits: l}, nil
}
