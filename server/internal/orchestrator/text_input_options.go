package orchestrator

import (
	"context"
	"strings"
)

type textInputOptionsKey struct{}

// TextInputOptions adjusts a single HandleTextInput turn (e.g. short danmaku replies).
type TextInputOptions struct {
	MaxLLMTokens       int32
	MaxReplyRunes      int
	SystemPromptSuffix string
	// EphemeralUserTurn keeps the user utterance out of session history (danmaku scheduler turns).
	EphemeralUserTurn bool
}

type ephemeralUserTextKey struct{}

func withEphemeralUserText(ctx context.Context, text string) context.Context {
	return context.WithValue(ctx, ephemeralUserTextKey{}, strings.TrimSpace(text))
}

func ephemeralUserTextFromContext(ctx context.Context) (string, bool) {
	text, ok := ctx.Value(ephemeralUserTextKey{}).(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}

func WithTextInputOptions(ctx context.Context, opts TextInputOptions) context.Context {
	return context.WithValue(ctx, textInputOptionsKey{}, opts)
}

func textInputOptionsFromContext(ctx context.Context) (TextInputOptions, bool) {
	opts, ok := ctx.Value(textInputOptionsKey{}).(TextInputOptions)
	return opts, ok
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return strings.TrimSpace(s)
	}
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max])
}
