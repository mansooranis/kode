// Package agent drives the conversation and tool-use loop against a
// provider.Provider. It has no UI dependency — the chat panel (Phase 5)
// wires into it by reading the channel returned from Send and forwarding
// events into the Bubble Tea program via tea.Program.Send.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mansooranis/kode/internal/agent/mcp"
	"github.com/mansooranis/kode/internal/agent/provider"
	"github.com/mansooranis/kode/internal/agent/skills"
)

// ToolHandler implements one of kode's own tools (as opposed to an
// MCP-sourced one, which agent/mcp.Registry.CallTool handles instead).
type ToolHandler func(ctx context.Context, input json.RawMessage) (provider.ToolResult, error)

type Core struct {
	provider     provider.Provider
	systemPrompt string
	effort       string

	messages []provider.Message

	toolDefs []provider.ToolDef
	tools    map[string]ToolHandler

	mcpRegistry *mcp.Registry
	skillsLib   *skills.Library
}

func NewCore(p provider.Provider, systemPrompt string) *Core {
	return &Core{
		provider:     p,
		systemPrompt: systemPrompt,
		tools:        map[string]ToolHandler{},
	}
}

func (c *Core) SetEffort(effort string) { c.effort = effort }

// RegisterTool adds a native kode tool (e.g. add_annotation, render_diagram)
// to the set offered to the model every turn.
func (c *Core) RegisterTool(def provider.ToolDef, handler ToolHandler) {
	c.toolDefs = append(c.toolDefs, def)
	c.tools[def.Name] = handler
}

// SetMCP wires in a connected MCP client registry; its tools are merged into
// every request alongside native tools, indistinguishable to the model.
func (c *Core) SetMCP(reg *mcp.Registry) {
	c.mcpRegistry = reg
}

// SetSkills wires in a skill library and registers the load_skill tool that
// progressively discloses a skill's full body on demand.
func (c *Core) SetSkills(lib *skills.Library) {
	c.skillsLib = lib
	c.RegisterTool(provider.ToolDef{
		Name:        "load_skill",
		Description: "Load the full instructions for a named skill by its name.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
	}, c.loadSkill)
}

func (c *Core) loadSkill(_ context.Context, input json.RawMessage) (provider.ToolResult, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return provider.ToolResult{}, fmt.Errorf("decode load_skill input: %w", err)
	}
	s, ok := c.skillsLib.Get(args.Name)
	if !ok {
		return provider.ToolResult{Content: fmt.Sprintf("no skill named %q", args.Name), IsError: true}, nil
	}
	return provider.ToolResult{Content: s.Body}, nil
}

// Send appends a user message and drives the full agentic turn — including
// any tool_use round-trips — forwarding every StreamEvent (across however
// many underlying provider.Stream calls that takes) on the returned channel.
// The channel closes when the turn reaches a stop reason other than
// "tool_use", or on error.
func (c *Core) Send(ctx context.Context, userText string) <-chan provider.StreamEvent {
	c.messages = append(c.messages, provider.Message{
		Role:    provider.RoleUser,
		Content: []provider.ContentBlock{provider.TextBlock(userText)},
	})

	out := make(chan provider.StreamEvent)
	go c.runLoop(ctx, out)
	return out
}

type pendingToolUse struct {
	name  string
	input strings.Builder
}

func (c *Core) runLoop(ctx context.Context, out chan<- provider.StreamEvent) {
	defer close(out)

	for {
		req := provider.Request{
			System:   c.effectiveSystem(),
			Messages: c.messages,
			Tools:    c.allTools(),
			Effort:   c.effort,
		}

		events, err := c.provider.Stream(ctx, req)
		if err != nil {
			out <- provider.StreamEvent{Type: provider.EventError, Err: err}
			return
		}

		var textBuf strings.Builder
		toolUses := map[string]*pendingToolUse{}
		var toolOrder []string
		stopReason := ""

		for e := range events {
			switch e.Type {
			case provider.EventTextDelta:
				textBuf.WriteString(e.TextDelta)
			case provider.EventToolUseStart:
				toolUses[e.ToolUse.ID] = &pendingToolUse{name: e.ToolUse.Name}
				toolOrder = append(toolOrder, e.ToolUse.ID)
			case provider.EventToolUseDelta:
				if p, ok := toolUses[e.ToolUse.ID]; ok {
					p.input.WriteString(e.InputDelta)
				}
			case provider.EventMessageStop:
				stopReason = e.StopReason
			case provider.EventError:
				out <- e
				return
			}
			out <- e
		}

		var assistantBlocks []provider.ContentBlock
		if textBuf.Len() > 0 {
			assistantBlocks = append(assistantBlocks, provider.TextBlock(textBuf.String()))
		}
		for _, id := range toolOrder {
			p := toolUses[id]
			assistantBlocks = append(assistantBlocks, provider.ToolUseBlock(provider.ToolUse{
				ID:    id,
				Name:  p.name,
				Input: json.RawMessage(p.input.String()),
			}))
		}
		if len(assistantBlocks) > 0 {
			c.messages = append(c.messages, provider.Message{Role: provider.RoleAssistant, Content: assistantBlocks})
		}

		if stopReason != "tool_use" || len(toolOrder) == 0 {
			return
		}

		var resultBlocks []provider.ContentBlock
		for _, id := range toolOrder {
			p := toolUses[id]
			result := c.executeTool(ctx, p.name, json.RawMessage(p.input.String()))
			result.ToolUseID = id
			resultBlocks = append(resultBlocks, provider.ToolResultBlock(result))
		}
		c.messages = append(c.messages, provider.Message{Role: provider.RoleUser, Content: resultBlocks})
	}
}

func (c *Core) executeTool(ctx context.Context, name string, input json.RawMessage) provider.ToolResult {
	if h, ok := c.tools[name]; ok {
		result, err := h(ctx, input)
		if err != nil {
			return provider.ToolResult{Content: err.Error(), IsError: true}
		}
		return result
	}
	if c.mcpRegistry != nil && c.mcpRegistry.Owns(name) {
		result, err := c.mcpRegistry.CallTool(ctx, name, input)
		if err != nil {
			return provider.ToolResult{Content: err.Error(), IsError: true}
		}
		return result
	}
	return provider.ToolResult{Content: fmt.Sprintf("unknown tool %q", name), IsError: true}
}

func (c *Core) allTools() []provider.ToolDef {
	all := make([]provider.ToolDef, 0, len(c.toolDefs))
	all = append(all, c.toolDefs...)
	if c.mcpRegistry != nil {
		all = append(all, c.mcpRegistry.ToolDefs()...)
	}
	return all
}

// effectiveSystem appends skill name+description summaries to the base
// system prompt — progressive disclosure keeps the full skill body out of
// context until load_skill is called.
func (c *Core) effectiveSystem() string {
	if c.skillsLib == nil {
		return c.systemPrompt
	}
	summaries := c.skillsLib.Summaries()
	if len(summaries) == 0 {
		return c.systemPrompt
	}

	var b strings.Builder
	b.WriteString(c.systemPrompt)
	b.WriteString("\n\nAvailable skills (use load_skill to load one's full instructions):\n")
	for _, s := range summaries {
		fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
	}
	return b.String()
}
