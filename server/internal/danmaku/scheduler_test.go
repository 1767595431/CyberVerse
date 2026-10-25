package danmaku

import (
	"strings"
	"sync"
	"testing"
)

func TestSchedulerRepliesOncePerMessage(t *testing.T) {
	var mu sync.Mutex
	replies := make([]string, 0)
	idle := 0

	s := NewScheduler(SchedulerCallbacks{
		OnReply: func(_ string, userText string) {
			mu.Lock()
			replies = append(replies, userText)
			mu.Unlock()
		},
		OnIdleAction: func(_ string) {
			mu.Lock()
			idle++
			mu.Unlock()
		},
	})
	s.Start("sess-1")
	s.Enqueue("sess-1", "m1", "alice", "hello", "bilibili")
	s.Enqueue("sess-1", "m2", "bob", "world", "bilibili")

	s.tick("sess-1", s.getOrCreateSession("sess-1"))
	s.tick("sess-1", s.getOrCreateSession("sess-1"))

	mu.Lock()
	defer mu.Unlock()
	if len(replies) != 2 {
		t.Fatalf("expected 2 replies, got %d: %v", len(replies), replies)
	}
	for _, reply := range replies {
		if strings.Contains(reply, "：") {
			t.Fatalf("LLM input must not include nickname prefix, got %q", reply)
		}
	}
	if idle != 0 {
		t.Fatalf("expected no idle actions, got %d", idle)
	}
}

func TestSchedulerIdleWhenQueueEmpty(t *testing.T) {
	idle := 0
	s := NewScheduler(SchedulerCallbacks{
		OnIdleAction: func(_ string) {
			idle++
		},
	})
	s.Start("sess-2")
	s.tick("sess-2", s.getOrCreateSession("sess-2"))
	if idle != 1 {
		t.Fatalf("expected 1 idle action, got %d", idle)
	}
}

func TestSchedulerIdlePausedWhileSong(t *testing.T) {
	idle := 0
	s := NewScheduler(SchedulerCallbacks{
		OnIdleAction: func(_ string) {
			idle++
		},
	})
	s.Start("sess-pause")
	s.PauseIdleActions("sess-pause")
	s.tick("sess-pause", s.getOrCreateSession("sess-pause"))
	if idle != 0 {
		t.Fatalf("expected no idle while paused, got %d", idle)
	}
	s.ResumeIdleActions("sess-pause")
	s.tick("sess-pause", s.getOrCreateSession("sess-pause"))
	if idle != 1 {
		t.Fatalf("expected 1 idle after resume, got %d", idle)
	}
}

func TestSchedulerDoesNotReplyTwice(t *testing.T) {
	replies := 0
	s := NewScheduler(SchedulerCallbacks{
		OnReply: func(_ string, _ string) {
			replies++
		},
	})
	s.Start("sess-3")
	s.Enqueue("sess-3", "fixed-id", "u", "same", "douyin")
	sess := s.getOrCreateSession("sess-3")
	for i := 0; i < 3; i++ {
		s.tick("sess-3", sess)
	}
	if replies != 1 {
		t.Fatalf("expected 1 reply for same message, got %d", replies)
	}
}
