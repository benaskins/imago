# Example session

A real session that produced [the first imago blog post](https://generativeplane.com). The conversation below is lightly edited for length — tool output is summarised.

## Interview phase

> **imago** What topic would you like to focus on for this blog post?

> **you** Ok, here's what I'm thinking. You write a short blog post on how and why imago was developed. You can explore the code base here. You can see my writing style on my blog, and you can interview me using an interview loop. What do you say?

imago calls `repo_overview` on the imago directory, reads the design doc, go.mod, and main.go.

> **imago** What specific problem or limitation in the blog-writing process did imago aim to solve that existing tools didn't address?

> **you** I was already writing blog posts in a conversational loop with claude and claude code, but the user interface is built for chat, not for content review and editing. I wanted an interview loop to help me get unblocked, and then a review loop that allowed me to focus on individual chunks that I could review and revise one at a time.

> **imago** What was the first specific moment you realized the chat interface was actively breaking your workflow for blog writing — did you hit a concrete technical limitation or a usability dead end?

> **you** It was having to flip back and forth between a text editor and the chat interface. There was no clean way to get the content in front of my eyes without switching my flow and focus, so I built a TUI that maps to my process. Conversational writing, followed by draft reviews and revision, before publishing it. It just matches how I was already working, but with less friction.

> **imago** What specific technical choices in the imago architecture made the two-phase workflow possible that weren't achievable with the chat interface?

> **you** Why are you only using the repo_overview tool - you have so many other tools available to explore the codebase. Go do some more thorough research into how imago was composed, explore the files.

imago calls `repo_overview`, `read_files` on go.mod, CLAUDE.md, main.go and several internal packages.

> **you** Do you have enough information from me to write a draft? It should be low hype. Matter of fact. Epistemologically honest.

> **imago** I believe I have sufficient material. Shall we transition to drafting?

> **you** Yes please.

User types `/draft`.

## Draft phase

imago generates a full post using `qwen3:32b`, split into 8 sections by heading. The draft appears section by section for review.

**Section 1** — Title and intro. Approved with `/keep`.

**Section 2** — The friction point.

> **you** I was actually using claude and claude code before building imago. I hadn't yet started using the axon tools for that.

imago revises the section to correct the timeline. Approved.

**Section 3** — The terminal as a state machine.

> **you** Let's get rid of the AI trope "This isn't just..."

imago rewrites to remove the trope. Approved.

**Section 4** — Local LLMs.

> **you** I'm really using ollama for the local inference capabilities. To not spend precious tokens on writing tasks that a smaller model can handle capably, and to keep everything private before I decide to publish outside of my local machine. There's no back and forth of this conversation across a network, it's private, it's secure, and it will only be used to train my models in future.

imago revises with the privacy and cost framing. Approved.

**Section 5** — Modular tooling. Approved with `/keep`.

**Section 6** — Editorial memory.

> **you** Reframe it so it's not using "This isn't just"

imago revises.

> **you** The memory is also about letting the 'journalist' persona develop over time, for imago to find its voice

imago incorporates the voice development angle. Approved.

**Sections 7-8** — Approved with `/keep`.

## Final review

All 8 sections assembled. User types `/done`.

Draft saved to `~/Documents/imago/imago-building-a-terminal-workflow-for-conversational-writing.md`.

Total time: ~26 minutes from first question to saved file.
