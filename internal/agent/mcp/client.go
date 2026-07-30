// Package mcp is kode's MCP CLIENT: it connects OUT to external MCP servers
// configured under [[mcp.servers]] and exposes their tools to agent.Core
// alongside kode's own native tools. This is distinct from internal/mcpserver,
// which hosts kode's own MCP SERVER for external callers like Claude Code.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mansooranis/kode/internal/agent/provider"
	"github.com/mansooranis/kode/internal/config"
)

const namespaceSep = "__"

type server struct {
	name    string
	session *sdk.ClientSession
	tools   map[string]sdk.Tool // keyed by original (non-namespaced) tool name
}

// Registry discovers tools across all configured MCP servers and routes
// tool_use calls back to whichever server owns the tool. A server that fails
// to connect or list tools is skipped with a warning, not a fatal error —
// kode's other features must keep working with zero or partially-broken MCP
// servers configured.
type Registry struct {
	servers []*server
}

func Connect(ctx context.Context, servers []config.MCPServer) *Registry {
	reg := &Registry{}

	for _, cfg := range servers {
		transport, err := buildTransport(cfg)
		if err != nil {
			log.Printf("kode: mcp server %q: %v", cfg.Name, err)
			continue
		}

		client := sdk.NewClient(&sdk.Implementation{Name: "kode", Version: "0.1.0"}, nil)
		session, err := client.Connect(ctx, transport, nil)
		if err != nil {
			log.Printf("kode: mcp server %q: connect failed: %v", cfg.Name, err)
			continue
		}

		res, err := session.ListTools(ctx, &sdk.ListToolsParams{})
		if err != nil {
			log.Printf("kode: mcp server %q: list tools failed: %v", cfg.Name, err)
			session.Close()
			continue
		}

		srv := &server{name: cfg.Name, session: session, tools: map[string]sdk.Tool{}}
		for _, t := range res.Tools {
			srv.tools[t.Name] = *t
		}
		reg.servers = append(reg.servers, srv)
	}

	return reg
}

func buildTransport(cfg config.MCPServer) (sdk.Transport, error) {
	switch cfg.Transport {
	case "stdio", "":
		if cfg.Command == "" {
			return nil, fmt.Errorf("stdio transport requires a command")
		}
		return &sdk.CommandTransport{Command: exec.Command(cfg.Command, cfg.Args...)}, nil
	case "sse":
		if cfg.URL == "" {
			return nil, fmt.Errorf("sse transport requires a url")
		}
		return &sdk.SSEClientTransport{Endpoint: cfg.URL}, nil
	case "http":
		if cfg.URL == "" {
			return nil, fmt.Errorf("http transport requires a url")
		}
		return &sdk.StreamableClientTransport{Endpoint: cfg.URL}, nil
	default:
		return nil, fmt.Errorf("unknown transport %q", cfg.Transport)
	}
}

// ToolDefs returns every discovered tool across all connected servers,
// namespaced as "<server>__<tool>" so two servers can't collide.
func (r *Registry) ToolDefs() []provider.ToolDef {
	var defs []provider.ToolDef
	for _, srv := range r.servers {
		for name, t := range srv.tools {
			schema, err := json.Marshal(t.InputSchema)
			if err != nil {
				schema = json.RawMessage(`{"type":"object"}`)
			}
			defs = append(defs, provider.ToolDef{
				Name:        srv.name + namespaceSep + name,
				Description: t.Description,
				InputSchema: schema,
			})
		}
	}
	return defs
}

// CallTool routes a namespaced tool_use call to the owning server.
func (r *Registry) CallTool(ctx context.Context, namespacedName string, input json.RawMessage) (provider.ToolResult, error) {
	serverName, toolName, ok := strings.Cut(namespacedName, namespaceSep)
	if !ok {
		return provider.ToolResult{}, fmt.Errorf("not an mcp tool: %q", namespacedName)
	}

	for _, srv := range r.servers {
		if srv.name != serverName {
			continue
		}
		var args any
		if len(input) > 0 {
			if err := json.Unmarshal(input, &args); err != nil {
				return provider.ToolResult{}, fmt.Errorf("decode tool input: %w", err)
			}
		}

		res, err := srv.session.CallTool(ctx, &sdk.CallToolParams{Name: toolName, Arguments: args})
		if err != nil {
			return provider.ToolResult{}, err
		}
		return provider.ToolResult{Content: contentToText(res.Content), IsError: res.IsError}, nil
	}

	return provider.ToolResult{}, fmt.Errorf("no connected mcp server named %q", serverName)
}

// Owns reports whether a tool name belongs to this registry (vs. a native
// kode tool), so agent.Core knows how to route a tool_use call.
func (r *Registry) Owns(name string) bool {
	serverName, _, ok := strings.Cut(name, namespaceSep)
	if !ok {
		return false
	}
	for _, srv := range r.servers {
		if srv.name == serverName {
			return true
		}
	}
	return false
}

func (r *Registry) Close() {
	for _, srv := range r.servers {
		srv.session.Close()
	}
}

func contentToText(blocks []sdk.Content) string {
	var b strings.Builder
	for _, c := range blocks {
		if tc, ok := c.(*sdk.TextContent); ok {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
