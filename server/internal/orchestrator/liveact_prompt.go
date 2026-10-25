package orchestrator

import (
	"os"
	"strings"
)

// LiveActDynamicPromptPath is read by the LiveAct avatar plugin on each generation chunk.
var LiveActDynamicPromptPath = "/root/autodl-tmp/dynamic_prompt.txt"

// WriteLiveActDynamicPrompt updates the prompt file LiveAct reads.
// An empty prompt removes the file so LiveAct falls back to its configured default.
func WriteLiveActDynamicPrompt(prompt string) error {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		if err := os.Remove(LiveActDynamicPromptPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(LiveActDynamicPromptPath, []byte(prompt), 0644)
}
