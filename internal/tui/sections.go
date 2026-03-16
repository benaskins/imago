package tui

import (
	"strings"
)

// splitSections splits markdown into sections by headings.
// Each section starts at a heading line (# or ##) and includes all
// content up to the next heading of equal or higher level, or EOF.
// If the markdown has no headings, the entire content is one section.
func splitSections(md string) []string {
	lines := strings.Split(md, "\n")

	type boundary struct {
		line  int
		level int
	}

	var headings []boundary
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if level := headingLevel(trimmed); level > 0 {
			headings = append(headings, boundary{line: i, level: level})
		}
	}

	if len(headings) == 0 {
		trimmed := strings.TrimSpace(md)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	}

	var sections []string

	// Content before first heading (preamble) — include if non-empty
	if headings[0].line > 0 {
		preamble := strings.TrimSpace(strings.Join(lines[:headings[0].line], "\n"))
		if preamble != "" {
			sections = append(sections, preamble)
		}
	}

	// Each heading starts a section that runs to the next heading
	for i, h := range headings {
		end := len(lines)
		if i+1 < len(headings) {
			end = headings[i+1].line
		}
		section := strings.TrimSpace(strings.Join(lines[h.line:end], "\n"))
		if section != "" {
			sections = append(sections, section)
		}
	}

	return sections
}

// headingLevel returns the heading level (1-6) for a markdown heading line,
// or 0 if the line is not a heading.
func headingLevel(line string) int {
	if !strings.HasPrefix(line, "#") {
		return 0
	}
	level := 0
	for _, c := range line {
		if c == '#' {
			level++
		} else {
			break
		}
	}
	if level > 6 {
		return 0
	}
	// Must be followed by a space or be just hashes
	rest := line[level:]
	if rest == "" || strings.HasPrefix(rest, " ") {
		return level
	}
	return 0
}
