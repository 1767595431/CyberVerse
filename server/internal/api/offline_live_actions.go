package api

import (
	"encoding/json"
	"log"
	"math/rand/v2"
	"net/http"
	"strings"

	"github.com/cyberverse/server/internal/orchestrator"
)

type offlineLiveActionClipRequest struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type triggerOfflineLiveActionRequest struct {
	Action   string `json:"action"`
	ActionID string `json:"action_id"`
	Clear    bool   `json:"clear"`
}

func offlineLiveActionsFromRequest(items []offlineLiveActionClipRequest) []orchestrator.OfflineLiveActionClip {
	if len(items) == 0 {
		return nil
	}
	out := make([]orchestrator.OfflineLiveActionClip, 0, len(items))
	for _, item := range items {
		out = append(out, orchestrator.OfflineLiveActionClip{
			ID:    item.ID,
			Title: item.Title,
			URL:   item.URL,
		})
	}
	return out
}

func (r *Router) handleTriggerOfflineLiveAction(w http.ResponseWriter, req *http.Request) {
	if r.sessionMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "session manager is not configured"})
		return
	}
	sessionID := req.PathValue("id")
	session, err := r.sessionMgr.Get(sessionID)
	if err != nil || session == nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "session not found"})
		return
	}
	if !session.IsOfflineLive() {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "session is not offline_live"})
		return
	}

	var body triggerOfflineLiveActionRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	if body.Clear || req.URL.Query().Get("clear") == "1" {
		r.broadcastOfflineLiveAction(sessionID, "", "", "", true)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "cleared": true})
		return
	}
	if strings.TrimSpace(body.Action) == "" && strings.TrimSpace(body.ActionID) == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "action, action_id, or clear is required"})
		return
	}

	clip, ok := session.FindOfflineLiveAction(body.ActionID, body.Action)
	if !ok || clip == nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "offline live action not found"})
		return
	}

	r.broadcastOfflineLiveAction(sessionID, clip.ID, clip.Title, clip.URL, false)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"action_id": clip.ID,
		"title":     clip.Title,
		"url":       clip.URL,
	})
}

func (r *Router) triggerRandomOfflineLiveAction(sessionID string) {
	if r.danmakuScheduler != nil && r.danmakuScheduler.IdleActionsPaused(sessionID) {
		return
	}
	if r.sessionMgr == nil {
		return
	}
	session, err := r.sessionMgr.Get(sessionID)
	if err != nil || session == nil || !session.IsOfflineLive() {
		return
	}
	_, actions, _ := session.OfflineLiveMediaSnapshot()
	if len(actions) == 0 {
		return
	}
	clip := actions[rand.IntN(len(actions))]
	r.broadcastOfflineLiveAction(sessionID, clip.ID, clip.Title, clip.URL, false)
	log.Printf("offline live: random idle action session=%s title=%s", sessionID, clip.Title)
}

func (r *Router) broadcastOfflineLiveAction(sessionID, actionID, title, url string, clear bool) {
	if r.wsHub == nil {
		return
	}
	payload := map[string]any{
		"type":  "offline_live_action",
		"clear": clear,
	}
	if !clear {
		payload["action_id"] = actionID
		payload["title"] = title
		payload["url"] = url
	}
	r.wsHub.BroadcastJSON(sessionID, payload)
}

type triggerOfflineLiveSongRequest struct {
	SongID string `json:"song_id"`
	Title  string `json:"title"`
	Stop   bool   `json:"stop"`
}

func (r *Router) handleTriggerOfflineLiveSong(w http.ResponseWriter, req *http.Request) {
	if r.sessionMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "session manager is not configured"})
		return
	}
	sessionID := req.PathValue("id")
	session, err := r.sessionMgr.Get(sessionID)
	if err != nil || session == nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "session not found"})
		return
	}
	if !session.IsOfflineLive() {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "session is not offline_live"})
		return
	}

	var body triggerOfflineLiveSongRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	if body.Stop || req.URL.Query().Get("stop") == "1" {
		r.broadcastOfflineLiveSong(sessionID, "", "", "", true)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "stopped": true})
		return
	}
	if strings.TrimSpace(body.Title) == "" && strings.TrimSpace(body.SongID) == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "song_id, title, or stop is required"})
		return
	}

	clip, ok := session.FindOfflineLiveSong(body.SongID, body.Title)
	if !ok || clip == nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "offline live song not found"})
		return
	}

	r.broadcastOfflineLiveSong(sessionID, clip.ID, clip.Title, clip.URL, false)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"song_id": clip.ID,
		"title":   clip.Title,
		"url":     clip.URL,
	})
}

func (r *Router) broadcastOfflineLiveSong(sessionID, songID, title, url string, stop bool) {
	if r.danmakuScheduler != nil {
		if stop {
			r.danmakuScheduler.ResumeIdleActions(sessionID)
		} else if strings.TrimSpace(url) != "" {
			r.danmakuScheduler.PauseIdleActions(sessionID)
		}
	}
	if r.wsHub == nil {
		return
	}
	payload := map[string]any{
		"type": "offline_live_song",
		"stop": stop,
	}
	if !stop {
		payload["song_id"] = songID
		payload["title"] = title
		payload["url"] = url
	}
	r.wsHub.BroadcastJSON(sessionID, payload)
}
