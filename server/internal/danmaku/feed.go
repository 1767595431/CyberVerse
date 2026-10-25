package danmaku

import "sync"

const maxFeedItems = 120

// FeedItem is one danmaku shown in the offline-live UI feed.
type FeedItem struct {
	Username  string `json:"username"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	MessageID string `json:"message_id,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// FeedStore keeps a rolling window of recent danmaku per session for WS reconnect sync.
type FeedStore struct {
	mu        sync.Mutex
	bySession map[string][]FeedItem
}

func NewFeedStore() *FeedStore {
	return &FeedStore{bySession: make(map[string][]FeedItem)}
}

func (f *FeedStore) Add(sessionID string, item FeedItem) {
	if f == nil || sessionID == "" || item.Text == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	items := append(f.bySession[sessionID], item)
	if len(items) > maxFeedItems {
		items = items[len(items)-maxFeedItems:]
	}
	f.bySession[sessionID] = items
}

func (f *FeedStore) List(sessionID string) []FeedItem {
	if f == nil || sessionID == "" {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	src := f.bySession[sessionID]
	if len(src) == 0 {
		return nil
	}
	out := make([]FeedItem, len(src))
	copy(out, src)
	return out
}

func (f *FeedStore) Clear(sessionID string) {
	if f == nil || sessionID == "" {
		return
	}
	f.mu.Lock()
	delete(f.bySession, sessionID)
	f.mu.Unlock()
}
