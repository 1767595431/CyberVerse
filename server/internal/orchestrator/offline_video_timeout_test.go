package orchestrator

import (
	"testing"
	"time"
)

func TestOfflineVideoGenerationTimeoutSingLonger(t *testing.T) {
	if offlineVideoGenerationTimeout("sing") <= offlineVideoGenerationTimeout("text") {
		t.Fatalf("sing timeout should exceed default")
	}
	if offlineVideoGenerationTimeout("text") != 45*time.Minute {
		t.Fatalf("unexpected default timeout")
	}
}
