package orchestrator

import (
	"strings"
	"time"
)

const offlineVideoSafetyTimeout = 4 * time.Hour

// OfflineVideoSafetyTimeout caps queued wait plus generation for a single offline job goroutine.
func OfflineVideoSafetyTimeout() time.Duration {
	return offlineVideoSafetyTimeout
}

func offlineVideoGenerationTimeout(inputType string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(inputType)) {
	case "sing":
		return 120 * time.Minute
	default:
		return 45 * time.Minute
	}
}
