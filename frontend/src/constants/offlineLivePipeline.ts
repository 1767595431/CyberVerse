/** Defaults for offline-live standard pipeline (LLM → TTS → offline answer video). */
export const OFFLINE_LIVE_DEFAULT_LLM_PROVIDER = 'qwen'
export const OFFLINE_LIVE_DEFAULT_LLM_MODEL = 'qwen3.6-plus'
export const OFFLINE_LIVE_DEFAULT_TTS_PROVIDER = 'qwen'
export const OFFLINE_LIVE_DEFAULT_TTS_MODEL = 'qwen3-tts-flash-realtime'
export const OFFLINE_LIVE_DEFAULT_TTS_VOICE = 'Serena'

export const OFFLINE_LIVE_OMNI_TTS_MODEL_OPTIONS = [
  { label: 'qwen3.5-omni-flash-realtime', value: 'qwen3.5-omni-flash-realtime' },
  { label: 'qwen3-tts-flash-realtime', value: 'qwen3-tts-flash-realtime' },
]
