package media

import "testing"

func TestResolveOfflineLiveActionPromptFromTitle(t *testing.T) {
	got := ResolveOfflineLiveActionPromptFromTitle("动作-撒娇")
	if got == "" || got == ResolveOfflineLiveActionPromptFromTitle("动作-歪头卖萌") {
		t.Fatalf("expected distinct prompt for 撒娇, got %q", got)
	}
	if ResolveOfflineLiveActionPromptFromTitle("动作-歪头卖萌") == "" {
		t.Fatal("expected prompt for 歪头卖萌")
	}
	if ResolveOfflineLiveActionPromptFromTitle("未知标题") != "" {
		t.Fatal("expected empty for unknown title")
	}
}
