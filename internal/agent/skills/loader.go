// Package skills loads Claude-Code-style skill files: markdown with YAML
// frontmatter under a configured directory. Only name+description are kept
// resident in the system prompt; the full body loads on demand via the
// load_skill tool (progressive disclosure), so an arbitrarily large skill
// library costs almost nothing until the agent actually needs one.
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"
)

type Skill struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Triggers    []string `yaml:"triggers"`
	Body        string   `yaml:"-"`
}

// Library holds every skill discovered under a directory, keyed by name.
type Library struct {
	skills map[string]Skill
	order  []string
}

func Load(dir string) (*Library, error) {
	lib := &Library{skills: map[string]Skill{}}

	dir = expandHome(dir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return lib, nil
	}
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read skill %s: %w", path, err)
		}
		s, err := parse(data)
		if err != nil {
			return nil, fmt.Errorf("parse skill %s: %w", path, err)
		}
		if s.Name == "" {
			s.Name = strings.TrimSuffix(e.Name(), ".md")
		}
		lib.skills[s.Name] = s
		lib.order = append(lib.order, s.Name)
	}

	return lib, nil
}

func parse(data []byte) (Skill, error) {
	text := string(data)
	if !strings.HasPrefix(text, "---") {
		return Skill{Body: text}, nil
	}

	rest := strings.TrimPrefix(text, "---")
	end := strings.Index(rest, "\n---")
	if end == -1 {
		return Skill{Body: text}, nil
	}

	frontmatter := rest[:end]
	body := strings.TrimPrefix(rest[end+len("\n---"):], "\n")

	var s Skill
	if err := yaml.Unmarshal([]byte(frontmatter), &s); err != nil {
		return Skill{}, err
	}
	s.Body = body
	return s, nil
}

// Summaries returns name+description for every skill, for injection into the
// system prompt at session start.
func (l *Library) Summaries() []Skill {
	out := make([]Skill, 0, len(l.order))
	for _, name := range l.order {
		s := l.skills[name]
		out = append(out, Skill{Name: s.Name, Description: s.Description, Triggers: s.Triggers})
	}
	return out
}

// Get returns a skill's full body by name, for the load_skill tool.
func (l *Library) Get(name string) (Skill, bool) {
	s, ok := l.skills[name]
	return s, ok
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}
