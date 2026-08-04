package diagram

import "testing"

func TestLibRendererRendersGraph(t *testing.T) {
	out, err := NewLibRenderer().Render("graph LR\nA-->B")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty rendered output")
	}
}

func TestLibRendererInvalidSource(t *testing.T) {
	if _, err := NewLibRenderer().Render(""); err == nil {
		t.Fatal("expected an error for empty mermaid source")
	}
}
