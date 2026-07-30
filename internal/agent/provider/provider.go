// Package provider defines the pluggable boundary between kode's agent core
// and whichever LLM backend answers a request. internal/agent/core drives
// the conversation and tool-use loop entirely against this interface, so
// swapping providers (Anthropic, OpenAI, ...) never touches agent logic.
package provider

import (
	"context"
	"encoding/json"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ToolUse is a model-requested call to a tool, identified by ToolUseID so the
// eventual result can be matched back to it.
type ToolUse struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult is fed back into the conversation as the outcome of a ToolUse.
type ToolResult struct {
	ToolUseID string
	Content   string
	IsError   bool
}

// ContentBlock is one piece of a Message: exactly one of Text, ToolUse, or
// ToolResult is set, matching Type.
type ContentBlock struct {
	Type       string // "text" | "tool_use" | "tool_result"
	Text       string
	ToolUse    *ToolUse
	ToolResult *ToolResult
}

func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

func ToolUseBlock(u ToolUse) ContentBlock {
	return ContentBlock{Type: "tool_use", ToolUse: &u}
}

func ToolResultBlock(r ToolResult) ContentBlock {
	return ContentBlock{Type: "tool_result", ToolResult: &r}
}

type Message struct {
	Role    Role
	Content []ContentBlock
}

// ToolDef is the shape shared by kode's native tools and MCP-sourced tools —
// from the model's point of view there is no difference between the two.
type ToolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

type Request struct {
	System    string
	Messages  []Message
	Tools     []ToolDef
	Effort    string // "low"|"medium"|"high"|"xhigh"|"max"; ignored by providers that don't support it
	MaxTokens int
}

type StreamEventType string

const (
	EventTextDelta    StreamEventType = "text_delta"
	EventToolUseStart StreamEventType = "tool_use_start"
	EventToolUseDelta StreamEventType = "tool_use_delta"
	EventToolUseStop  StreamEventType = "tool_use_stop"
	EventMessageStop  StreamEventType = "message_stop"
	EventError        StreamEventType = "error"
)

// StreamEvent is one incremental update from a Provider.Stream call. The
// caller (agent.Core, ultimately the chat panel bubble) forwards these as
// they arrive — providers never buffer a full response before yielding.
type StreamEvent struct {
	Type StreamEventType

	TextDelta string

	// Set on EventToolUseStart (ID, Name); Input accumulates as
	// EventToolUseDelta events arrive with InputDelta fragments.
	ToolUse    *ToolUse
	InputDelta string

	StopReason string
	Err        error
}

// Provider is the pluggable LLM boundary. One implementation per backend.
type Provider interface {
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
	Name() string
	SupportsTools() bool
	SupportsThinking() bool
}
