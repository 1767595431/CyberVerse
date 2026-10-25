package danmaku

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var douyinRoomRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// NormalizeDouyinRoom accepts a room id or live.douyin.com URL (same rules as live-danmaku/douyin.py).
func NormalizeDouyinRoom(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("douyin room is required")
	}
	lower := strings.ToLower(raw)
	if strings.Contains(raw, "://") || strings.HasPrefix(lower, "live.douyin.com/") || strings.HasPrefix(lower, "www.douyin.com/") {
		toParse := raw
		if !strings.Contains(raw, "://") {
			toParse = "https://" + raw
		}
		u, err := url.Parse(toParse)
		if err != nil {
			return "", errors.New("invalid douyin live URL")
		}
		host := u.Hostname()
		if host != "live.douyin.com" && host != "www.douyin.com" {
			return "", errors.New("invalid douyin live URL host")
		}
		q := u.Query()
		for _, key := range []string{"live_web_rid", "web_rid", "room_id", "roomid"} {
			if v := strings.TrimSpace(q.Get(key)); v != "" {
				if !douyinRoomRE.MatchString(v) {
					return "", errors.New("invalid douyin room id in URL")
				}
				return v, nil
			}
		}
		segments := strings.Split(strings.Trim(u.Path, "/"), "/")
		if host != "live.douyin.com" || len(segments) != 1 {
			return "", errors.New("douyin URL must contain live_web_rid or live.douyin.com/{roomId}")
		}
		raw = segments[0]
	} else if strings.Contains(raw, "/") {
		return "", errors.New("invalid douyin room id")
	}
	if !douyinRoomRE.MatchString(raw) {
		return "", errors.New("invalid douyin room id format")
	}
	return raw, nil
}

func normalizeDyhubRelayBase(raw string) (string, error) {
	raw = strings.TrimSpace(strings.TrimRight(raw, "/"))
	if raw == "" {
		raw = defaultDyhubRelay
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("douyin relay must be http or https")
	}
	host := u.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return "", errors.New("douyin relay must be loopback")
	}
	if u.User != nil {
		return "", errors.New("douyin relay must not embed credentials")
	}
	return strings.TrimRight(raw, "/"), nil
}

type douyinInjectPayload struct {
	Username  string `json:"username"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	MessageID string `json:"message_id"`
}

func postDanmakuInject(ctx context.Context, apiBase, sessionID string, body douyinInjectPayload) error {
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	url := fmt.Sprintf("%s/api/v1/sessions/%s/danmaku/inject", apiBase, sessionID)
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("danmaku inject HTTP %d", resp.StatusCode)
	}
	return nil
}

func dyhubWSURL(relayBase, roomID string) (string, error) {
	u, err := url.Parse(relayBase)
	if err != nil {
		return "", err
	}
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}
	u.Scheme = scheme
	path := strings.TrimRight(u.Path, "/") + "/ws"
	u.Path = path
	q := u.Query()
	q.Set("roomId", roomID)
	q.Set("types", "chat")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func connectDyhubRoom(ctx context.Context, relayBase, roomID string) error {
	url := relayBase + "/api/rooms/connect"
	payload, _ := json.Marshal(map[string]string{"roomId": roomID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("dyhub relay unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("dyhub connect HTTP %d", resp.StatusCode)
	}
	return nil
}

// runDouyinRelayListener blocks until ctx is cancelled; reconnects DyHub WS on errors.
func runDouyinRelayListener(
	ctx context.Context,
	sessionID string,
	cfg SessionConfig,
	handle *douyinListener,
	inject func(sessionID, username, text, source, messageID string) error,
	onRelayConnected func(sessionID string),
) {
	roomID, err := NormalizeDouyinRoom(cfg.DouyinRoom)
	if err != nil {
		log.Printf("danmaku douyin session=%s: %v", sessionID, err)
		return
	}
	relay, err := normalizeDyhubRelayBase(cfg.DouyinRelay)
	if err != nil {
		log.Printf("danmaku douyin session=%s: %v", sessionID, err)
		return
	}
	wsURL, err := dyhubWSURL(relay, roomID)
	if err != nil {
		log.Printf("danmaku douyin session=%s: %v", sessionID, err)
		return
	}

	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	backoff := 2 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		connectCtx, cancelConnect := context.WithTimeout(ctx, 30*time.Second)
		err := connectDyhubRoom(connectCtx, relay, roomID)
		cancelConnect()
		if err != nil {
			log.Printf("danmaku douyin session=%s dyhub connect: %v", sessionID, err)
			if !sleepCtx(ctx, backoff) {
				return
			}
			continue
		}

		conn, _, err := dialer.DialContext(ctx, wsURL, nil)
		if err != nil {
			log.Printf("danmaku douyin session=%s dyhub ws: %v", sessionID, err)
			if !sleepCtx(ctx, backoff) {
				return
			}
			continue
		}

		if handle != nil {
			handle.relayWS.Store(true)
		}
		if onRelayConnected != nil {
			onRelayConnected(sessionID)
		}
		log.Printf("danmaku douyin relay connected session=%s room=%s", sessionID, roomID)
		readErr := readDyhubChatWS(ctx, conn, func(username, text, messageID string) {
			username = strings.TrimSpace(username)
			if username == "" {
				username = "观众"
			}
			text = strings.TrimSpace(text)
			if text == "" {
				return
			}
			if inject == nil {
				return
			}
			if err := inject(sessionID, username, text, "douyin", messageID); err != nil {
				log.Printf("danmaku douyin inject session=%s: %v", sessionID, err)
				return
			}
			log.Printf("danmaku chat session=%s user=%s text=%q", sessionID, username, text)
		})
		if handle != nil {
			handle.relayWS.Store(false)
		}
		_ = conn.Close()

		if ctx.Err() != nil {
			return
		}
		if readErr != nil && !errors.Is(readErr, context.Canceled) {
			log.Printf("danmaku douyin session=%s ws read: %v", sessionID, readErr)
		}
		if !sleepCtx(ctx, 5*time.Second) {
			return
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func readDyhubChatWS(ctx context.Context, conn *websocket.Conn, onChat func(username, text, messageID string)) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var event map[string]any
		if err := json.Unmarshal(data, &event); err != nil {
			continue
		}
		typ, _ := event["type"].(string)
		if typ != "chat" {
			continue
		}
		content := ""
		if dataObj, ok := event["data"].(map[string]any); ok {
			if c, ok := dataObj["content"].(string); ok {
				content = c
			}
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		username := "抖音观众"
		if userObj, ok := event["user"].(map[string]any); ok {
			if n, ok := userObj["nickname"].(string); ok && strings.TrimSpace(n) != "" {
				username = n
			} else if n, ok := userObj["nickName"].(string); ok && strings.TrimSpace(n) != "" {
				username = n
			}
		}
		msgID := ""
		if id, ok := event["id"].(string); ok {
			msgID = id
		}
		onChat(username, content, msgID)
	}
}
