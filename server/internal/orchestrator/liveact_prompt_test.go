package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteLiveActDynamicPromptClearsWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dynamic_prompt.txt")
	orig := LiveActDynamicPromptPath
	LiveActDynamicPromptPath = path
	t.Cleanup(func() { LiveActDynamicPromptPath = orig })

	if err := WriteLiveActDynamicPrompt("test prompt"); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "test prompt" {
		t.Fatalf("expected file content, got %q err=%v", data, err)
	}
	if err := WriteLiveActDynamicPrompt(""); err != nil {
		t.Fatalf("clear prompt: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, stat err=%v", err)
	}
}
