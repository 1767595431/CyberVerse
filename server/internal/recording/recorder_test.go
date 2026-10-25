package recording

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/cyberverse/server/internal/config"
)

func TestSaveTranscriptWritesTurnTextFile(t *testing.T) {
	tmpDir := t.TempDir()
	recorder := NewVideoRecorder(config.RecordingConfig{
		Enabled:   true,
		OutputDir: tmpDir,
	})

	const transcript = "豆包回复文本"
	if err := recorder.SaveTranscript("session-1", "turn1", transcript); err != nil {
		t.Fatalf("SaveTranscript returned error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tmpDir, "session-1", "turn1.txt"))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(got) != transcript {
		t.Fatalf("expected transcript %q, got %q", transcript, string(got))
	}
}

func TestSaveTranscriptUsesAbsoluteSessionDir(t *testing.T) {
	tmpDir := t.TempDir()
	sessionDir := filepath.Join(tmpDir, "sessions", "session-abs")
	recorder := NewVideoRecorder(config.RecordingConfig{
		Enabled:   true,
		OutputDir: filepath.Join(tmpDir, "ignored"),
	})

	if err := recorder.SaveTranscript(sessionDir, "turn2", "absolute transcript"); err != nil {
		t.Fatalf("SaveTranscript returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(sessionDir, "turn2.txt")); err != nil {
		t.Fatalf("expected transcript file in absolute session dir: %v", err)
	}
}

func TestPadRGBChunksToAudioDurationExtendsShortVideo(t *testing.T) {
	const w, h, fps = 4, 4, 20
	frameBytes := w * h * 3
	oneFrame := bytes.Repeat([]byte{1}, frameBytes)
	rgb := [][]byte{oneFrame, oneFrame}
	// 0.15s mono PCM16 @ 16kHz => 3 frames @ 20fps; input has only 2
	pcm := make([]byte, 4800)

	out := padRGBChunksToAudioDuration(rgb, w, h, fps, pcm, 16000)
	got := rgbFrameCount(out, frameBytes)
	if got < 2 {
		t.Fatalf("expected at least 2 frames after pad, got %d", got)
	}
	if got <= rgbFrameCount(rgb, frameBytes) {
		t.Fatalf("expected more frames than input, got %d", got)
	}
}
