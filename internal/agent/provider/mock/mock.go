// Package mock provides a scriptable provider.Provider for unit-testing
// agent.Core's tool-use loop without hitting a real LLM API.
package mock

import (
	"context"

	"github.com/mansooranis/kode/internal/agent/provider"
)

// Provider replays a fixed sequence of StreamEvent batches: each call to
// Stream consumes the next batch in Responses (repeating the last one if
// Stream is called more times than there are batches).
type Provider struct {
	Responses [][]provider.StreamEvent
	calls     int

	// Requests records every request Stream was called with, so tests can
	// assert on what the agent core sent (e.g. that a tool_result was fed
	// back correctly).
	Requests []provider.Request
}

func New(responses ...[]provider.StreamEvent) *Provider {
	return &Provider{Responses: responses}
}

func (p *Provider) Name() string           { return "mock" }
func (p *Provider) SupportsTools() bool    { return true }
func (p *Provider) SupportsThinking() bool { return false }

func (p *Provider) Stream(_ context.Context, req provider.Request) (<-chan provider.StreamEvent, error) {
	p.Requests = append(p.Requests, req)

	idx := p.calls
	if idx >= len(p.Responses) {
		idx = len(p.Responses) - 1
	}
	p.calls++

	out := make(chan provider.StreamEvent, len(p.Responses[idx]))
	for _, e := range p.Responses[idx] {
		out <- e
	}
	close(out)
	return out, nil
}

var _ provider.Provider = (*Provider)(nil)
