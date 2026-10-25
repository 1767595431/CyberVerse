package danmaku

import "strings"

// DanmakuTextForLLM is the viewer utterance passed to the LLM (message body only, no nickname prefix).
func DanmakuTextForLLM(username, text string) string {
	text = strings.TrimSpace(text)
	username = strings.TrimSpace(username)
	if username == "" || username == "观众" {
		return text
	}
	prefix := username + "："
	if strings.HasPrefix(text, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(text, prefix))
	}
	// ASCII colon fallback for mis-encoded payloads.
	asciiPrefix := username + ":"
	if strings.HasPrefix(text, asciiPrefix) {
		return strings.TrimSpace(strings.TrimPrefix(text, asciiPrefix))
	}
	return text
}
