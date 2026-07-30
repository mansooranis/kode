// Package anthropic is kode's default Provider implementation, backed by
// the official Anthropic Go SDK.
package anthropic

import (
	"context"
	"encoding/json"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/mansooranis/kode/internal/agent/provider"
)

type Provider struct {
	client sdk.Client
	model  sdk.Model
}

// New builds an Anthropic provider. apiKey may be empty to fall back to the
// SDK's default environment resolution (ANTHROPIC_API_KEY).
func New(apiKey, model string) *Provider {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if model == "" {
		model = sdk.ModelClaudeOpus5
	}
	return &Provider{
		client: sdk.NewClient(opts...),
		model:  sdk.Model(model),
	}
}

func (p *Provider) Name() string           { return "anthropic" }
func (p *Provider) SupportsTools() bool    { return true }
func (p *Provider) SupportsThinking() bool { return true }

func (p *Provider) Stream(ctx context.Context, req provider.Request) (<-chan provider.StreamEvent, error) {
	params := sdk.MessageNewParams{
		Model:     p.model,
		MaxTokens: maxTokens(req.MaxTokens),
		Messages:  toSDKMessages(req.Messages),
	}
	if req.System != "" {
		params.System = []sdk.TextBlockParam{{Text: req.System}}
	}
	if len(req.Tools) > 0 {
		params.Tools = toSDKTools(req.Tools)
	}
	if req.Effort != "" {
		params.OutputConfig.Effort = sdk.OutputConfigEffort(req.Effort)
		params.Thinking = sdk.ThinkingConfigParamUnion{OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{}}
	}

	stream := p.client.Messages.NewStreaming(ctx, params)

	out := make(chan provider.StreamEvent)
	go func() {
		defer close(out)

		var currentToolID string
		for stream.Next() {
			event := stream.Current()
			switch event.Type {
			case "content_block_start":
				block := event.ContentBlock
				if block.Type == "tool_use" {
					currentToolID = block.ID
					out <- provider.StreamEvent{
						Type: provider.EventToolUseStart,
						ToolUse: &provider.ToolUse{
							ID:   block.ID,
							Name: block.Name,
						},
					}
				}
			case "content_block_delta":
				delta := event.Delta
				switch delta.Type {
				case "text_delta":
					out <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: delta.Text}
				case "input_json_delta":
					out <- provider.StreamEvent{
						Type:       provider.EventToolUseDelta,
						ToolUse:    &provider.ToolUse{ID: currentToolID},
						InputDelta: delta.PartialJSON,
					}
				}
			case "content_block_stop":
				if currentToolID != "" {
					out <- provider.StreamEvent{
						Type:    provider.EventToolUseStop,
						ToolUse: &provider.ToolUse{ID: currentToolID},
					}
					currentToolID = ""
				}
			case "message_delta":
				if reason := event.Delta.StopReason; reason != "" {
					out <- provider.StreamEvent{Type: provider.EventMessageStop, StopReason: string(reason)}
				}
			}
		}
		if err := stream.Err(); err != nil {
			out <- provider.StreamEvent{Type: provider.EventError, Err: err}
		}
	}()

	return out, nil
}

func maxTokens(requested int) int64 {
	if requested > 0 {
		return int64(requested)
	}
	return 4096
}

func toSDKMessages(msgs []provider.Message) []sdk.MessageParam {
	out := make([]sdk.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		blocks := make([]sdk.ContentBlockParamUnion, 0, len(m.Content))
		for _, c := range m.Content {
			switch c.Type {
			case "text":
				blocks = append(blocks, sdk.NewTextBlock(c.Text))
			case "tool_use":
				blocks = append(blocks, sdk.ContentBlockParamUnion{
					OfToolUse: &sdk.ToolUseBlockParam{
						ID:    c.ToolUse.ID,
						Name:  c.ToolUse.Name,
						Input: json.RawMessage(c.ToolUse.Input),
					},
				})
			case "tool_result":
				blocks = append(blocks, sdk.NewToolResultBlock(c.ToolResult.ToolUseID, c.ToolResult.Content, c.ToolResult.IsError))
			}
		}
		if m.Role == provider.RoleAssistant {
			out = append(out, sdk.NewAssistantMessage(blocks...))
		} else {
			out = append(out, sdk.NewUserMessage(blocks...))
		}
	}
	return out
}

func toSDKTools(tools []provider.ToolDef) []sdk.ToolUnionParam {
	out := make([]sdk.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		var schema sdk.ToolInputSchemaParam
		if err := json.Unmarshal(t.InputSchema, &schema); err != nil {
			// Fall back to an empty object schema rather than dropping the
			// tool entirely; a malformed schema will surface as a model-side
			// tool-use error instead of silently vanishing.
			schema = sdk.ToolInputSchemaParam{}
		}
		tool := sdk.ToolUnionParamOfTool(schema, t.Name)
		if tool.OfTool != nil {
			tool.OfTool.Description = sdk.String(t.Description)
		}
		out = append(out, tool)
	}
	return out
}

var _ provider.Provider = (*Provider)(nil)
