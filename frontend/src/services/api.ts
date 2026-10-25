import type { AvatarModelInfo, BaiduXilingCharacterConfig, Character, CharacterForm, ComponentsResponse, ImageInfo, KnowledgeSource, KnowledgeUploadSkippedFile, OfflineVideoJob, Settings, LaunchConfig, LaunchConfigUpdate, PipelineMode, XunfeiAvatarConfig } from '../types'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

// ── Helpers ──

async function request<T>(path: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    ...opts,
  })
  if (!res.ok) {
    let message = `API error ${res.status}: ${path}`
    try {
      const data = await res.clone().json() as { error?: string }
      if (data?.error) message = data.error
    } catch {
      try {
        const text = await res.text()
        if (text) message = text
      } catch {
        // keep default message
      }
    }
    throw new Error(message)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

// ── Sessions (existing) ──

export type PlaybackMode = 'offline_live'

export interface CreateSessionResponse {
  session_id: string
  mode: PipelineMode
  streaming_mode: string  // "direct" or "livekit"
  playback_mode?: PlaybackMode | string
  avatar_enabled?: boolean
  idle_strategy?: 'cached_video' | 'silent_inference'
  audio_input?: {
    enabled: boolean
  }
  baidu_xiling?: {
    iframe_url: string
    origin: string
    figure_id: string
    camera_id?: string
    width: number
    height: number
    audio_sample_rate: number
    audio_max_pcm_bytes: number
  }
  xunfei?: {
    stream_url: string
    playback_url?: string
    avatar_id: string
    avatar_name?: string
    scene_id?: string
    vcn?: string
    protocol: 'xrtc' | 'rtmp' | 'webrtc' | 'flv'
    width: number
    height: number
    fps: number
    bitrate: number
    audio_sample_rate: number
    audio_max_pcm_bytes: number
  }
  vidu?: {
    live_id: string
    app_id: string
    channel_id: string
    user_id: string
    token: string
    token_expire_at: number
    live_duration: number
  }
  livekit_url?: string
  livekit_token?: string
  idle_video_url?: string
  idle_video_urls?: string[]
  offline_live_actions?: Array<{ id: string; title: string; url: string }>
  offline_live_songs?: Array<{ id: string; title: string; url: string }>
  danmaku_platform?: string
  bilibili_room_id?: number
  douyin_room?: string
  douyin_relay?: string
  idle_image_url?: string
  warnings?: string[]
  visual_input?: {
    enabled: boolean
    frame_interval_ms: number
    max_width: number
    max_height: number
    jpeg_quality: number
    max_frame_bytes: number
    ws_max_message_bytes: number
    max_recent_frames: number
    frame_ttl_ms: number
  }
}

export interface SessionInfo {
  id: string
  state: string
}

export interface HealthResponse {
  status: string
  sessions: number
  inference_connected: boolean
  error?: string
}

export interface CreateSessionPipelineOptions {
  llmProvider?: string
  llmModel?: string
  ttsProvider?: string
  ttsModel?: string
  ttsVoice?: string
  idleVideoUrl?: string
  offlineLiveActions?: Array<{ id: string; title: string; url: string }>
  offlineLiveSongs?: Array<{ id: string; title: string; url: string }>
  danmakuPlatform?: 'none' | 'bilibili' | 'douyin' | string
  bilibiliRoomId?: number
  douyinRoom?: string
  douyinRelay?: string
}

export async function createSession(
  characterId: string,
  mode: PipelineMode = 'standard',
  playbackMode?: PlaybackMode | string,
  pipeline?: CreateSessionPipelineOptions,
): Promise<CreateSessionResponse> {
  const body: Record<string, unknown> = { character_id: characterId, mode }
  if (playbackMode) {
    body.playback_mode = playbackMode
  }
  if (pipeline?.llmProvider) body.llm_provider = pipeline.llmProvider
  if (pipeline?.llmModel) body.llm_model = pipeline.llmModel
  if (pipeline?.ttsProvider) body.tts_provider = pipeline.ttsProvider
  if (pipeline?.ttsModel) body.tts_model = pipeline.ttsModel
  if (pipeline?.ttsVoice) body.tts_voice = pipeline.ttsVoice
  if (pipeline?.idleVideoUrl) body.idle_video_url = pipeline.idleVideoUrl
  if (pipeline?.offlineLiveActions?.length) body.offline_live_actions = pipeline.offlineLiveActions
  if (pipeline?.offlineLiveSongs?.length) body.offline_live_songs = pipeline.offlineLiveSongs
  if (pipeline?.danmakuPlatform && pipeline.danmakuPlatform !== 'none') {
    body.danmaku_platform = pipeline.danmakuPlatform
  }
  if (typeof pipeline?.bilibiliRoomId === 'number' && pipeline.bilibiliRoomId > 0) {
    body.bilibili_room_id = pipeline.bilibiliRoomId
  }
  if (pipeline?.douyinRoom) body.douyin_room = pipeline.douyinRoom
  if (pipeline?.douyinRelay) body.douyin_relay = pipeline.douyinRelay
  return request('/sessions', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export async function getComponents(): Promise<ComponentsResponse> {
  return request('/components')
}

export async function connectDanmaku(sessionId: string): Promise<{ status: string }> {
  return request(`/sessions/${encodeURIComponent(sessionId)}/danmaku/connect`, {
    method: 'POST',
    body: '{}',
  })
}

export async function reconnectDanmaku(sessionId: string): Promise<{ status: string }> {
  return connectDanmaku(sessionId)
}

export async function disconnectDanmaku(sessionId: string): Promise<{ status: string }> {
  return request(`/sessions/${encodeURIComponent(sessionId)}/danmaku/disconnect`, {
    method: 'POST',
    body: '{}',
  })
}

export interface DanmakuFeedSnapshotItem {
  username: string
  text: string
  source: string
  message_id?: string
  timestamp: number
}

export async function fetchDanmakuFeed(sessionId: string): Promise<{
  items: DanmakuFeedSnapshotItem[]
  status: string
  platform?: string
}> {
  return request(`/sessions/${encodeURIComponent(sessionId)}/danmaku/feed`)
}

export async function triggerOfflineLiveAction(
  sessionId: string,
  body: { action?: string; action_id?: string; clear?: boolean },
): Promise<{ status: string; action_id?: string; title?: string; url?: string; cleared?: boolean }> {
  return request(`/sessions/${encodeURIComponent(sessionId)}/offline-live/action`, {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export async function triggerOfflineLiveSong(
  sessionId: string,
  body: { title?: string; song_id?: string; stop?: boolean },
): Promise<{ status: string; song_id?: string; title?: string; url?: string; stopped?: boolean }> {
  return request(`/sessions/${encodeURIComponent(sessionId)}/offline-live/song`, {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export async function deleteSession(sessionId: string, keepalive = false): Promise<void> {
  const res = await fetch(`${API_BASE}/sessions/${sessionId}`, { method: 'DELETE', keepalive })
  if (!res.ok && res.status !== 404) throw new Error(`Failed to delete session: ${res.status}`)
}

export async function sendMessage(sessionId: string, text: string): Promise<void> {
  return request(`/sessions/${sessionId}/message`, {
    method: 'POST',
    body: JSON.stringify({ text }),
  })
}

export async function listSessions(): Promise<SessionInfo[]> {
  return request('/sessions')
}

export async function getHealth(): Promise<HealthResponse> {
  return request('/health')
}

// ── Agent Tasks ──

export interface AgentTask {
  id: string
  session_id: string
  character_id?: string
  kind: string
  title: string
  user_request: string
  status: 'queued' | 'running' | 'waiting_user' | 'completed' | 'failed' | 'cancelled'
  progress: number
  result_summary?: string
  created_at: string
  updated_at: string
  finished_at?: string
}

export interface AgentTaskEvent {
  task_id: string
  seq: number
  event_type: string
  status: AgentTask['status']
  message?: string
  progress: number
  payload?: Record<string, unknown>
  created_at: string
}

export async function listSessionTasks(sessionId: string): Promise<{ tasks: AgentTask[] }> {
  return request(`/sessions/${sessionId}/tasks`)
}

export async function getTaskEvents(taskId: string, afterSeq = 0): Promise<{ events: AgentTaskEvent[] }> {
  return request(`/tasks/${taskId}/events?after_seq=${afterSeq}`)
}

export function getTaskArtifactUrl(taskId: string, artifactId: string): string {
  return `${API_BASE}/tasks/${encodeURIComponent(taskId)}/artifacts/${encodeURIComponent(artifactId)}`
}

// ── Offline Videos ──

export interface CreateOfflineVideoInput {
  inputType: 'text' | 'audio' | 'sing'
  text?: string
  audio?: File | null
  inputAudioUrl?: string
  ttsProvider?: string
  ttsModel?: string
  ttsVoice?: string
  width?: number
  height?: number
  transparent?: boolean
  ttsPerson?: string
  ttsLan?: string
  ttsSpeed?: number
  ttsVolume?: number
  ttsPitch?: number
  backgroundImageUrl?: string
  autoAnimoji?: boolean
  actionPrompt?: string
  title?: string
  songName?: string
}

export async function listOfflineVideos(characterId: string): Promise<{ videos: OfflineVideoJob[] }> {
  return request(`/characters/${encodeURIComponent(characterId)}/offline-videos`)
}

export async function createOfflineVideo(characterId: string, input: CreateOfflineVideoInput): Promise<OfflineVideoJob> {
  const form = new FormData()
  form.append('input_type', input.inputType)
  if (input.text) form.append('text', input.text)
  if (input.audio) form.append('audio', input.audio)
  if (input.inputAudioUrl) form.append('input_audio_url', input.inputAudioUrl)
  if (input.ttsProvider) form.append('tts_provider', input.ttsProvider)
  if (input.ttsModel) form.append('tts_model', input.ttsModel)
  if (input.ttsVoice) form.append('tts_voice', input.ttsVoice)
  if (input.width) form.append('width', String(input.width))
  if (input.height) form.append('height', String(input.height))
  if (typeof input.transparent === 'boolean') form.append('transparent', String(input.transparent))
  if (input.ttsPerson) form.append('tts_person', input.ttsPerson)
  if (input.ttsLan) form.append('tts_lan', input.ttsLan)
  if (typeof input.ttsSpeed === 'number') form.append('tts_speed', String(input.ttsSpeed))
  if (typeof input.ttsVolume === 'number') form.append('tts_volume', String(input.ttsVolume))
  if (typeof input.ttsPitch === 'number') form.append('tts_pitch', String(input.ttsPitch))
  if (input.backgroundImageUrl) form.append('background_image_url', input.backgroundImageUrl)
  if (typeof input.autoAnimoji === 'boolean') form.append('auto_animoji', String(input.autoAnimoji))
  if (input.actionPrompt) form.append('actionPrompt', input.actionPrompt)
  if (input.title) form.append('title', input.title)
  if (input.songName) form.append('song_name', input.songName)
  const res = await fetch(`${API_BASE}/characters/${encodeURIComponent(characterId)}/offline-videos`, {
    method: 'POST',
    body: form,
    credentials: 'same-origin',
  })
  if (!res.ok) {
    let message = `API error ${res.status}: offline video`
    try {
      const data = await res.json() as { error?: string }
      if (data?.error) message = data.error
    } catch {
      // keep default message
    }
    throw new Error(message)
  }
  return res.json()
}

export async function renameOfflineVideo(characterId: string, jobId: string, title: string): Promise<OfflineVideoJob> {
  return request(`/characters/${encodeURIComponent(characterId)}/offline-videos/${encodeURIComponent(jobId)}`, {
    method: 'PATCH',
    body: JSON.stringify({ title }),
  })
}

export async function deleteOfflineVideo(characterId: string, jobId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/characters/${encodeURIComponent(characterId)}/offline-videos/${encodeURIComponent(jobId)}`, {
    method: 'DELETE',
    credentials: 'same-origin',
  })
  if (!res.ok && res.status !== 404) throw new Error(`Failed to delete offline video: ${res.status}`)
}

// ── Conversation History ──

export interface ConversationMessagesResponse {
  messages: { role: string; content: string; timestamp: string; session_id: string }[]
  next_cursor: string
  has_more: boolean
}

export async function getConversationMessages(
  characterId: string,
  limit: number = 50,
  before?: string,
): Promise<ConversationMessagesResponse> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (before) params.set('before', before)
  return request(`/characters/${characterId}/conversations/messages?${params}`)
}

// ── Characters ──

export async function getCharacters(): Promise<Character[]> {
  return request('/characters')
}

export async function getCharacter(id: string): Promise<Character> {
  return request(`/characters/${id}`)
}

export async function createCharacter(data: CharacterForm): Promise<Character> {
  return request('/characters', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export async function updateCharacter(id: string, data: CharacterForm): Promise<Character> {
  return request(`/characters/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export async function updateOfflineVideoTTS(
  id: string,
  data: { provider: string; model?: string; voice: string },
): Promise<Character> {
  return request(`/characters/${id}/offline-video-tts`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export async function deleteCharacter(id: string): Promise<void> {
  return request(`/characters/${id}`, { method: 'DELETE' })
}

export async function testCharacterVoice(data: { voice_provider: string; voice_type: string; model?: string }): Promise<{ status: string }> {
  return request('/characters/test-voice', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export async function getBaiduXilingFigure(figureId: string): Promise<BaiduXilingCharacterConfig> {
  return request(`/baidu-xiling/figures/${encodeURIComponent(figureId)}`)
}

export async function getXunfeiAvatar(avatarId: string): Promise<XunfeiAvatarConfig> {
  return request(`/xunfei/avatars/${encodeURIComponent(avatarId)}`)
}

export async function uploadAvatar(id: string, file: File): Promise<{ path: string; filename?: string }> {
  const formData = new FormData()
  formData.append('avatar', file)
  const res = await fetch(`${API_BASE}/characters/${id}/avatar`, {
    method: 'POST',
    body: formData,
  })
  if (!res.ok) throw new Error(`Failed to upload avatar: ${res.status}`)
  return res.json()
}

// ── Character Images ──

export async function getCharacterImages(id: string): Promise<ImageInfo[]> {
  return request(`/characters/${id}/images`)
}

export async function deleteCharacterImage(id: string, filename: string): Promise<void> {
  const res = await fetch(`${API_BASE}/characters/${id}/images/${filename}`, { method: 'DELETE' })
  if (!res.ok && res.status !== 404) throw new Error(`Failed to delete image: ${res.status}`)
}

export async function activateCharacterImage(id: string, filename: string): Promise<void> {
  const res = await fetch(`${API_BASE}/characters/${id}/images/${filename}/activate`, { method: 'PUT' })
  if (!res.ok) throw new Error(`Failed to activate image: ${res.status}`)
}

// ── Character Knowledge Sources ──

export async function getKnowledgeSources(id: string): Promise<KnowledgeSource[]> {
  const data = await request<{ sources: KnowledgeSource[] }>(`/characters/${id}/knowledge`)
  return data.sources
}

export interface UploadKnowledgeFilesResult {
  sources: KnowledgeSource[]
  skipped?: KnowledgeUploadSkippedFile[]
}

export async function uploadKnowledgeFiles(
  id: string,
  files: File[],
): Promise<UploadKnowledgeFilesResult> {
  const formData = new FormData()
  for (const file of files) {
    const uploadFile = file as File & { webkitRelativePath?: string; relativePath?: string }
    const relativePath = uploadFile.relativePath || uploadFile.webkitRelativePath || file.name
    formData.append('files', file, relativePath)
    formData.append('relative_paths', relativePath)
  }
  const res = await fetch(`${API_BASE}/characters/${id}/knowledge/files`, {
    method: 'POST',
    body: formData,
  })
  if (!res.ok) {
    let message = `Failed to upload knowledge source: ${res.status}`
    try {
      const body = await res.json() as { error?: string; skipped?: KnowledgeUploadSkippedFile[] }
      if (body.error) message = body.error
      if (body.skipped?.length) {
        message += ` (${body.skipped.length} skipped)`
      }
    } catch {
      // keep default message
    }
    throw new Error(message)
  }
  const body = await res.json()
  if (Array.isArray(body?.sources)) return body
  return { sources: [body as KnowledgeSource] }
}

export async function deleteKnowledgeSource(id: string, sourceId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/characters/${id}/knowledge/${sourceId}`, { method: 'DELETE' })
  if (!res.ok && res.status !== 404) throw new Error(`Failed to delete knowledge source: ${res.status}`)
}

export async function reindexKnowledgeSource(id: string, sourceId: string): Promise<KnowledgeSource> {
  return request(`/characters/${id}/knowledge/${sourceId}/reindex`, { method: 'POST' })
}

// ── Settings ──

export async function getSettings(): Promise<Settings> {
  return request('/settings')
}

export async function updateSettings(data: Settings): Promise<void> {
  return request('/settings', {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export async function testConnection(): Promise<{ status: string }> {
  return request('/settings/test', { method: 'POST' })
}

// ── Launch Config ──

export async function getAvatarModelInfo(): Promise<AvatarModelInfo> {
  return request('/config/avatar-model')
}

export async function getLaunchConfig(model?: string): Promise<LaunchConfig> {
  const qs = model ? `?model=${encodeURIComponent(model)}` : ''
  return request(`/config/launch${qs}`)
}

export async function updateLaunchConfig(data: LaunchConfigUpdate): Promise<{ status: string; requires_restart: boolean }> {
  return request('/config/launch', {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}
