package config

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"
)

// Today returns today's date in the format prompts expect.
func Today() string {
	return time.Now().Format("2 January 2006")
}

// ResolveWorkspacePath returns $DEV or a friendly fallback string.
// Used for the interview-mode system prompt where there's no
// explicit workspace argument.
func ResolveWorkspacePath() string {
	dev := os.Getenv("DEV")
	if dev == "" {
		return "(workspace not configured — set $DEV)"
	}
	return dev
}

// PreviousPostSection formats the optional "previous post" reference
// block for period-mode system prompts. Returns empty string if there
// is no previous post. periodLabel is "weekly" or "daily" and selects
// the heading wording.
func PreviousPostSection(periodLabel, previousPost string) string {
	if previousPost == "" {
		return ""
	}
	heading := "post"
	if periodLabel == "daily" {
		heading = "entry"
	}
	return fmt.Sprintf("\n## Previous %s %s (voice and structure reference)\n\n%s", periodLabel, heading, previousPost)
}

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

// AvailableAudiences returns the sorted names of audiences that
// support the given mode (i.e., audiences/<name>/<mode>/ exists).
func AvailableAudiences(mode string) []string {
	entries, err := fs.ReadDir(audiencesFS, "audiences")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := fs.ReadDir(audiencesFS, "audiences/"+e.Name()+"/"+mode); err == nil {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// LoadAudience returns the templates for the given audience and mode.
// Resolution order (later overrides earlier):
//  1. audiences/self/ (audience-level baseline — revision/review)
//  2. audiences/<audience>/ (audience-level overrides)
//  3. audiences/<audience>/<mode>/ (mode-specific templates — system, draft)
//
// Falling back to self at step 1 lets a new audience (e.g. manager)
// inherit shared revision/review templates without duplicating them.
// Returns an error if no templates exist at audiences/<audience>/<mode>/.
func LoadAudience(audience, mode string) (*AudienceTemplates, error) {
	out := &AudienceTemplates{}

	if audience != "self" {
		if err := readTemplatesInto("audiences/self", out); err != nil {
			return nil, err
		}
	}

	audienceDir := "audiences/" + audience
	if err := readTemplatesInto(audienceDir, out); err != nil {
		return nil, err
	}

	modeDir := audienceDir + "/" + mode
	if _, err := fs.ReadDir(audiencesFS, modeDir); err != nil {
		return nil, fmt.Errorf("audience %q mode %q: %w", audience, mode, err)
	}
	if err := readTemplatesInto(modeDir, out); err != nil {
		return nil, err
	}
	return out, nil
}

// readTemplatesInto reads any *.tmpl files at the top level of dir and
// assigns them to the matching field on out. Subdirectories are ignored.
func readTemplatesInto(dir string, out *AudienceTemplates) error {
	entries, err := fs.ReadDir(audiencesFS, dir)
	if err != nil {
		return nil // a missing dir is not an error here — caller checks
	}
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
			return fmt.Errorf("read %s/%s: %w", dir, name, err)
		}
		body := strings.TrimRight(string(raw), "\n")
		tpl, err := template.New(name).Parse(body)
		if err != nil {
			return fmt.Errorf("parse %s/%s: %w", dir, name, err)
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
	return nil
}
