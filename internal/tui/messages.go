package tui

// tokenMsg carries a single streamed token from the LLM.
type tokenMsg struct {
	token string
}

// doneMsg signals the LLM has finished its response.
type doneMsg struct{}

// toolUseMsg signals a tool is being invoked.
type toolUseMsg struct {
	name string
	args map[string]any
}

// errMsg carries an error from the LLM.
type errMsg struct {
	err error
}

// phaseSwitchMsg triggers a transition from interview to draft.
type phaseSwitchMsg struct{}

// sectionDoneMsg signals a draft section has been generated.
type sectionDoneMsg struct {
	sections []string
}

// sectionReviseMsg carries the revised section content.
type sectionReviseMsg struct {
	content string
}
