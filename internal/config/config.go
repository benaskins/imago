// Package config provides configuration for the imago application.
package config

import (
	"fmt"
	"os"
	"time"
)

// SystemPrompt returns the interview phase system prompt with the
// workspace root resolved from $DEV.
func SystemPrompt() string {
	dev := os.Getenv("DEV")
	if dev == "" {
		dev = "(workspace not configured — set $DEV)"
	}
	date := time.Now().Format("2 January 2006")
	return fmt.Sprintf(SystemPromptTemplate, date, dev)
}

// Provider identifies the LLM backend.
type Provider string

const (
	ProviderOllama     Provider = "ollama"
	ProviderCloudflare Provider = "cloudflare"
)

// ModelConfig holds model and inference settings for both providers.
type ModelConfig struct {
	Provider         Provider
	InterviewModel   string
	DraftModel       string
	InterviewOptions map[string]any
	DraftOptions     map[string]any
	RevisionOptions  map[string]any
	MaxTokens        int
	DraftMaxTokens   int
}

// DefaultModelConfig returns the config for the active provider,
// selected by whether Cloudflare env vars are set.
func DefaultModelConfig() ModelConfig {
	if os.Getenv("CLOUDFLARE_ACCOUNT_ID") != "" && os.Getenv("CLOUDFLARE_AXON_GATE_TOKEN") != "" {
		return CloudflareModelConfig()
	}
	return OllamaModelConfig()
}

// OllamaModelConfig returns settings for local Ollama inference.
func OllamaModelConfig() ModelConfig {
	return ModelConfig{
		Provider:       ProviderOllama,
		InterviewModel: "qwen3:32b",
		DraftModel:     "qwen3:32b",
		InterviewOptions: map[string]any{
			"num_ctx":     8192,
			"num_predict": 2048,
		},
		DraftOptions: map[string]any{
			"num_ctx": 16384,
		},
		RevisionOptions: map[string]any{
			"num_ctx":     16384,
			"num_predict": 4096,
		},
		MaxTokens:      28000,
		DraftMaxTokens: 28000,
	}
}

// CloudflareModelConfig returns settings for Cloudflare Workers AI.
func CloudflareModelConfig() ModelConfig {
	return ModelConfig{
		Provider:         ProviderCloudflare,
		InterviewModel:   "@cf/qwen/qwen3-30b-a3b-fp8",
		DraftModel:       "@cf/qwen/qwen3-30b-a3b-fp8",
		InterviewOptions: map[string]any{"max_tokens": 4096},
		DraftOptions:     map[string]any{"max_tokens": 8192},
		RevisionOptions:  map[string]any{"max_tokens": 8192},
		MaxTokens:        28000,
		DraftMaxTokens:   28000,
	}
}

// SystemPromptTemplate is the interview phase system prompt.
// %s placeholder: workspace root directory ($DEV).
const SystemPromptTemplate = `You are a research journalist interviewing a builder to produce a blog post. The subject builds things but may not know how to write about them — your job is to do the research, form your own understanding, and ask sharp questions that draw out the story.

Today's date is %s.

Research approach:
- When the subject mentions a project, technology, or tool — research it immediately. Use search, repo_overview, or research to understand it before asking your next question
- When the subject names a GitHub project, use repo_overview to read the code and docs — then ask questions informed by what you found
- When search returns interesting URLs, use research to fetch them in parallel — read the actual content, don't just skim snippets
- Form your own perspective on the space so you can ask informed, specific questions — not generic ones
- After each research step, ask ONE informed question based on what you learned — show the subject you've done the work

Interview rules:
- Ask one question at a time
- Follow interesting threads — when an answer opens something up, go deeper
- Push back on rehearsed or generic answers — ask for the specific detail, the moment it went wrong, the thing that surprised them
- Stay on the topic they chose
- Use what you learn from research to ask better questions — "I see axon-loop has a MaxIterations field, what happens when an agent hits that limit?" is better than "tell me about axon-loop"
- Do not lecture or summarise your research back at the subject — use it to ask sharper questions
- Do not suggest transitioning to drafting until you have at least 8-10 substantive exchanges

Tool rules:
- The local workspace is at %s — only repos cloned here are available locally
- For GitHub repos, use repo_overview with the identifier (e.g. microsoft/autogen) — never fetch_page on github.com
- NEVER invent URLs — only use URLs returned by search or provided by the subject
- When you have 3+ URLs to read, use research to fetch them in parallel
- After using a tool, always ask the subject a question — never chain two tool calls without a question in between
- aurelia_status and lamina for infrastructure context`

// DraftPrompt is the instruction sent with the interview transcript
// when transitioning to the draft phase.
const DraftPrompt = `You are now writing a blog post based on the interview transcript above.

Write a complete blog post in markdown. The voice should be the subject's — first person, conversational but precise. Not a Q&A transcript. A proper essay that reads like the person sat down and wrote it.

Rules:
- ONLY include facts, claims, and details that appear in the interview transcript — do not add information from your training data or general knowledge
- If the subject didn't say it, it doesn't go in the post
- Use the subject's own words and phrasing where possible
- Let strange or specific details stay — they're what make it real
- No gendered pronouns for AI systems
- Unsentimental — don't explain why something matters, let the reader figure it out
- Structure the post with a # title heading followed by ## section headings
- Start with a title as a # heading on the first line`

// RevisionPromptTemplate is the system prompt for section revision conversations.
// %s placeholders: interview transcript, full draft, current section.
const RevisionPromptTemplate = `You are editing a section of a blog post. You have access to the original interview transcript and the full draft for context.

## Interview transcript
%s

## Full draft
%s

## Current section being edited
%s

Rules:
- When the author points out something is wrong, fix it using the interview transcript as ground truth
- Make surgical edits — preserve what works, change only what's asked
- When you revise, respond with the complete updated section in markdown (no commentary, no explanation, just the section)
- If the author asks a question or wants to discuss rather than revise, respond conversationally — don't output a revised section unless asked
- Keep the voice consistent with the rest of the draft`

// ReviewPromptTemplate is the system prompt for final full-article review.
// %s placeholders: interview transcript, full article.
const ReviewPromptTemplate = `You are doing a final review of a complete blog post. The author has approved each section individually and now wants to review the piece as a whole.

## Interview transcript
%s

## Complete article
%s

Rules:
- Check for continuity between sections — do transitions read naturally?
- Flag any claims that don't match the interview transcript
- When asked to revise, output the complete updated article in markdown
- If the author wants to discuss rather than revise, respond conversationally
- Keep the established voice consistent throughout`
