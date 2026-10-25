package api

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cyberverse/server/internal/agenttask"
	"github.com/cyberverse/server/internal/character"
	"github.com/cyberverse/server/internal/config"
	"github.com/cyberverse/server/internal/danmaku"
	"github.com/cyberverse/server/internal/livekit"
	"github.com/cyberverse/server/internal/orchestrator"
	ragstore "github.com/cyberverse/server/internal/rag"
	"github.com/cyberverse/server/internal/ws"
)

type Router struct {
	sessionMgr *orchestrator.SessionManager
	orch       *orchestrator.Orchestrator
	wsHub      *ws.Hub
	roomMgr    *livekit.RoomManager
	taskSvc    *agenttask.Service
	cfg        *config.Config
	charStore  *character.Store
	ragStore   *ragstore.Store
	envPath    string
	configPath string
	modelsDir      string
	danmakuRunner    *danmaku.Runner
	danmakuScheduler *danmaku.Scheduler
	danmakuFeed      *danmaku.FeedStore
	mux              *http.ServeMux
}

func (r *Router) StopDanmakuWorkers() {
	if r == nil {
		return
	}
	if r.danmakuRunner != nil {
		r.danmakuRunner.StopAll()
	}
	if r.danmakuScheduler != nil {
		r.danmakuScheduler.StopAll()
	}
}

func NewRouter(
	sessionMgr *orchestrator.SessionManager,
	orch *orchestrator.Orchestrator,
	wsHub *ws.Hub,
	roomMgr *livekit.RoomManager,
	cfg *config.Config,
	charStore *character.Store,
	envPath string,
	configPath string,
	taskServices ...*agenttask.Service,
) *Router {
	r := &Router{
		sessionMgr: sessionMgr,
		orch:       orch,
		wsHub:      wsHub,
		roomMgr:    roomMgr,
		cfg:        cfg,
		charStore:  charStore,
		ragStore:   ragstore.NewStore(charStore),
		envPath:    envPath,
		configPath: configPath,
		modelsDir:  filepath.Join(filepath.Dir(configPath), "models"),
		mux:        http.NewServeMux(),
	}
	if len(taskServices) > 0 {
		r.taskSvc = taskServices[0]
	}
	repoRoot := filepath.Dir(configPath)
	if filepath.Base(repoRoot) == "config" {
		repoRoot = filepath.Dir(repoRoot)
	}
	if absRoot, err := filepath.Abs(repoRoot); err == nil {
		repoRoot = absRoot
	}
	apiBase := "http://127.0.0.1:6006"
	if cfg != nil && cfg.Server.HTTPPort > 0 {
		apiBase = fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.HTTPPort)
	}
	r.danmakuRunner = danmaku.NewRunner(apiBase, repoRoot)
	r.danmakuRunner.SetInjectHandler(func(sessionID, username, text, source, messageID string) error {
		return r.injectDanmaku(sessionID, danmakuInjectRequest{
			Username:  username,
			Text:      text,
			Source:    source,
			MessageID: messageID,
		})
	})
	r.danmakuRunner.SetRelayConnectedHandler(func(sessionID string) {
		session, err := r.sessionMgr.Get(sessionID)
		if err != nil || session == nil {
			return
		}
		cfg := session.DanmakuConfigSnapshot()
		if r.wsHub != nil {
			r.wsHub.BroadcastJSON(sessionID, map[string]any{
				"type":     "danmaku_status",
				"status":   "connected",
				"platform": cfg.Platform,
			})
		}
		log.Printf("danmaku dyhub ws live session=%s platform=%s", sessionID, cfg.Platform)
	})
	r.danmakuFeed = danmaku.NewFeedStore()
	r.danmakuScheduler = danmaku.NewScheduler(danmaku.SchedulerCallbacks{
		OnReply: func(sessionID, userText string) {
			if r.orch == nil || r.sessionMgr == nil {
				return
			}
			if _, err := r.sessionMgr.Get(sessionID); err != nil {
				return
			}
			ctx := orchestrator.ContextForDanmakuReply(context.Background())
			if err := r.orch.HandleTextInput(ctx, sessionID, userText); err != nil {
				log.Printf("scheduled danmaku reply failed session=%s: %v", sessionID, err)
			}
		},
		OnIdleAction: func(sessionID string) {
			r.triggerRandomOfflineLiveAction(sessionID)
		},
	})
	// WebSocket upgrades share the CORS origin allowlist enforced by
	// corsMiddleware so both HTTP and WS reject cross-origin clients.
	// Non-browser clients without an Origin header are always allowed.
	ws.SetCheckOrigin(nil)
	r.registerRoutes()
	if sessionMgr != nil {
		prevOnSessionEnd := sessionMgr.OnSessionEnd
		sessionMgr.OnSessionEnd = func(s *orchestrator.Session) {
			if s != nil {
				r.stopSessionDanmaku(s.ID)
			}
			if prevOnSessionEnd != nil {
				prevOnSessionEnd(s)
			}
		}
	}
	return r
}

func (r *Router) registerRoutes() {
	r.mux.HandleFunc("GET /api/v1/health", r.handleHealth)
	r.mux.HandleFunc("GET /api/v1/components", r.handleListComponents)
	r.mux.HandleFunc("GET /api/v1/baidu-xiling/figures/{figure_id}", r.handleGetBaiduXilingFigure)
	r.mux.HandleFunc("GET /api/v1/xunfei/avatars/{avatar_id}", r.handleGetXunfeiAvatar)
	r.mux.HandleFunc("GET /api/v1/xunfei/sessions/{id}/stream.flv", r.handleXunfeiAvatarStream)
	r.mux.HandleFunc("POST /api/v1/sessions", r.handleCreateSession)
	r.mux.HandleFunc("GET /api/v1/sessions/{id}/turns/{turn}/video", r.handleGetSessionTurnVideo)
	r.mux.HandleFunc("DELETE /api/v1/sessions/{id}", r.handleDeleteSession)
	r.mux.HandleFunc("POST /api/v1/sessions/{id}/message", r.handleSendMessage)
	r.mux.HandleFunc("POST /api/v1/sessions/{id}/offline-live/action", r.handleTriggerOfflineLiveAction)
	r.mux.HandleFunc("POST /api/v1/sessions/{id}/offline-live/song", r.handleTriggerOfflineLiveSong)
	r.mux.HandleFunc("POST /api/v1/sessions/{id}/danmaku/inject", r.handleDanmakuInject)
	r.mux.HandleFunc("GET /api/v1/sessions/{id}/danmaku/feed", r.handleDanmakuFeed)
	r.mux.HandleFunc("POST /api/v1/sessions/{id}/danmaku/connect", r.handleDanmakuConnect)
	r.mux.HandleFunc("POST /api/v1/sessions/{id}/danmaku/disconnect", r.handleDanmakuDisconnect)
	r.mux.HandleFunc("POST /api/v1/sessions/{id}/danmaku/reconnect", r.handleDanmakuReconnect)
	r.mux.HandleFunc("GET /api/v1/sessions/{id}/tasks", r.handleListSessionTasks)
	r.mux.HandleFunc("GET /api/v1/sessions", r.handleListSessions)
	r.mux.HandleFunc("GET /api/v1/tasks/{task_id}", r.handleGetTask)
	r.mux.HandleFunc("GET /api/v1/tasks/{task_id}/events", r.handleListTaskEvents)
	r.mux.HandleFunc("GET /api/v1/tasks/{task_id}/artifacts/{artifact_id}", r.handleGetTaskArtifact)
	r.mux.HandleFunc("POST /api/v1/internal/tasks/{task_id}/events", r.handleInternalTaskEvent)
	r.mux.HandleFunc("POST /api/v1/internal/tasks/{task_id}/artifacts", r.handleInternalTaskArtifact)
	r.mux.HandleFunc("POST /api/v1/internal/characters/{id}/knowledge/search", r.handleInternalKnowledgeSearch)
	r.mux.HandleFunc("GET /ws/chat/{id}", r.handleWebSocket)

	// Character CRUD
	r.mux.HandleFunc("GET /api/v1/characters", r.handleListCharacters)
	r.mux.HandleFunc("POST /api/v1/characters", r.handleCreateCharacter)
	r.mux.HandleFunc("POST /api/v1/characters/test-voice", r.handleTestCharacterVoice)
	r.mux.HandleFunc("GET /api/v1/characters/{id}", r.handleGetCharacter)
	r.mux.HandleFunc("PUT /api/v1/characters/{id}", r.handleUpdateCharacter)
	r.mux.HandleFunc("PUT /api/v1/characters/{id}/offline-video-tts", r.handleUpdateCharacterOfflineVideoTTS)
	r.mux.HandleFunc("DELETE /api/v1/characters/{id}", r.handleDeleteCharacter)
	r.mux.HandleFunc("POST /api/v1/characters/{id}/avatar", r.handleUploadAvatar)
	r.mux.HandleFunc("GET /api/v1/characters/{id}/images", r.handleListImages)
	r.mux.HandleFunc("GET /api/v1/characters/{id}/images/{filename}", r.handleGetCharacterImage)
	r.mux.HandleFunc("GET /api/v1/characters/{id}/offline-videos", r.handleListOfflineVideos)
	r.mux.HandleFunc("POST /api/v1/characters/{id}/offline-videos", r.handleCreateOfflineVideo)
	r.mux.HandleFunc("GET /api/v1/characters/{id}/offline-videos/{job_id}", r.handleGetOfflineVideo)
	r.mux.HandleFunc("PATCH /api/v1/characters/{id}/offline-videos/{job_id}", r.handleUpdateOfflineVideo)
	r.mux.HandleFunc("GET /api/v1/characters/{id}/offline-videos/{job_id}/video", r.handleGetOfflineVideoFile)
	r.mux.HandleFunc("DELETE /api/v1/characters/{id}/offline-videos/{job_id}", r.handleDeleteOfflineVideo)
	r.mux.HandleFunc("GET /api/v1/characters/{id}/knowledge", r.handleListKnowledgeSources)
	r.mux.HandleFunc("POST /api/v1/characters/{id}/knowledge/files", r.handleUploadKnowledgeFiles)
	r.mux.HandleFunc("DELETE /api/v1/characters/{id}/knowledge/{source_id}", r.handleDeleteKnowledgeSource)
	r.mux.HandleFunc("POST /api/v1/characters/{id}/knowledge/{source_id}/reindex", r.handleReindexKnowledgeSource)
	r.mux.HandleFunc("GET /api/v1/characters/{id}/idle-videos/{imgbase}/{variant}/{filename}", r.handleGetIdleVideo)
	r.mux.HandleFunc("GET /api/v1/characters/{id}/idle-videos/{imgbase}/{filename}", r.handleGetIdleVideo)
	r.mux.HandleFunc("DELETE /api/v1/characters/{id}/images/{filename}", r.handleDeleteImage)
	r.mux.HandleFunc("PUT /api/v1/characters/{id}/images/{filename}/activate", r.handleActivateImage)
	r.mux.HandleFunc("GET /api/v1/avatars/{filename}", r.handleGetAvatar)

	// Conversation history
	r.mux.HandleFunc("GET /api/v1/characters/{id}/conversations/messages", r.handleGetConversationMessages)

	// Settings
	r.mux.HandleFunc("GET /api/v1/settings", r.handleGetSettings)
	r.mux.HandleFunc("PUT /api/v1/settings", r.handleUpdateSettings)
	r.mux.HandleFunc("POST /api/v1/settings/test", r.handleTestConnection)
	r.mux.HandleFunc("GET /action/panel", r.handleActionPanel)
	r.mux.HandleFunc("GET /action/set", r.handleActionSet)
	r.mux.HandleFunc("GET /action/res", r.handleActionRes)
	r.mux.HandleFunc("GET /action/ping", r.handleActionPing)

	// Launch config
	r.mux.HandleFunc("GET /api/v1/config/avatar-model", r.handleGetAvatarModelInfo)
	r.mux.HandleFunc("GET /api/v1/config/launch", r.handleGetLaunchConfig)
	r.mux.HandleFunc("PUT /api/v1/config/launch", r.handleUpdateLaunchConfig)
}

func (r *Router) Handler() http.Handler {
	return r.corsMiddleware(r.mux)
}

// corsMiddleware enforces an origin allowlist on cross-origin requests.
// Requests without an Origin header (curl, tests, same-origin) are unaffected.
func (r *Router) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		origin := req.Header.Get("Origin")
		if origin == "" {
			// Non-browser client or same-origin request; keep the wildcard
			// behavior so plain HTTP clients are not affected.
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		} else if r.originAllowed(origin) {
			// Echo the exact origin instead of "*" so credentials could be
			// used later and only allowlisted origins receive the header.
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if req.Method == http.MethodOptions {
			// Preflight: reject explicitly when the origin is not allowed.
			if origin != "" && !r.originAllowed(origin) {
				writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "origin not allowed"})
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, req)
	})
}

// originAllowed reports whether an Origin header value is trusted. Local
// development origins are always allowed; anything else must be listed in
// server.cors_origins (an explicit "*" entry keeps the legacy wildcard
// behavior for deployments that opt into it).
func (r *Router) originAllowed(origin string) bool {
	if isLocalhostOrigin(origin) {
		return true
	}
	for _, o := range r.cfg.Server.CORSOrigins {
		if o == "*" || o == origin {
			return true
		}
	}
	return false
}

// isLocalhostOrigin reports whether origin is a local development origin
// (Vite dev server, local preview, etc.) on any port.
func isLocalhostOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func (r *Router) handleActionPanel(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html><html><head><meta charset="utf-8"><title>数字人控制台</title>
<style>body{background:#1a1a2e;color:#fff;font-family:sans-serif;padding:20px;max-width:680px;margin:auto}
h2{color:#e94560;margin-top:20px}.row{margin:10px 0}
button{margin:5px;padding:10px 16px;font-size:14px;background:#16213e;border:1px solid #0f3460;color:#fff;border-radius:8px;cursor:pointer}
button:hover{background:#e94560}#status{margin-top:15px;padding:12px;background:#0f3460;border-radius:8px;min-height:20px}
select,input{padding:8px;margin:5px;background:#16213e;border:1px solid #0f3460;color:#fff;border-radius:6px}
.section{border:1px solid #0f3460;border-radius:10px;padding:15px;margin:15px 0}</style></head>
<body><h2>数字人控制台</h2>

<div class="section"><b>动作控制</b><div class="row">
<button onclick="doAct('idle')">待机</button>
<button onclick="doAct('wave')">挥手</button>
<button onclick="doAct('nod')">点头</button>
<button onclick="doAct('smile')">微笑</button>
<button onclick="doAct('laugh')">大笑</button>
<button onclick="doAct('think')">思考</button>
<button onclick="doAct('shy')">害羞</button>
<button onclick="doAct('angry')">生气</button>
<button onclick="doAct('drink')">喝水</button>
<button onclick="doAct('look')">看镜头</button>
</div></div>

<div class="section"><b>分辨率/帧率（离线生成）</b><div class="row">
<select id="res">
<option value="320*576">320×576（实时流畅）</option>
<option value="384*672">384×672（中等）</option>
<option value="480*832" selected>480×832（高清离线）</option>
</select>
帧率 <input id="fps" type="number" value="24" style="width:60px">
<button onclick="doRes()">应用</button>
</div></div>

<div class="section"><b>离线视频直播（WebRTC）</b><div class="row">
<button onclick="startOfflineLive()">开始离线直播</button>
<button onclick="stopOfflineLive()">停止</button>
</div></div>
<div id="status">操作状态</div>
<script>
function doAct(a){fetch('/action/set?a='+a).then(r=>r.json()).then(d=>{document.getElementById('status').innerText='动作: '+a;});}
function doRes(){var r=document.getElementById('res').value;var f=document.getElementById('fps').value;
document.getElementById('status').innerText='重启中... 约80秒后完成';
fetch('/action/res?r='+r+'&f='+f).then(r=>r.json()).then(d=>{
  // 轮询检查推理是否起来
  var cnt=0;
  var timer=setInterval(function(){
    cnt++;
    fetch('/action/ping').then(r=>r.json()).then(p=>{
      if(p.ok){clearInterval(timer);document.getElementById('status').innerText='✅ 重启完成！分辨率: '+d.size+' fps: '+d.fps+' 可以生成了';}
      else if(cnt>30){clearInterval(timer);document.getElementById('status').innerText='⚠️ 超时，刷新页面看看';}
      else{document.getElementById('status').innerText='重启中... '+cnt*3+'秒';}
    }).catch(()=>{});
  },3000);
});}
</script>
</body></html>`
	w.Write([]byte(html))
}

func (r *Router) handleActionSet(w http.ResponseWriter, req *http.Request) {
	actions := map[string]string{
		"idle":   "一个年轻女性自然地坐着说话，表情平静，偶尔眨眼",
		"wave":   "一个年轻女性在开心地挥手打招呼，面带微笑",
		"nod":    "一个年轻女性在点头表示赞同，表情认真",
		"smile":  "一个年轻女性在微笑，眼神温柔，表情愉悦",
		"laugh":  "一个年轻女性在开心大笑，身体微微前倾",
		"think":  "一个年轻女性在思考，手托下巴，表情疑惑",
		"shy":    "一个年轻女性害羞地低头微笑，手摸头发",
		"angry":  "一个年轻女性表情生气，双手叉腰",
		"drink":  "一个年轻女性拿起杯子喝水，动作自然",
		"look":   "一个年轻女性抬眼看镜头，眼神交流",
	}
	a := req.URL.Query().Get("a")
	prompt, ok := actions[a]
	if !ok { prompt = actions["idle"] }
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true,"action":"` + a + `"}`))
	go func(){ _ = os.WriteFile("/root/autodl-tmp/dynamic_prompt.txt", []byte(prompt), 0644) }()
}

func (r *Router) handleActionRes(w http.ResponseWriter, req *http.Request) {
	res := req.URL.Query().Get("r")
	fps := req.URL.Query().Get("f")
	// 改 config
	cfgPath := "/root/autodl-tmp/CyberVerse/config/cyberverse.yaml"
	data, _ := os.ReadFile(cfgPath)
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "size:") {
			lines[i] = "      size: " + res
		}
		if strings.HasPrefix(trimmed, "fps:") {
			lines[i] = "      fps: " + fps
		}
	}
	os.WriteFile(cfgPath, []byte(strings.Join(lines, "\n")), 0644)
	// 重启推理
	go func() {
		time.Sleep(1 * time.Second)
		exec.Command("pkill", "-f", "inference.sh").Run()
		time.Sleep(3 * time.Second)
		cmd := exec.Command("/bin/bash", "-c",
			"cd /root/autodl-tmp/CyberVerse && PYTHON=/root/autodl-tmp/envs/cyberverse/bin/python PATH=/root/autodl-tmp/envs/cyberverse/bin:$PATH nohup bash scripts/inference.sh > /root/autodl-tmp/inf.log 2>&1 &")
		cmd.Run()
	}()
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"size":"` + res + `","fps":"` + fps + `","msg":"重启中，约80秒后生效"}`))
}

func (r *Router) handleActionPing(w http.ResponseWriter, req *http.Request) {
	// 检查 50051 是否通
	conn, err := net.Dial("tcp", "127.0.0.1:50051")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":false}`))
		return
	}
	conn.Close()
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}



