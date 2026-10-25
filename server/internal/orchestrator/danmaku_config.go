package orchestrator

import "strings"

// DanmakuConfig holds live-room listener settings for offline live.
type DanmakuConfig struct {
	Platform       string
	BilibiliRoomID int
	DouyinRoom     string
	DouyinRelay    string
}

func (s *Session) SetDanmakuConfig(cfg DanmakuConfig) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.DanmakuPlatform = strings.TrimSpace(cfg.Platform)
	s.DanmakuBilibiliRoomID = cfg.BilibiliRoomID
	s.DanmakuDouyinRoom = strings.TrimSpace(cfg.DouyinRoom)
	s.DanmakuDouyinRelay = strings.TrimSpace(cfg.DouyinRelay)
}

func (s *Session) DanmakuConfigSnapshot() DanmakuConfig {
	if s == nil {
		return DanmakuConfig{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return DanmakuConfig{
		Platform:       s.DanmakuPlatform,
		BilibiliRoomID: s.DanmakuBilibiliRoomID,
		DouyinRoom:     s.DanmakuDouyinRoom,
		DouyinRelay:    s.DanmakuDouyinRelay,
	}
}

func (s *Session) DanmakuEnabled() bool {
	cfg := s.DanmakuConfigSnapshot()
	p := cfg.Platform
	return p != "" && p != "none"
}
