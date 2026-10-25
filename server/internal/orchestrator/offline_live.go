package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OfflineLiveTurnVideoFilename returns the MP4 basename for a turn.
func OfflineLiveTurnVideoFilename(turnSeq uint64) string {
	if turnSeq == 0 {
		return "turn.mp4"
	}
	return fmt.Sprintf("turn%d.mp4", turnSeq)
}

// OfflineLiveTurnVideoURL is the HTTP path the frontend uses to fetch a turn MP4.
func OfflineLiveTurnVideoURL(sessionID string, turnSeq uint64) string {
	return fmt.Sprintf("/api/v1/sessions/%s/turns/%d/video", sessionID, turnSeq)
}

const offlineLiveTTSModelFallback = "qwen3-tts-flash-realtime"

// NormalizeOfflineLiveTTSModel maps legacy omni-realtime session overrides to qwen3-tts-flash-realtime.
func NormalizeOfflineLiveTTSModel(model string) string {
	m := strings.TrimSpace(model)
	if m == "" {
		return offlineLiveTTSModelFallback
	}
	lower := strings.ToLower(m)
	if strings.Contains(lower, "omni") && strings.Contains(lower, "realtime") {
		return offlineLiveTTSModelFallback
	}
	return m
}

// GenerateOfflineLiveTurnVideo buffers complete reply PCM into an offline LiveAct
// MP4 under the session recording directory, then notifies the frontend.
func (o *Orchestrator) GenerateOfflineLiveTurnVideo(
	ctx context.Context,
	session *Session,
	sessionID string,
	turnSeq uint64,
	pcm []byte,
	sampleRate int,
	sessionDir string,
) error {
	if o == nil {
		return errors.New("orchestrator is not configured")
	}
	if session == nil || !session.IsOfflineLive() {
		return errors.New("session is not offline_live")
	}
	if len(pcm) == 0 {
		return errors.New("reply audio is empty")
	}
	if strings.TrimSpace(sessionDir) == "" {
		sessionDir = o.sessionRecordingDir(session)
	}
	if strings.TrimSpace(sessionDir) == "" {
		return errors.New("session recording directory is unavailable")
	}
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	if sampleRate <= 0 {
		sampleRate = 16000
	}

	characterID := session.CharacterID
	if characterID == "" {
		return errors.New("character id is required")
	}
	_, imageFilename, err := o.activeCharacterImage(characterID)
	if err != nil || imageFilename == "" {
		return fmt.Errorf("resolve character image: %w", err)
	}
	imageData, imageFormat, err := o.loadCharacterImage(characterID, imageFilename)
	if err != nil {
		return err
	}

	outPath := filepath.Join(sessionDir, OfflineLiveTurnVideoFilename(turnSeq))
	jobID := fmt.Sprintf("%s-turn%d", sessionID, turnSeq)
	crf := 23

	o.broadcastStatusTurn(sessionID, "processing", turnSeq)
	log.Printf("offline_live: generating turn video session=%s turn=%d pcm_bytes=%d sr=%d out=%s",
		sessionID, turnSeq, len(pcm), sampleRate, outPath)

	genCtx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()

	_, err = o.GenerateOfflineVideo(genCtx, OfflineVideoGenerateInput{
		JobID:       jobID,
		CharacterID: characterID,
		ImageData:   imageData,
		ImageFormat: imageFormat,
		Audio: &OfflineAudioInput{
			PCM16:      pcm,
			SampleRate: sampleRate,
		},
		OutputPath: outPath,
		CRF:        crf,
		OnProgress: func(stage string, progress int, message string) {
			o.broadcastJSON(sessionID, map[string]any{
				"type":     "turn_video_progress",
				"turn_seq": turnSeq,
				"stage":    stage,
				"progress": progress,
				"message":  message,
			})
		},
	})
	if err != nil {
		return err
	}

	// Always notify with the owning turn_seq. Frontend queues by turn_seq so a
	// late Q1 clip still plays before Q2, never swapped with another turn's file.
	url := OfflineLiveTurnVideoURL(sessionID, turnSeq)
	o.broadcastJSON(sessionID, map[string]any{
		"type":     "turn_video_ready",
		"turn_seq": turnSeq,
		"url":      url,
	})
	log.Printf("offline_live: turn video ready session=%s turn=%d url=%s", sessionID, turnSeq, url)
	return nil
}

// ResolveOfflineLiveTurnVideoPath returns the absolute MP4 path for a session turn.
func (o *Orchestrator) ResolveOfflineLiveTurnVideoPath(sessionID string, turnSeq uint64) (string, error) {
	if o == nil || o.sessionMgr == nil {
		return "", errors.New("session manager is not configured")
	}
	session, err := o.sessionMgr.Get(sessionID)
	if err != nil || session == nil {
		return "", errors.New("session not found")
	}
	dir := session.RecordingDir
	if dir == "" {
		dir = o.sessionRecordingDir(session)
	}
	if dir == "" {
		return "", errors.New("session recording directory is unavailable")
	}
	path := filepath.Join(dir, OfflineLiveTurnVideoFilename(turnSeq))
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}
