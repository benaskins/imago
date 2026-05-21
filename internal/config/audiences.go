package config

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"text/template"
)

//go:embed all:audiences
var audiencesFS embed.FS

// PromptData holds all placeholder values an audience template may reference.
// A given template uses only the fields it needs.
type PromptData struct {
	Date                string
	WorkspaceName       string
	WorkspacePath       string
	ActivityReport      string
	PreviousSection     string
	InterviewTranscript string
	FullDraft           string
	CurrentSection      string
	FullArticle         string
}

// Template wraps a parsed prompt template with a Render method.
type Template struct {
	tpl *template.Template
}

// Render executes the template with the provided data.
func (t *Template) Render(data PromptData) (string, error) {
	if t == nil || t.tpl == nil {
		return "", nil
	}
	var sb strings.Builder
	if err := t.tpl.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// AudienceTemplates is the set of templates an (audience, mode) pair provides.
// Period modes (daily, weekly) populate System and Draft. Interview mode
// additionally populates Revision and Review.
type AudienceTemplates struct {
	System   *Template
	Draft    *Template
	Revision *Template
	Review   *Template
}

// LoadAudience returns the templates for the given audience and mode.
// Returns an error if no templates exist at audiences/<audience>/<mode>/.
func LoadAudience(audience, mode string) (*AudienceTemplates, error) {
	dir := fmt.Sprintf("audiences/%s/%s", audience, mode)
	entries, err := fs.ReadDir(audiencesFS, dir)
	if err != nil {
		return nil, fmt.Errorf("audience %q mode %q: %w", audience, mode, err)
	}

	out := &AudienceTemplates{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".tmpl") {
			continue
		}
		raw, err := fs.ReadFile(audiencesFS, dir+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("read %s/%s: %w", dir, name, err)
		}
		body := strings.TrimRight(string(raw), "\n")
		tpl, err := template.New(name).Parse(body)
		if err != nil {
			return nil, fmt.Errorf("parse %s/%s: %w", dir, name, err)
		}
		t := &Template{tpl: tpl}
		switch strings.TrimSuffix(name, ".tmpl") {
		case "system":
			out.System = t
		case "draft":
			out.Draft = t
		case "revision":
			out.Revision = t
		case "review":
			out.Review = t
		}
	}
	return out, nil
}
