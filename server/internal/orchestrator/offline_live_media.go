package orchestrator

import "strings"

// OfflineLiveActionClip is a pre-rendered action MP4 bound to an offline-live session.
type OfflineLiveActionClip struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

func normalizeOfflineLiveActionClips(clips []OfflineLiveActionClip) []OfflineLiveActionClip {
	if len(clips) == 0 {
		return nil
	}
	out := make([]OfflineLiveActionClip, 0, len(clips))
	for _, clip := range clips {
		id := strings.TrimSpace(clip.ID)
		title := strings.TrimSpace(clip.Title)
		url := strings.TrimSpace(clip.URL)
		if id == "" || url == "" {
			continue
		}
		if title == "" {
			title = id
		}
		out = append(out, OfflineLiveActionClip{ID: id, Title: title, URL: url})
	}
	return out
}

// SetOfflineLiveMedia stores idle, action, and singing clips for offline-live playback control.
func (s *Session) SetOfflineLiveMedia(idleURL string, actions, songs []OfflineLiveActionClip) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.OfflineLiveIdleVideoURL = strings.TrimSpace(idleURL)
	s.OfflineLiveActions = normalizeOfflineLiveActionClips(actions)
	s.OfflineLiveSongs = normalizeOfflineLiveActionClips(songs)
}

func (s *Session) OfflineLiveMediaSnapshot() (idleURL string, actions, songs []OfflineLiveActionClip) {
	if s == nil {
		return "", nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	idleURL = s.OfflineLiveIdleVideoURL
	if len(s.OfflineLiveActions) > 0 {
		actions = append([]OfflineLiveActionClip(nil), s.OfflineLiveActions...)
	}
	if len(s.OfflineLiveSongs) > 0 {
		songs = append([]OfflineLiveActionClip(nil), s.OfflineLiveSongs...)
	}
	return idleURL, actions, songs
}

// FindOfflineLiveAction resolves by id (preferred) or exact title match.
func (s *Session) FindOfflineLiveAction(actionID, actionTitle string) (*OfflineLiveActionClip, bool) {
	if s == nil {
		return nil, false
	}
	actionID = strings.TrimSpace(actionID)
	actionTitle = strings.TrimSpace(actionTitle)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if actionID != "" {
		for i := range s.OfflineLiveActions {
			if s.OfflineLiveActions[i].ID == actionID {
				clip := s.OfflineLiveActions[i]
				return &clip, true
			}
		}
	}
	if actionTitle != "" {
		for i := range s.OfflineLiveActions {
			if s.OfflineLiveActions[i].Title == actionTitle {
				clip := s.OfflineLiveActions[i]
				return &clip, true
			}
		}
	}
	return nil, false
}

// FindOfflineLiveSong resolves a singing clip by id (preferred) or exact title match.
func (s *Session) FindOfflineLiveSong(songID, songTitle string) (*OfflineLiveActionClip, bool) {
	if s == nil {
		return nil, false
	}
	songID = strings.TrimSpace(songID)
	songTitle = strings.TrimSpace(songTitle)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if songID != "" {
		for i := range s.OfflineLiveSongs {
			if s.OfflineLiveSongs[i].ID == songID {
				clip := s.OfflineLiveSongs[i]
				return &clip, true
			}
		}
	}
	if songTitle != "" {
		for i := range s.OfflineLiveSongs {
			if s.OfflineLiveSongs[i].Title == songTitle {
				clip := s.OfflineLiveSongs[i]
				return &clip, true
			}
		}
	}
	return nil, false
}
