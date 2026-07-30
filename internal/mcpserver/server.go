// Package mcpserver is kode's MCP SERVER: it hosts a local HTTP/SSE endpoint
// so an external MCP client — most notably a separately-running Claude Code
// session — can read the diff and annotations of a live kode TUI and add its
// own annotations back, which then appear in the TUI immediately. This is
// distinct from internal/agent/mcp, which is the client side kode's own
// embedded agent uses to reach OUT to external MCP servers.
package mcpserver

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mansooranis/kode/internal/annotate"
	"github.com/mansooranis/kode/internal/diagram"
	"github.com/mansooranis/kode/internal/diffparse"
)

type Server struct {
	store     *annotate.Store
	changeset diffparse.Changeset
	diagram   diagram.Renderer
	httpSrv   *http.Server
}

func New(store *annotate.Store, changeset diffparse.Changeset, renderer diagram.Renderer, port int) *Server {
	s := &Server{store: store, changeset: changeset, diagram: renderer}

	mcpSrv := sdk.NewServer(&sdk.Implementation{Name: "kode", Version: "0.1.0"}, nil)
	sdk.AddTool(mcpSrv, &sdk.Tool{
		Name:        "add_annotation",
		Description: "Add a comment on a specific line of a file kode currently has open (reviewing a diff, or in `kode explain` mode). Use this to answer a reviewer's question, or to leave an explanation of a piece of code so someone else can learn it via `kode explain`.",
	}, s.addAnnotation)
	sdk.AddTool(mcpSrv, &sdk.Tool{
		Name:        "add_diagram",
		Description: "Render Mermaid diagram source to ASCII/Unicode art and attach it at a specific file/line, so it shows up as a diagram card in kode (e.g. via `kode explain`). Use this for flowcharts/control-flow/architecture diagrams when explaining code.",
	}, s.addDiagram)
	sdk.AddTool(mcpSrv, &sdk.Tool{
		Name:        "list_annotations",
		Description: "List all comments/annotations/diagrams currently in kode, optionally filtered to one file. Use this to find unanswered reviewer comments, or to see what's already been explained before adding more.",
	}, s.listAnnotations)
	sdk.AddTool(mcpSrv, &sdk.Tool{
		Name:        "get_diff",
		Description: "Get the unified diff text for a file (or the whole changeset if file is omitted) currently open in kode.",
	}, s.getDiff)
	sdk.AddTool(mcpSrv, &sdk.Tool{
		Name:        "list_files",
		Description: "List the files in the diff currently open in kode.",
	}, s.listFiles)

	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return mcpSrv }, nil)
	s.httpSrv = &http.Server{
		Addr:    "127.0.0.1:" + strconv.Itoa(port),
		Handler: handler,
	}
	return s
}

// Start begins serving in the background. It logs (rather than returns) a
// listen failure, since a broken MCP server must never take down the TUI —
// chat/diff/annotation features all work without it.
func (s *Server) Start() {
	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("kode: mcp server: %v", err)
		}
	}()
}

func (s *Server) Close(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

// Addr is the address the server is bound to, for printing setup
// instructions (e.g. `claude mcp add`).
func (s *Server) Addr() string {
	return s.httpSrv.Addr
}

type addAnnotationArgs struct {
	File string `json:"file" jsonschema:"path of the file to comment on, as shown by list_files"`
	Line int    `json:"line" jsonschema:"the line number to attach the comment to, as shown in the diff"`
	Text string `json:"text" jsonschema:"the comment text"`
}

func (s *Server) addAnnotation(_ context.Context, req *sdk.CallToolRequest, args addAnnotationArgs) (*sdk.CallToolResult, any, error) {
	source := "mcp"
	if info := req.ClientInfo(); info != nil && info.Name != "" {
		source = "mcp:" + info.Name
	}

	a := s.store.Add(annotate.Annotation{
		File:   args.File,
		Line:   args.Line,
		Author: source,
		Text:   args.Text,
	})

	return &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: fmt.Sprintf("added annotation %s on %s:%d", a.ID, a.File, a.Line)}},
	}, nil, nil
}

type addDiagramArgs struct {
	File    string `json:"file" jsonschema:"path of the file to attach the diagram to"`
	Line    int    `json:"line" jsonschema:"the line number to attach the diagram to"`
	Mermaid string `json:"mermaid" jsonschema:"Mermaid diagram source (e.g. a flowchart/graph definition)"`
}

func (s *Server) addDiagram(_ context.Context, req *sdk.CallToolRequest, args addDiagramArgs) (*sdk.CallToolResult, any, error) {
	rendered, err := s.diagram.Render(args.Mermaid)
	if err != nil {
		return &sdk.CallToolResult{
			Content: []sdk.Content{&sdk.TextContent{Text: fmt.Sprintf("failed to render diagram: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	source := "mcp"
	if info := req.ClientInfo(); info != nil && info.Name != "" {
		source = "mcp:" + info.Name
	}

	a := s.store.Add(annotate.Annotation{
		File:   args.File,
		Line:   args.Line,
		Author: source,
		Kind:   annotate.KindDiagram,
		Text:   rendered,
		Source: args.Mermaid,
	})

	return &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: fmt.Sprintf("added diagram %s on %s:%d", a.ID, a.File, a.Line)}},
	}, nil, nil
}

type listAnnotationsArgs struct {
	File string `json:"file,omitempty" jsonschema:"optional: restrict to this file"`
}

func (s *Server) listAnnotations(_ context.Context, _ *sdk.CallToolRequest, args listAnnotationsArgs) (*sdk.CallToolResult, any, error) {
	var annotations []annotate.Annotation
	if args.File != "" {
		annotations = s.store.ForFile(args.File)
	} else {
		annotations = s.store.All()
	}

	if len(annotations) == 0 {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "no annotations yet"}}}, nil, nil
	}

	var b strings.Builder
	for _, a := range annotations {
		kind := a.Kind
		if kind == "" {
			kind = annotate.KindComment
		}
		fmt.Fprintf(&b, "[%s] %s:%d (%s, %s): %s\n", a.ID, a.File, a.Line, a.Author, kind, a.Text)
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: b.String()}}}, nil, nil
}

type getDiffArgs struct {
	File string `json:"file,omitempty" jsonschema:"optional: only return the diff for this file"`
}

func (s *Server) getDiff(_ context.Context, _ *sdk.CallToolRequest, args getDiffArgs) (*sdk.CallToolResult, any, error) {
	var b strings.Builder
	for _, f := range s.changeset.Files {
		if args.File != "" && f.Name() != args.File {
			continue
		}
		fmt.Fprintf(&b, "--- %s\n+++ %s\n", f.OldName, f.NewName)
		for _, h := range f.Hunks {
			b.WriteString(h.Header)
			b.WriteString("\n")
			for _, l := range h.Lines {
				sign := " "
				switch l.Op {
				case diffparse.OpAdd:
					sign = "+"
				case diffparse.OpDelete:
					sign = "-"
				}
				b.WriteString(sign)
				b.WriteString(l.Content)
				b.WriteString("\n")
			}
		}
	}

	text := b.String()
	if text == "" {
		text = "no matching file in the current diff"
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: text}}}, nil, nil
}

func (s *Server) listFiles(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, any, error) {
	var b strings.Builder
	for _, f := range s.changeset.Files {
		status := "M"
		switch {
		case f.IsNew:
			status = "A"
		case f.IsDelete:
			status = "D"
		case f.IsRename:
			status = "R"
		}
		fmt.Fprintf(&b, "%s %s\n", status, f.Name())
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: b.String()}}}, nil, nil
}
