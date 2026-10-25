package danmaku

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// SessionConfig selects a live platform listener for one offline-live session.
type SessionConfig struct {
	Platform       string `json:"platform"`
	BilibiliRoomID int    `json:"bilibili_room_id"`
	DouyinRoom     string `json:"douyin_room"`
	DouyinRelay    string `json:"douyin_relay"`
}

type workerConfig struct {
	SessionID string         `json:"session_id"`
	APIBase   string         `json:"api_base"`
	Live      workerLive     `json:"live"`
	Bilibili  workerBilibili `json:"bilibili"`
}

type workerLive struct {
	Platform    string `json:"platform"`
	DouyinRoom  string `json:"douyin_room"`
	DouyinRelay string `json:"douyin_relay"`
}

type workerBilibili struct {
	Mode   string `json:"mode"`
	RoomID int    `json:"room_id"`
}

type douyinListener struct {
	ctx      context.Context
	cancel   context.CancelFunc
	relayWS  atomic.Bool
}

// InjectHandler ingests one danmaku into the session (feed, WS, scheduler).
type InjectHandler func(sessionID, username, text, source, messageID string) error

// RelayConnectedHandler is called when the DyHub WebSocket for a session becomes live.
type RelayConnectedHandler func(sessionID string)

// Runner supervises danmaku ingestion: Douyin via in-process DyHub WS (Go), Bilibili via Python worker.
type Runner struct {
	mu        sync.Mutex
	processes map[string]*exec.Cmd
	douyin    map[string]*douyinListener
	inject    InjectHandler
	onRelay   RelayConnectedHandler
	apiBase   string
	python    string
	repoRoot  string
}

func (r *Runner) SetInjectHandler(h InjectHandler) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.inject = h
	r.mu.Unlock()
}

func (r *Runner) SetRelayConnectedHandler(h RelayConnectedHandler) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.onRelay = h
	r.mu.Unlock()
}

// DouyinRelayConnected reports whether the session's DyHub WebSocket is currently up.
func (r *Runner) DouyinRelayConnected(sessionID string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	h := r.douyin[sessionID]
	r.mu.Unlock()
	return h != nil && h.relayWS.Load()
}

func NewRunner(apiBase, repoRoot string) *Runner {
	python := os.Getenv("CYBERVERSE_PYTHON")
	if python == "" {
		python = os.Getenv("PYTHON")
	}
	if python == "" {
		python = "/root/autodl-tmp/envs/cyberverse/bin/python"
	}
	if _, err := os.Stat(python); err != nil {
		python = "python3"
	}
	return &Runner{
		processes: make(map[string]*exec.Cmd),
		douyin:    make(map[string]*douyinListener),
		apiBase:   apiBase,
		python:    python,
		repoRoot:  repoRoot,
	}
}

func (r *Runner) Start(sessionID string, cfg SessionConfig) error {
	if r == nil {
		return fmt.Errorf("danmaku runner is not configured")
	}
	platform := cfg.Platform
	if platform == "" || platform == "none" {
		return nil
	}
	if platform != "bilibili" && platform != "douyin" {
		return fmt.Errorf("unsupported danmaku platform %q", platform)
	}

	if r.Running(sessionID) {
		return nil
	}
	r.Stop(sessionID)

	relay := cfg.DouyinRelay
	if relay == "" {
		relay = defaultDyhubRelay
	}
	if platform == "douyin" {
		ensureCtx, cancel := context.WithTimeout(context.Background(), 13*time.Minute)
		defer cancel()
		if err := EnsureDyhubRelay(ensureCtx, relay, r.repoRoot); err != nil {
			return fmt.Errorf("douyin dyhub relay: %w", err)
		}
		ctx, runCancel := context.WithCancel(context.Background())
		handle := &douyinListener{ctx: ctx, cancel: runCancel}
		r.mu.Lock()
		r.douyin[sessionID] = handle
		r.mu.Unlock()
		go func(sid string, listenCtx context.Context) {
			defer func() {
				r.mu.Lock()
				if cur, ok := r.douyin[sid]; ok && cur != nil && cur.ctx == listenCtx {
					delete(r.douyin, sid)
				}
				r.mu.Unlock()
			}()
			runDouyinRelayListener(listenCtx, sid, cfg, handle, r.injectMessage, r.relayConnected)
		}(sessionID, ctx)
		log.Printf("danmaku douyin listener started session=%s (Go DyHub client)", sessionID)
		return nil
	}

	payload := workerConfig{
		SessionID: sessionID,
		APIBase:   r.apiBase,
		Live: workerLive{
			Platform:    platform,
			DouyinRoom:  cfg.DouyinRoom,
			DouyinRelay: relay,
		},
		Bilibili: workerBilibili{
			Mode:   "web",
			RoomID: cfg.BilibiliRoomID,
		},
	}
	configPath := filepath.Join(os.TempDir(), fmt.Sprintf("cyberverse-danmaku-%s.json", sessionID))
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return err
	}

	script := filepath.Join(r.repoRoot, "scripts", "offline_live_danmaku_worker.py")
	if absScript, err := filepath.Abs(script); err == nil {
		script = absScript
	}
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("danmaku worker script missing: %w", err)
	}

	cmd := exec.Command(r.python, script, "--config", configPath)
	cmd.Dir = r.repoRoot
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
	var workerLog *os.File
	logPath := filepath.Join(r.repoRoot, "runtime", "danmaku-worker.log")
	if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		workerLog = logFile
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		_, _ = logFile.WriteString(fmt.Sprintf("\n--- danmaku worker session=%s platform=%s ---\n", sessionID, platform))
	}
	if err := cmd.Start(); err != nil {
		if workerLog != nil {
			_ = workerLog.Close()
		}
		return fmt.Errorf("start danmaku worker: %w", err)
	}

	r.mu.Lock()
	r.processes[sessionID] = cmd
	r.mu.Unlock()

	go func() {
		waitErr := cmd.Wait()
		if workerLog != nil {
			_ = workerLog.Close()
		}
		r.mu.Lock()
		if r.processes[sessionID] == cmd {
			delete(r.processes, sessionID)
		}
		r.mu.Unlock()
		_ = os.Remove(configPath)
		if waitErr != nil {
			log.Printf("danmaku worker exited session=%s: %v", sessionID, waitErr)
		}
	}()

	log.Printf("danmaku worker started session=%s platform=%s", sessionID, platform)
	return nil
}

// Running reports whether danmaku ingestion is active for the session.
func (r *Runner) Running(sessionID string) bool {
	if r == nil {
		return false
	}
	if r.workerRunning(sessionID) {
		return true
	}
	r.mu.Lock()
	_, ok := r.douyin[sessionID]
	r.mu.Unlock()
	return ok
}

func (r *Runner) workerRunning(sessionID string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	cmd := r.processes[sessionID]
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return false
	}
	return cmd.Process.Signal(syscall.Signal(0)) == nil
}

func (r *Runner) Stop(sessionID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if h, ok := r.douyin[sessionID]; ok {
		delete(r.douyin, sessionID)
		if h != nil && h.cancel != nil {
			h.cancel()
		}
	}
	cmd := r.processes[sessionID]
	delete(r.processes, sessionID)
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	go func() {
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
	}()
}

func (r *Runner) injectMessage(sessionID, username, text, source, messageID string) error {
	r.mu.Lock()
	h := r.inject
	apiBase := r.apiBase
	r.mu.Unlock()
	if h != nil {
		return h(sessionID, username, text, source, messageID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return postDanmakuInject(ctx, apiBase, sessionID, douyinInjectPayload{
		Username:  username,
		Text:      text,
		Source:    source,
		MessageID: messageID,
	})
}

func (r *Runner) relayConnected(sessionID string) {
	r.mu.Lock()
	h := r.onRelay
	r.mu.Unlock()
	if h != nil {
		h(sessionID)
	}
}

func (r *Runner) StopAll() {
	if r == nil {
		return
	}
	r.mu.Lock()
	ids := make([]string, 0, len(r.processes)+len(r.douyin))
	for id := range r.processes {
		ids = append(ids, id)
	}
	for id := range r.douyin {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		r.Stop(id)
	}
}
