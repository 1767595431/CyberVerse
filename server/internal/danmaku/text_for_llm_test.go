package danmaku

import "testing"

func TestDanmakuTextForLLM(t *testing.T) {
	tests := []struct {
		user, text, want string
	}{
		{"你的名字", "你是谁啊", "你是谁啊"},
		{"alice", "hello", "hello"},
		{"", "plain", "plain"},
		{"观众", "plain", "plain"},
		{"bob", "bob：already stripped", "already stripped"},
		{"bob", "bob:ascii", "ascii"},
	}
	for _, tc := range tests {
		got := DanmakuTextForLLM(tc.user, tc.text)
		if got != tc.want {
			t.Fatalf("DanmakuTextForLLM(%q, %q) = %q, want %q", tc.user, tc.text, got, tc.want)
		}
	}
}
