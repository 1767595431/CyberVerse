package orchestrator

import "context"

const (
	// DanmakuReplyMaxRunes caps spoken reply length for offline-live danmaku turns.
	DanmakuReplyMaxRunes = 35
	// DanmakuReplyMaxTokens is passed to the LLM provider for the same turns.
	DanmakuReplyMaxTokens int32 = 72
)

const danmakuReplySystemSuffix = "当前为直播弹幕互动：请用一两句口语直接回应观众，总字数不超过35个汉字，不要寒暄套话、不要列表或长段落。"

// ContextForDanmakuReply returns a context that limits LLM/TTS reply length for one danmaku turn.
func ContextForDanmakuReply(ctx context.Context) context.Context {
	return WithTextInputOptions(ctx, TextInputOptions{
		MaxLLMTokens:       DanmakuReplyMaxTokens,
		MaxReplyRunes:      DanmakuReplyMaxRunes,
		SystemPromptSuffix: danmakuReplySystemSuffix,
		EphemeralUserTurn:  true,
	})
}
