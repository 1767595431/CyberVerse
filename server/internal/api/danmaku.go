package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cyberverse/server/internal/danmaku"
	"github.com/cyberverse/server/internal/orchestrator"
)

type danmakuInjectRequest struct {
	Username  string `json:"username"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	MessageID string `json:"message_id"`
}

func isLoopbackRequest(req *http.Request) bool {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (r *Router) handleDanmakuInject(w http.ResponseWriter, req *http.Request) {
	if !isLoopbackRequest(req) {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "danmaku inject is localhost only"})
		return
	}
	sessionID := req.PathValue("id")
	var body danmakuInjectRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}
	if err := r.injectDanmaku(sessionID, body); err != nil {
		switch err.Error() {
		case "session not found":
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		case "session is not offline_live", "text is required":
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) injectDanmaku(sessionID string, body danmakuInjectRequest) error {
	session, err := r.sessionMgr.Get(sessionID)
	if err != nil || session == nil {
		return fmt.Errorf("session not found")
	}
	if !session.IsOfflineLive() {
		return fmt.Errorf("session is not offline_live")
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		return fmt.Errorf("text is required")
	}
	username := strings.TrimSpace(body.Username)
	if username == "" {
		username = "观众"
	}
	source := strings.TrimSpace(body.Source)
	if source == "" {
		source = "unknown"
	}
	ts := time.Now().UnixMilli()
	msgID := strings.TrimSpace(body.MessageID)
	if r.danmakuFeed != nil {
		r.danmakuFeed.Add(sessionID, danmaku.FeedItem{
			Username:  username,
			Text:      text,
			Source:    source,
			MessageID: msgID,
			Timestamp: ts,
		})
	}
	if r.wsHub != nil {
		r.wsHub.BroadcastJSON(sessionID, map[string]any{
			"type":       "danmaku",
			"username":   username,
			"text":       text,
			"source":     source,
			"message_id": msgID,
			"timestamp":  ts,
		})
	}
	if r.danmakuScheduler != nil && session.DanmakuEnabled() {
		r.danmakuScheduler.Enqueue(sessionID, body.MessageID, username, text, source)
	}
	return nil
}

func (r *Router) startSessionDanmaku(sessionID string, session *orchestrator.Session) {
	if r.danmakuRunner == nil || session == nil || !session.DanmakuEnabled() {
		return
	}
	cfg := session.DanmakuConfigSnapshot()
	log.Printf("danmaku connect start session=%s platform=%s", sessionID, cfg.Platform)
	if r.danmakuRunner.Running(sessionID) {
		log.Printf("danmaku connect skipped session=%s already running", sessionID)
		if r.wsHub != nil {
			r.wsHub.BroadcastJSON(sessionID, map[string]any{
				"type":     "danmaku_status",
				"status":   "connected",
				"platform": cfg.Platform,
			})
		}
		return
	}
	if r.sessionMgr != nil {
		if live, err := r.sessionMgr.Get(sessionID); err != nil || live == nil {
			return
		}
	}
	if r.danmakuScheduler != nil {
		r.danmakuScheduler.Start(sessionID)
	}
	if r.wsHub != nil && cfg.Platform == "douyin" {
		r.wsHub.BroadcastJSON(sessionID, map[string]any{
			"type":    "danmaku_status",
			"status":  "starting",
			"message": "starting DyHub relay",
			"platform": cfg.Platform,
		})
	}
	if r.sessionMgr != nil {
		if live, err := r.sessionMgr.Get(sessionID); err != nil || live == nil {
			if r.danmakuScheduler != nil {
				r.danmakuScheduler.Stop(sessionID)
			}
			return
		}
	}
	if err := r.danmakuRunner.Start(sessionID, danmaku.SessionConfig{
		Platform:       cfg.Platform,
		BilibiliRoomID: cfg.BilibiliRoomID,
		DouyinRoom:     cfg.DouyinRoom,
		DouyinRelay:    cfg.DouyinRelay,
	}); err != nil {
		log.Printf("failed to start danmaku listener session=%s: %v", sessionID, err)
		if r.danmakuScheduler != nil {
			r.danmakuScheduler.Stop(sessionID)
		}
		if r.wsHub != nil {
			r.wsHub.BroadcastJSON(sessionID, map[string]any{
				"type":    "danmaku_status",
				"status":  "error",
				"message": err.Error(),
			})
		}
		return
	}
	if r.sessionMgr != nil {
		if live, err := r.sessionMgr.Get(sessionID); err != nil || live == nil {
			log.Printf("danmaku listener ready but session ended session=%s", sessionID)
			r.stopSessionDanmaku(sessionID)
			return
		}
	}
	if cfg.Platform != "douyin" && r.wsHub != nil {
		r.wsHub.BroadcastJSON(sessionID, map[string]any{
			"type":     "danmaku_status",
			"status":   "connected",
			"platform": cfg.Platform,
		})
	}
	log.Printf("danmaku connect ready session=%s platform=%s listener=running", sessionID, cfg.Platform)
}

func (r *Router) handleDanmakuConnect(w http.ResponseWriter, req *http.Request) {
	sessionID := req.PathValue("id")
	session, err := r.sessionMgr.Get(sessionID)
	if err != nil || session == nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "session not found"})
		return
	}
	if !session.IsOfflineLive() || !session.DanmakuEnabled() {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "session has no danmaku configured"})
		return
	}
	cfg := session.DanmakuConfigSnapshot()
	if r.danmakuRunner != nil && r.danmakuRunner.Running(sessionID) {
		if cfg.Platform != "douyin" || r.danmakuRunner.DouyinRelayConnected(sessionID) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "already_connected"})
			return
		}
		r.stopSessionDanmaku(sessionID)
	}
	r.stopDanmakuListenersExcept(sessionID)
	go r.startSessionDanmaku(sessionID, session)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "connecting"})
}

func (r *Router) stopDanmakuListenersExcept(keepSessionID string) {
	if r.sessionMgr == nil {
		return
	}
	for _, s := range r.sessionMgr.List() {
		if s == nil || s.ID == "" || s.ID == keepSessionID {
			continue
		}
		r.stopSessionDanmaku(s.ID)
	}
}

func (r *Router) handleDanmakuDisconnect(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		return
	}
	sessionID := req.PathValue("id")
	session, err := r.sessionMgr.Get(sessionID)
	if err != nil || session == nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "session not found"})
		return
	}
	r.stopSessionDanmaku(sessionID)
	if r.wsHub != nil {
		r.wsHub.BroadcastJSON(sessionID, map[string]any{
			"type":    "danmaku_status",
			"status":  "disconnected",
			"message": "danmaku listener stopped",
		})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

func (r *Router) handleDanmakuReconnect(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		return
	}
	r.handleDanmakuConnect(w, req)
}

func (r *Router) stopSessionDanmaku(sessionID string) {
	if r.danmakuScheduler != nil {
		r.danmakuScheduler.Stop(sessionID)
	}
	if r.danmakuRunner != nil {
		r.danmakuRunner.Stop(sessionID)
	}
	if r.danmakuFeed != nil {
		r.danmakuFeed.Clear(sessionID)
	}
}

func (r *Router) handleDanmakuFeed(w http.ResponseWriter, req *http.Request) {
	sessionID := req.PathValue("id")
	session, err := r.sessionMgr.Get(sessionID)
	if err != nil || session == nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "session not found"})
		return
	}
	if !session.IsOfflineLive() || !session.DanmakuEnabled() {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "session has no danmaku configured"})
		return
	}
	items := []danmaku.FeedItem{}
	if r.danmakuFeed != nil {
		if list := r.danmakuFeed.List(sessionID); list != nil {
			items = list
		}
	}
	cfg := session.DanmakuConfigSnapshot()
	status := "idle"
	relayWS := false
	if r.danmakuRunner != nil && r.danmakuRunner.Running(sessionID) {
		if cfg.Platform == "douyin" {
			relayWS = r.danmakuRunner.DouyinRelayConnected(sessionID)
			if relayWS {
				status = "connected"
			} else {
				status = "connecting"
			}
		} else {
			status = "connected"
			relayWS = true
		}
	}
	payload := map[string]any{
		"items":            items,
		"status":           status,
		"platform":         cfg.Platform,
		"listener_running": r.danmakuRunner != nil && r.danmakuRunner.Running(sessionID),
		"relay_ws_open":    relayWS,
	}
	if cfg.Platform == "douyin" {
		payload["douyin_room"] = cfg.DouyinRoom
	}
	if cfg.Platform == "bilibili" && cfg.BilibiliRoomID > 0 {
		payload["bilibili_room_id"] = cfg.BilibiliRoomID
	}
	writeJSON(w, http.StatusOK, payload)
}
