package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mansooranis/kode/internal/agent/provider"
	"github.com/mansooranis/kode/internal/agent/provider/mock"
)

func drain(ch <-chan provider.StreamEvent) []provider.StreamEvent {
	var events []provider.StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func TestSendWithoutTools(t *testing.T) {
	p := mock.New([]provider.StreamEvent{
		{Type: provider.EventTextDelta, TextDelta: "Hello, "},
		{Type: provider.EventTextDelta, TextDelta: "world."},
		{Type: provider.EventMessageStop, StopReason: "end_turn"},
	})
	core := NewCore(p, "you are kode")

	events := drain(core.Send(context.Background(), "hi"))

	var text string
	for _, e := range events {
		if e.Type == provider.EventTextDelta {
			text += e.TextDelta
		}
	}
	if text != "Hello, world." {
		t.Fatalf("got text %q, want %q", text, "Hello, world.")
	}
	if len(p.Requests) != 1 {
		t.Fatalf("expected 1 provider call, got %d", len(p.Requests))
	}
}

func TestSendRoundTripsToolUse(t *testing.T) {
	p := mock.New(
		[]provider.StreamEvent{
			{Type: provider.EventToolUseStart, ToolUse: &provider.ToolUse{ID: "t1", Name: "echo"}},
			{Type: provider.EventToolUseDelta, ToolUse: &provider.ToolUse{ID: "t1"}, InputDelta: `{"msg":"hi"}`},
			{Type: provider.EventMessageStop, StopReason: "tool_use"},
		},
		[]provider.StreamEvent{
			{Type: provider.EventTextDelta, TextDelta: "done"},
			{Type: provider.EventMessageStop, StopReason: "end_turn"},
		},
	)
	core := NewCore(p, "you are kode")

	var gotInput json.RawMessage
	core.RegisterTool(provider.ToolDef{Name: "echo", Description: "echoes input"}, func(_ context.Context, input json.RawMessage) (provider.ToolResult, error) {
		gotInput = input
		return provider.ToolResult{Content: "echoed"}, nil
	})

	drain(core.Send(context.Background(), "call echo"))

	if string(gotInput) != `{"msg":"hi"}` {
		t.Fatalf("tool got input %q", gotInput)
	}
	if len(p.Requests) != 2 {
		t.Fatalf("expected 2 provider calls (initial + after tool result), got %d", len(p.Requests))
	}

	// The second request must include the tool_result fed back to the model.
	second := p.Requests[1]
	last := second.Messages[len(second.Messages)-1]
	if last.Role != provider.RoleUser || len(last.Content) != 1 || last.Content[0].Type != "tool_result" {
		t.Fatalf("expected last message to be a tool_result, got %+v", last)
	}
	if last.Content[0].ToolResult.Content != "echoed" {
		t.Fatalf("tool_result content = %q, want %q", last.Content[0].ToolResult.Content, "echoed")
	}
}

func TestUnknownToolReturnsErrorResult(t *testing.T) {
	p := mock.New(
		[]provider.StreamEvent{
			{Type: provider.EventToolUseStart, ToolUse: &provider.ToolUse{ID: "t1", Name: "nope"}},
			{Type: provider.EventMessageStop, StopReason: "tool_use"},
		},
		[]provider.StreamEvent{
			{Type: provider.EventMessageStop, StopReason: "end_turn"},
		},
	)
	core := NewCore(p, "you are kode")

	drain(core.Send(context.Background(), "call nope"))

	second := p.Requests[1]
	last := second.Messages[len(second.Messages)-1]
	if !last.Content[0].ToolResult.IsError {
		t.Fatalf("expected IsError=true for unknown tool, got %+v", last.Content[0].ToolResult)
	}
}
