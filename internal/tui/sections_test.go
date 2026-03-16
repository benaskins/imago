package tui

import (
	"testing"
)

func TestSplitSections_HeadingsOnly(t *testing.T) {
	md := `# Title

Intro paragraph.

## Section One

Content one.

## Section Two

Content two.`

	sections := splitSections(md)
	if len(sections) != 3 {
		t.Fatalf("expected 3 sections, got %d: %v", len(sections), sections)
	}
	if sections[0] != "# Title\n\nIntro paragraph." {
		t.Errorf("section 0: %q", sections[0])
	}
	if sections[1] != "## Section One\n\nContent one." {
		t.Errorf("section 1: %q", sections[1])
	}
	if sections[2] != "## Section Two\n\nContent two." {
		t.Errorf("section 2: %q", sections[2])
	}
}

func TestSplitSections_NoHeadings(t *testing.T) {
	md := `Just a plain paragraph.

Another paragraph.`

	sections := splitSections(md)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if sections[0] != md {
		t.Errorf("expected full content, got: %q", sections[0])
	}
}

func TestSplitSections_Empty(t *testing.T) {
	sections := splitSections("")
	if sections != nil {
		t.Errorf("expected nil, got %v", sections)
	}

	sections = splitSections("   \n\n  ")
	if sections != nil {
		t.Errorf("expected nil for whitespace, got %v", sections)
	}
}

func TestSplitSections_PreambleBeforeFirstHeading(t *testing.T) {
	md := `Some preamble text.

# Actual Title

Content.`

	sections := splitSections(md)
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d: %v", len(sections), sections)
	}
	if sections[0] != "Some preamble text." {
		t.Errorf("preamble: %q", sections[0])
	}
	if sections[1] != "# Actual Title\n\nContent." {
		t.Errorf("section 1: %q", sections[1])
	}
}

func TestSplitSections_WithHorizontalRules(t *testing.T) {
	// Ensure --- doesn't confuse the parser
	md := `# Title

Intro.

---

## Section One

Content.`

	sections := splitSections(md)
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d: %v", len(sections), sections)
	}
	// --- stays with the title section
	if sections[0] != "# Title\n\nIntro.\n\n---" {
		t.Errorf("section 0: %q", sections[0])
	}
}

func TestSplitSections_MultipleH1(t *testing.T) {
	md := `# Part One

Content one.

# Part Two

Content two.`

	sections := splitSections(md)
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
}

func TestSplitSections_NestedHeadings(t *testing.T) {
	md := `# Title

## Sub One

Content.

### Sub Sub

Detail.

## Sub Two

More content.`

	sections := splitSections(md)
	if len(sections) != 4 {
		t.Fatalf("expected 4 sections, got %d: %v", len(sections), sections)
	}
}

func TestHeadingLevel(t *testing.T) {
	tests := []struct {
		line  string
		level int
	}{
		{"# Title", 1},
		{"## Section", 2},
		{"### Sub", 3},
		{"###### Deep", 6},
		{"####### TooDeep", 0},
		{"#NoSpace", 0},
		{"Not a heading", 0},
		{"", 0},
		{"#", 1},
		{"##", 2},
	}

	for _, tt := range tests {
		got := headingLevel(tt.line)
		if got != tt.level {
			t.Errorf("headingLevel(%q) = %d, want %d", tt.line, got, tt.level)
		}
	}
}
