// Package openai is a minimal second Provider implementation, built during
// Phase 4 (not deferred) specifically to prove provider.Provider isn't
// accidentally shaped around Anthropic's API — it hand-rolls a Chat
// Completions streaming client rather than pulling in a full SDK, since
// proving the interface is the goal here, not production OpenAI support.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mansooranis/kode/internal/agent/provider"
)

type Provider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

func New(apiKey, baseURL, model string) *Provider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o"
	}
	return &Provider{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		httpClient: http.DefaultClient,
	}
}

func (p *Provider) Name() string           { return "openai" }
func (p *Provider) SupportsTools() bool    { return true }
func (p *Provider) SupportsThinking() bool { return false } // Chat Completions has no thinking/effort concept; Request.Effort is ignored.

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolSpec struct {
	Type     string       `json:"type"`
	Function functionSpec `json:"function"`
}

type functionSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Stream   bool          `json:"stream"`
	Messages []chatMessage `json:"messages"`
	Tools    []toolSpec    `json:"tools,omitempty"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func (p *Provider) Stream(ctx context.Context, req provider.Request) (<-chan provider.StreamEvent, error) {
	body := chatRequest{
		Model:    p.model,
		Stream:   true,
		Messages: toOpenAIMessages(req.System, req.Messages),
		Tools:    toOpenAITools(req.Tools),
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("openai: unexpected status %s", resp.Status)
	}

	out := make(chan provider.StreamEvent)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		// index -> tool_call id, so subsequent chunks that only carry an
		// index (no id) can still be attributed to the right tool call.
		toolIDByIndex := map[int]string{}
		started := map[string]bool{}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk streamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]

			if choice.Delta.Content != "" {
				out <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: choice.Delta.Content}
			}

			for _, tc := range choice.Delta.ToolCalls {
				id := tc.ID
				if id == "" {
					id = toolIDByIndex[tc.Index]
				} else {
					toolIDByIndex[tc.Index] = id
				}

				if !started[id] && tc.Function.Name != "" {
					started[id] = true
					out <- provider.StreamEvent{
						Type:    provider.EventToolUseStart,
						ToolUse: &provider.ToolUse{ID: id, Name: tc.Function.Name},
					}
				}
				if tc.Function.Arguments != "" {
					out <- provider.StreamEvent{
						Type:       provider.EventToolUseDelta,
						ToolUse:    &provider.ToolUse{ID: id},
						InputDelta: tc.Function.Arguments,
					}
				}
			}

			if choice.FinishReason != "" {
				reason := choice.FinishReason
				if reason == "tool_calls" {
					reason = "tool_use"
				} else if reason == "stop" {
					reason = "end_turn"
				}
				out <- provider.StreamEvent{Type: provider.EventMessageStop, StopReason: reason}
			}
		}
		if err := scanner.Err(); err != nil {
			out <- provider.StreamEvent{Type: provider.EventError, Err: err}
		}
	}()

	return out, nil
}

func toOpenAIMessages(system string, msgs []provider.Message) []chatMessage {
	out := make([]chatMessage, 0, len(msgs)+1)
	if system != "" {
		out = append(out, chatMessage{Role: "system", Content: system})
	}

	for _, m := range msgs {
		role := "user"
		if m.Role == provider.RoleAssistant {
			role = "assistant"
		}

		var text strings.Builder
		var toolCalls []toolCall
		var toolResults []chatMessage

		for _, c := range m.Content {
			switch c.Type {
			case "text":
				text.WriteString(c.Text)
			case "tool_use":
				toolCalls = append(toolCalls, toolCall{
					ID:   c.ToolUse.ID,
					Type: "function",
					Function: functionCall{
						Name:      c.ToolUse.Name,
						Arguments: string(c.ToolUse.Input),
					},
				})
			case "tool_result":
				toolResults = append(toolResults, chatMessage{
					Role:       "tool",
					Content:    c.ToolResult.Content,
					ToolCallID: c.ToolResult.ToolUseID,
				})
			}
		}

		if text.Len() > 0 || len(toolCalls) > 0 {
			out = append(out, chatMessage{Role: role, Content: text.String(), ToolCalls: toolCalls})
		}
		out = append(out, toolResults...)
	}
	return out
}

func toOpenAITools(tools []provider.ToolDef) []toolSpec {
	if len(tools) == 0 {
		return nil
	}
	out := make([]toolSpec, 0, len(tools))
	for _, t := range tools {
		out = append(out, toolSpec{
			Type: "function",
			Function: functionSpec{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	return out
}

var _ provider.Provider = (*Provider)(nil)
