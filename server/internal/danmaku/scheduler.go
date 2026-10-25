package danmaku

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"
	"sync"
	"time"
)

const schedulerTickInterval = 30 * time.Second

// PendingMessage is one danmaku waiting for a scheduled reply.
type PendingMessage struct {
	Key       string
	Username  string
	Text      string
	Source    string
	Received  time.Time
}

// SchedulerCallbacks runs LLM replies and idle actions for offline live.
type SchedulerCallbacks struct {
	OnReply      func(sessionID, userText string)
	OnIdleAction func(sessionID string)
}

// Scheduler batches danmaku and every 30s replies to one random unreplied message,
// or triggers a random offline-live action when the queue is empty.
type Scheduler struct {
	cb SchedulerCallbacks

	mu       sync.Mutex
	sessions map[string]*schedulerSession
}

type schedulerSession struct {
	cancel        context.CancelFunc
	loopStarted   bool
	stopped       bool
	kickScheduled bool
	pending       []PendingMessage
	replied       map[string]struct{}
	mu            sync.Mutex
}

func NewScheduler(cb SchedulerCallbacks) *Scheduler {
	return &Scheduler{
		cb:       cb,
		sessions: make(map[string]*schedulerSession),
	}
}

func pendingKey(messageID, source, username, text string) string {
	messageID = strings.TrimSpace(messageID)
	if messageID != "" {
		return messageID
	}
	h := sha256.Sum256([]byte(strings.Join([]string{source, username, text, fmt.Sprintf("%d", time.Now().UnixNano())}, "\x00")))
	return "auto-" + hex.EncodeToString(h[:8])
}

func (s *Scheduler) getOrCreateSession(sessionID string) *schedulerSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess := s.sessions[sessionID]; sess != nil {
		return sess
	}
	sess := &schedulerSession{
		pending: make([]PendingMessage, 0),
		replied: make(map[string]struct{}),
	}
	s.sessions[sessionID] = sess
	return sess
}

// Enqueue adds a danmaku for later scheduled reply (does not call LLM immediately).
func (s *Scheduler) Enqueue(sessionID string, messageID, username, text, source string) {
	text = strings.TrimSpace(text)
	if text == "" || sessionID == "" {
		return
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "观众"
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "unknown"
	}
	key := pendingKey(messageID, source, username, text)

	s.mu.Lock()
	sess := s.sessions[sessionID]
	s.mu.Unlock()
	if sess == nil {
		return
	}

	sess.mu.Lock()
	if sess.stopped || !sess.loopStarted {
		sess.mu.Unlock()
		return
	}
	if _, done := sess.replied[key]; done {
		sess.mu.Unlock()
		return
	}
	for _, item := range sess.pending {
		if item.Key == key {
			sess.mu.Unlock()
			return
		}
	}
	sess.pending = append(sess.pending, PendingMessage{
		Key:      key,
		Username: username,
		Text:     text,
		Source:   source,
		Received: time.Now(),
	})
	sess.mu.Unlock()
	s.kickReply(sessionID, sess)
}

const schedulerKickDelay = 3 * time.Second

// kickReply schedules a near-term tick so the first danmaku does not wait a full 30s interval.
func (s *Scheduler) kickReply(sessionID string, sess *schedulerSession) {
	if s == nil || sess == nil {
		return
	}
	sess.mu.Lock()
	if sess.stopped || !sess.loopStarted || sess.kickScheduled {
		sess.mu.Unlock()
		return
	}
	sess.kickScheduled = true
	sess.mu.Unlock()
	go func() {
		time.Sleep(schedulerKickDelay)
		sess.mu.Lock()
		sess.kickScheduled = false
		stopped := sess.stopped || !sess.loopStarted
		sess.mu.Unlock()
		if stopped {
			return
		}
		s.tick(sessionID, sess)
	}()
}

// Start begins the 30s tick loop for a session. Idempotent per sessionID.
func (s *Scheduler) Start(sessionID string) {
	if s == nil || sessionID == "" {
		return
	}
	sess := s.getOrCreateSession(sessionID)
	sess.mu.Lock()
	if sess.loopStarted {
		sess.mu.Unlock()
		return
	}
	sess.loopStarted = true
	sess.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	sess.mu.Lock()
	sess.cancel = cancel
	sess.mu.Unlock()

	go s.runLoop(ctx, sessionID, sess)
}

func (s *Scheduler) runLoop(ctx context.Context, sessionID string, sess *schedulerSession) {
	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(sessionID, sess)
		}
	}
}

func (s *Scheduler) tick(sessionID string, sess *schedulerSession) {
	if s.cb.OnReply == nil && s.cb.OnIdleAction == nil {
		return
	}

	sess.mu.Lock()
	if sess.stopped {
		sess.mu.Unlock()
		return
	}
	var pick *PendingMessage
	remaining := make([]PendingMessage, 0, len(sess.pending))
	for i := range sess.pending {
		item := sess.pending[i]
		if _, done := sess.replied[item.Key]; done {
			continue
		}
		remaining = append(remaining, item)
	}
	sess.pending = remaining
	if len(remaining) > 0 {
		idx := rand.IntN(len(remaining))
		chosen := remaining[idx]
		sess.replied[chosen.Key] = struct{}{}
		pick = &chosen
	}
	sess.mu.Unlock()

	if pick != nil {
		userText := DanmakuTextForLLM(pick.Username, pick.Text)
		log.Printf("danmaku scheduler: reply session=%s key=%s", sessionID, pick.Key)
		s.cb.OnReply(sessionID, userText)
		return
	}

	if s.cb.OnIdleAction != nil {
		log.Printf("danmaku scheduler: idle action session=%s", sessionID)
		s.cb.OnIdleAction(sessionID)
	}
}

// Stop ends the tick loop and drops queued danmaku for the session.
func (s *Scheduler) Stop(sessionID string) {
	if s == nil || sessionID == "" {
		return
	}
	s.mu.Lock()
	sess := s.sessions[sessionID]
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	if sess == nil {
		return
	}
	sess.mu.Lock()
	sess.stopped = true
	cancel := sess.cancel
	sess.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// StopAll stops every session loop.
func (s *Scheduler) StopAll() {
	if s == nil {
		return
	}
	s.mu.Lock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.Stop(id)
	}
}
