<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useCharacterStore } from '../stores/characters'
import { createSession, getAvatarModelInfo, getComponents, getLaunchConfig, listOfflineVideos, testCharacterVoice, updateLaunchConfig } from '../services/api'
import {
  OFFLINE_LIVE_DEFAULT_LLM_MODEL,
  OFFLINE_LIVE_DEFAULT_LLM_PROVIDER,
  OFFLINE_LIVE_DEFAULT_TTS_MODEL,
  OFFLINE_LIVE_DEFAULT_TTS_VOICE,
  OFFLINE_LIVE_OMNI_TTS_MODEL_OPTIONS,
} from '../constants/offlineLivePipeline'
import { QWEN_OMNI_VOICE_OPTIONS } from '../types'
import type { ComponentsResponse } from '../types'
import { localizedVoiceOptions } from '../utils/voice'
import AppHeader from '../components/AppHeader.vue'
import CvSelect from '../components/CvSelect.vue'
import type { AvatarModelInfo, ConfigSection, ConfigParam } from '../types'
import { saveLaunchWorkspaceMode, type LaunchWorkspaceMode } from '../utils/launchModePreference'
import { formatVoiceTypeDisplay } from '../utils/voice'
import { mapSelectedUrlsToClips } from '../utils/offlineLiveClipOrder'
import { buildSessionLaunchState, saveSessionLaunchState } from '../utils/sessionLaunchState'
import {
  isOfflineLiveActionVideoTitle,
  isOfflineLiveIdleVideoTitle,
  isOfflineLiveSingVideoTitle,
} from '../utils/offlineLiveVideoTitle'
import {
  buildOfflineLiveLaunchPreferenceFromUrls,
  loadOfflineLiveLaunchPreference,
  offlineLiveLaunchPreferenceKey,
  resolveOfflineLiveLaunchVideoUrls,
  saveOfflineLiveLaunchPreference,
} from '../utils/offlineLiveLaunchPreference'
import { LOCAL_ONLY_DEPLOYMENT } from '../constants/localDeployment'
import { isCloudAvatarBackend } from '../utils/localDeployment'

type ManagedVideoOption = {
  id: string
  url: string
  label: string
}

const router = useRouter()
const route = useRoute()
const store = useCharacterStore()
const { t, locale } = useI18n()
const characterId = computed(() => route.params.id as string)
const workspaceMode = computed<LaunchWorkspaceMode>(() =>
  route.name === 'launch-offline-live' ? 'offline_live' : 'live',
)
const isOfflineLiveWorkspace = computed(() => workspaceMode.value === 'offline_live')
const pageTitle = computed(() =>
  isOfflineLiveWorkspace.value ? t('offlineVideo.offlineLiveTitle') : t('launch.workspaceTitle'),
)
const hasCurrentCharacter = computed(() => store.current?.id === characterId.value)
const showLoading = computed(() => loading.value && !hasCurrentCharacter.value)
const connecting = ref(false)

// Config state
const configSections = ref<ConfigSection[]>([])
const originalSections = ref<ConfigSection[]>([])
const loading = ref(true)
const saving = ref(false)
const saveMessage = ref('')
const errorMessage = ref('')
const avatarModelInfo = ref<AvatarModelInfo | null>(null)

const activeAvatarModel = computed(() => avatarModelInfo.value?.active_model || '')
const configuredDefaultModel = computed(() => avatarModelInfo.value?.configured_default_model || '')
const runtimeConfigMismatch = computed(() =>
  !!activeAvatarModel.value &&
  !!configuredDefaultModel.value &&
  activeAvatarModel.value !== configuredDefaultModel.value
)
const isBaiduXilingCharacter = computed(() => store.current?.avatar_backend === 'baidu_xiling')
const isXunfeiCharacter = computed(() => store.current?.avatar_backend === 'xunfei')
const isViduCharacter = computed(() => store.current?.avatar_backend === 'vidu')
const isExternalAvatarCharacter = computed(() => isBaiduXilingCharacter.value || isXunfeiCharacter.value || isViduCharacter.value)
const cloudAvatarBlocked = computed(() =>
  LOCAL_ONLY_DEPLOYMENT && isCloudAvatarBackend(store.current?.avatar_backend),
)
const managedVideoOptions = ref<ManagedVideoOption[]>([])
const selectedIdleVideoUrls = ref<string[]>([])
const selectedActionVideoUrls = ref<string[]>([])
const selectedSingVideoUrls = ref<string[]>([])
const danmakuPlatform = ref<'none' | 'bilibili' | 'douyin'>('none')
const bilibiliRoomId = ref('')
const douyinRoom = ref('')
const douyinRelay = ref('http://127.0.0.1:8757')
const videoManagerPath = computed(() => `/characters/${characterId.value}/offline-videos`)
const previewingManagedVideo = ref<ManagedVideoOption | null>(null)

const idleVideoOptions = computed(() =>
  managedVideoOptions.value.filter(item => isOfflineLiveIdleVideoTitle(item.label)),
)
const actionVideoOptions = computed(() =>
  managedVideoOptions.value.filter(item => isOfflineLiveActionVideoTitle(item.label)),
)
const singVideoOptions = computed(() =>
  managedVideoOptions.value.filter(item => isOfflineLiveSingVideoTitle(item.label)),
)

function openManagedVideoPreview(option: ManagedVideoOption) {
  previewingManagedVideo.value = option
}

function closeManagedVideoPreview() {
  previewingManagedVideo.value = null
}

const offlineLiveComponentCatalog = ref<ComponentsResponse>({ llm: [], asr: [], tts: [] })
const offlineLiveLLMProvider = ref(OFFLINE_LIVE_DEFAULT_LLM_PROVIDER)
const offlineLiveLLMModel = ref(OFFLINE_LIVE_DEFAULT_LLM_MODEL)
const offlineLiveTTSModel = ref(OFFLINE_LIVE_DEFAULT_TTS_MODEL)
const offlineLiveTTSVoice = ref(OFFLINE_LIVE_DEFAULT_TTS_VOICE)

const offlineLiveLLMProviderOptions = computed(() =>
  offlineLiveComponentCatalog.value.llm.map(item => ({ label: item.name, value: item.id })),
)
const offlineLiveLLMModelOptions = computed(() => {
  const selected = offlineLiveComponentCatalog.value.llm.find(item => item.id === offlineLiveLLMProvider.value)
  const model = selected?.model || OFFLINE_LIVE_DEFAULT_LLM_MODEL
  return [{ label: model, value: model }]
})
const offlineLiveTTSModelOptions = computed(() => OFFLINE_LIVE_OMNI_TTS_MODEL_OPTIONS)
const offlineLiveTTSVoiceOptions = computed(() =>
  localizedVoiceOptions(QWEN_OMNI_VOICE_OPTIONS, locale.value),
)

watch(offlineLiveLLMProvider, (provider) => {
  const selected = offlineLiveComponentCatalog.value.llm.find(item => item.id === provider)
  if (selected?.model) {
    offlineLiveLLMModel.value = selected.model
  }
})

const QWEN_OMNI_VOICE_LIST_URL = 'https://help.aliyun.com/zh/model-studio/omni-voice-list'
const testingOfflineLiveVoice = ref(false)
const offlineLiveVoiceCheckSucceeded = ref(false)
const offlineLiveVoiceTestMessage = ref('')

watch([offlineLiveTTSModel, offlineLiveTTSVoice], () => {
  offlineLiveVoiceCheckSucceeded.value = false
  offlineLiveVoiceTestMessage.value = ''
})

async function handleOfflineLiveCheckVoice() {
  const voiceType = offlineLiveTTSVoice.value.trim()
  if (!voiceType) return

  offlineLiveVoiceCheckSucceeded.value = false
  offlineLiveVoiceTestMessage.value = ''
  testingOfflineLiveVoice.value = true
  try {
    await testCharacterVoice({
      voice_provider: 'qwen_omni',
      voice_type: voiceType,
      model: offlineLiveTTSModel.value,
    })
    offlineLiveVoiceCheckSucceeded.value = true
  } catch (e) {
    offlineLiveVoiceTestMessage.value = e instanceof Error ? e.message : String(e)
  } finally {
    testingOfflineLiveVoice.value = false
  }
}

const selectedIdleClips = computed(() =>
  mapSelectedUrlsToClips(selectedIdleVideoUrls.value, idleVideoOptions.value),
)
const selectedActionClips = computed(() =>
  mapSelectedUrlsToClips(selectedActionVideoUrls.value, actionVideoOptions.value),
)
const selectedSingClips = computed(() =>
  mapSelectedUrlsToClips(selectedSingVideoUrls.value, singVideoOptions.value),
)
const canLaunch = computed(() => {
  if (cloudAvatarBlocked.value) return false
  if (isOfflineLiveWorkspace.value) {
    if (
      selectedIdleVideoUrls.value.length < 1
      || (selectedActionClips.value.length === 0 && selectedSingClips.value.length === 0)
      || isExternalAvatarCharacter.value
    ) {
      return false
    }
    if (danmakuPlatform.value === 'bilibili') {
      const room = Number.parseInt(bilibiliRoomId.value.trim(), 10)
      return Number.isFinite(room) && room > 0
    }
    if (danmakuPlatform.value === 'douyin') {
      return douyinRoom.value.trim().length > 0
    }
    return true
  }
  if (isBaiduXilingCharacter.value) {
    return !!store.current?.baidu_xiling?.figure_id
  }
  if (isXunfeiCharacter.value) {
    return !!store.current?.xunfei?.avatar_id
  }
  if (isViduCharacter.value) {
    return !!store.current?.active_image || !!store.current?.avatar_image
  }
  return !!activeAvatarModel.value
})
const characterCoverImage = computed(() => {
  const character = store.current
  if (!character) return ''
  if (character.avatar_backend === 'baidu_xiling') {
    return character.baidu_xiling?.thumbnail_url || character.baidu_xiling?.source_image_url || ''
  }
  if (character.avatar_backend === 'xunfei') {
    return character.xunfei?.thumbnail_url || character.xunfei?.source_image_url || ''
  }
  return character.avatar_image || ''
})
const baiduXilingConfig = computed(() => store.current?.baidu_xiling || null)
const baiduXilingResolution = computed(() => {
  const width = baiduXilingConfig.value?.width || 0
  const height = baiduXilingConfig.value?.height || 0
  if (width <= 0 || height <= 0) return t('common.emptyDash')
  return `${width} × ${height}`
})
const baiduXilingInfoRows = computed(() => {
  return [
    { label: t('launch.baiduResolution'), value: baiduXilingResolution.value },
  ]
})
const xunfeiInfoRows = computed(() => {
  const config = store.current?.xunfei
  return [
    { label: t('launch.xunfeiAvatarId'), value: config?.avatar_id || t('common.emptyDash') },
    { label: t('launch.xunfeiSceneId'), value: config?.scene_id || t('common.emptyDash') },
    { label: t('launch.xunfeiVcn'), value: config?.vcn || t('common.emptyDash') },
  ]
})

// Input width auto-sizing (in ch units)
const INPUT_MIN_WIDTH_CH = 16
const INPUT_MAX_WIDTH_CH = 48
const INPUT_PADDING_CH = 0

function inputWidth(value: string | number): string {
  const len = String(value).length + INPUT_PADDING_CH
  return Math.min(Math.max(len, INPUT_MIN_WIDTH_CH), INPUT_MAX_WIDTH_CH) + 'ch'
}

// Deep clone helper
function cloneSections(sections: ConfigSection[]): ConfigSection[] {
  return JSON.parse(JSON.stringify(sections))
}

function comparableSections(sections: ConfigSection[]): ConfigSection[] {
  return sections.map((section) => {
    const comparable = { ...section }
    delete comparable.collapsed
    return comparable
  })
}

// Check if there are unsaved changes
const hasChanges = computed(() => {
  if (originalSections.value.length === 0) return false
  return JSON.stringify(comparableSections(configSections.value)) !== JSON.stringify(comparableSections(originalSections.value))
})

function sectionHasRestartPending(section: ConfigSection): boolean {
  const orig = originalSections.value.find(s => s.title === section.title)
  if (!orig) return false
  for (const param of section.params) {
    if (!param.requires_restart) continue
    const origParam = orig.params.find((p: ConfigParam) => p.path === param.path)
    if (origParam && origParam.value !== param.value) return true
  }
  return false
}

const restartBadgeHint = computed(() => t('launch.restartHint'))

function launchSectionTitle(section: ConfigSection): string {
  const key = section.key || ''
  return key ? t(`launch.sections.${key}`) : section.title
}

watch(workspaceMode, (mode) => {
  saveLaunchWorkspaceMode(mode)
}, { immediate: true })

async function loadOfflineLiveComponentCatalog() {
  try {
    offlineLiveComponentCatalog.value = await getComponents()
    const qwen = offlineLiveComponentCatalog.value.llm.find(item => item.id === 'qwen')
    if (qwen?.model) {
      offlineLiveLLMProvider.value = 'qwen'
      offlineLiveLLMModel.value = qwen.model
    }
  } catch (e) {
    console.warn('Failed to load component catalog for offline live:', e)
  }
}

async function loadOfflineLiveManagedVideos() {
  await loadOfflineLiveComponentCatalog()
  const options: ManagedVideoOption[] = []

  try {
    const { videos } = await listOfflineVideos(characterId.value)
    videos
      .filter(job => job.status === 'completed' && !!job.video_url)
      .forEach((job) => {
        options.push({
          id: job.id,
          url: job.video_url!,
          label: job.title || `${t('offlineVideo.offlineLiveVideoFromManager')} ${job.id.slice(0, 8)}`,
        })
      })
  } catch (e) {
    console.warn('Failed to load offline videos for offline live picker:', e)
  }

  managedVideoOptions.value = options

  const labelById = new Map(options.map(item => [item.id, item.label]))
  const saved = loadOfflineLiveLaunchPreference(characterId.value)
  const resolved = resolveOfflineLiveLaunchVideoUrls(
    options,
    saved,
    isOfflineLiveIdleVideoTitle,
    isOfflineLiveActionVideoTitle,
    isOfflineLiveSingVideoTitle,
    labelById,
  )
  selectedIdleVideoUrls.value = resolved.idleUrls
  selectedActionVideoUrls.value = resolved.actionUrls
  selectedSingVideoUrls.value = resolved.singUrls

  const hadStoredPreference = typeof window !== 'undefined'
    && window.localStorage.getItem(offlineLiveLaunchPreferenceKey(characterId.value)) != null
  if (!hadStoredPreference && selectedIdleVideoUrls.value.length === 0) {
    const firstIdle = options.find(item => isOfflineLiveIdleVideoTitle(item.label))
    if (firstIdle) {
      selectedIdleVideoUrls.value = [firstIdle.url]
    }
  }

  saveOfflineLiveLaunchPreference(
    characterId.value,
    buildOfflineLiveLaunchPreferenceFromUrls(
      options,
      selectedIdleVideoUrls.value,
      selectedActionVideoUrls.value,
      selectedSingVideoUrls.value,
    ),
  )
}

function persistOfflineLiveVideoPreference() {
  if (!isOfflineLiveWorkspace.value || !characterId.value) return
  const pref = buildOfflineLiveLaunchPreferenceFromUrls(
    managedVideoOptions.value,
    selectedIdleVideoUrls.value,
    selectedActionVideoUrls.value,
    selectedSingVideoUrls.value,
  )
  saveOfflineLiveLaunchPreference(characterId.value, pref)
}

function toggleManagedVideoSelection(url: string, selected: typeof selectedIdleVideoUrls, minSelected = 1) {
  const exists = selected.value.includes(url)
  if (exists) {
    if (selected.value.length <= minSelected) return
    selected.value = selected.value.filter(item => item !== url)
    return
  }
  selected.value = [...selected.value, url]
}

function toggleIdleVideo(url: string) {
  toggleManagedVideoSelection(url, selectedIdleVideoUrls, 1)
  persistOfflineLiveVideoPreference()
}

function toggleActionVideo(url: string) {
  toggleManagedVideoSelection(url, selectedActionVideoUrls, 0)
  persistOfflineLiveVideoPreference()
}

function toggleSingVideo(url: string) {
  toggleManagedVideoSelection(url, selectedSingVideoUrls, 0)
  persistOfflineLiveVideoPreference()
}

onMounted(async () => {
  await store.fetchOne(characterId.value).catch(() => {})

  if (isOfflineLiveWorkspace.value) {
    await loadOfflineLiveManagedVideos()
  } else {
    await loadLiveLaunchConfig()
  }
  loading.value = false
})

async function loadLiveLaunchConfig() {
  if (isExternalAvatarCharacter.value) {
    configSections.value = []
    originalSections.value = []
    return
  }
  try {
    avatarModelInfo.value = await getAvatarModelInfo()
    const config = await getLaunchConfig()
    configSections.value = config.sections.map(s => ({ ...s, collapsed: false }))
    originalSections.value = cloneSections(configSections.value)
  } catch (e) {
    errorMessage.value = e instanceof Error ? e.message : t('launch.loadConfigFailed')
    console.error('Failed to load launch config:', e)
  }
}

watch(isOfflineLiveWorkspace, async (offlineLive) => {
  if (!hasCurrentCharacter.value) return
  loading.value = true
  errorMessage.value = ''
  if (offlineLive) {
    await loadOfflineLiveManagedVideos()
  } else if (configSections.value.length === 0) {
    await loadLiveLaunchConfig()
  }
  loading.value = false
})

async function saveConfig() {
  saving.value = true
  saveMessage.value = ''
  errorMessage.value = ''

  // Collect changed non-readonly params
  const changedParams: Array<{ path: string; value: string | number }> = []
  for (const section of configSections.value) {
    for (const param of section.params) {
      if (param.readonly) continue
      // Find original value
      const origSection = originalSections.value.find(s => s.title === section.title)
      const origParam = origSection?.params.find(p => p.path === param.path)
      if (origParam && origParam.value !== param.value) {
        changedParams.push({ path: param.path, value: param.value })
      }
    }
  }

  if (changedParams.length === 0) {
    saving.value = false
    return
  }

  try {
    const resp = await updateLaunchConfig({
      model: activeAvatarModel.value,
      params: changedParams,
    })
    originalSections.value = cloneSections(configSections.value)
    if (resp.requires_restart) {
      saveMessage.value = t('launch.savedRequiresRestart')
    } else {
      saveMessage.value = t('launch.saved')
    }
  } catch (e) {
    errorMessage.value = t('launch.saveFailed')
    console.error('Failed to save launch config:', e)
  } finally {
    saving.value = false
  }
}

async function launch() {
  if (!canLaunch.value) return
  connecting.value = true
  try {
    if (isOfflineLiveWorkspace.value) {
      persistOfflineLiveVideoPreference()
    }
    const playbackMode = isOfflineLiveWorkspace.value ? 'offline_live' : undefined
    // Offline live: text/API → LLM → TTS → offline answer clip (no ASR / omni mic).
    const launchMode = isOfflineLiveWorkspace.value
      ? 'standard'
      : (store.current?.mode || 'standard')
    const resp = await createSession(
      characterId.value,
      launchMode,
      playbackMode,
      isOfflineLiveWorkspace.value
        ? {
            llmProvider: offlineLiveLLMProvider.value,
            llmModel: offlineLiveLLMModel.value,
            ttsProvider: 'qwen',
            ttsModel: offlineLiveTTSModel.value,
            ttsVoice: offlineLiveTTSVoice.value,
            idleVideoUrl: selectedIdleVideoUrls.value[0],
            idleVideoUrls: selectedIdleVideoUrls.value,
            offlineLiveActions: selectedActionClips.value,
            offlineLiveSongs: selectedSingClips.value,
            danmakuPlatform: danmakuPlatform.value,
            bilibiliRoomId: danmakuPlatform.value === 'bilibili'
              ? Number.parseInt(bilibiliRoomId.value.trim(), 10)
              : undefined,
            douyinRoom: danmakuPlatform.value === 'douyin' ? douyinRoom.value.trim() : '',
            douyinRelay: danmakuPlatform.value === 'douyin' ? douyinRelay.value.trim() : '',
          }
        : undefined,
    )
    resp.warnings?.forEach((warning) => {
      console.warn('[CyberVerse]', warning)
    })
    const launchReturnPath = isOfflineLiveWorkspace.value
      ? `/launch/${characterId.value}/offline-live`
      : `/launch/${characterId.value}`
    const launchState = buildSessionLaunchState(
      resp,
      characterId.value,
      launchMode,
      launchReturnPath,
      playbackMode,
    )
    if (playbackMode === 'offline_live' && selectedIdleVideoUrls.value.length >= 1) {
      launchState.idle_strategy = 'cached_video'
      launchState.idle_video_url = selectedIdleVideoUrls.value[0]
      launchState.idle_video_urls = [...selectedIdleVideoUrls.value]
      launchState.offline_live_idle_clips = selectedIdleClips.value
      launchState.offline_live_actions = resp.offline_live_actions?.length
        ? resp.offline_live_actions
        : selectedActionClips.value
      launchState.offline_live_songs = resp.offline_live_songs?.length
        ? resp.offline_live_songs
        : selectedSingClips.value
      if (resp.danmaku_platform) {
        launchState.danmaku_platform = resp.danmaku_platform
        launchState.bilibili_room_id = resp.bilibili_room_id
        launchState.douyin_room = resp.douyin_room
        launchState.douyin_relay = resp.douyin_relay
      } else if (danmakuPlatform.value !== 'none') {
        launchState.danmaku_platform = danmakuPlatform.value
        launchState.bilibili_room_id = danmakuPlatform.value === 'bilibili'
          ? Number.parseInt(bilibiliRoomId.value.trim(), 10)
          : undefined
        launchState.douyin_room = danmakuPlatform.value === 'douyin' ? douyinRoom.value.trim() : undefined
        launchState.douyin_relay = danmakuPlatform.value === 'douyin' ? douyinRelay.value.trim() : undefined
      }
    }
    saveSessionLaunchState(launchState)
    router.push(`/session/${resp.session_id}`)
  } catch (e) {
    errorMessage.value = e instanceof Error ? e.message : t('launch.launchFailed')
    console.error('Failed to launch:', e)
  } finally {
    connecting.value = false
  }
}

</script>

<template>
  <div class="launch-workspace flex min-h-screen flex-col bg-cv-base text-cv-text">
    <AppHeader showBack :title="pageTitle" />

    <div class="py-6 text-center">
      <div class="cv-pi-segment mx-auto h-11 w-[390px] max-w-[calc(100vw-48px)] grid-cols-3">
        <button
          class="cv-pi-segment-item"
          type="button"
          :disabled="isViduCharacter"
          :title="isViduCharacter ? t('offlineVideo.viduUnsupported') : ''"
          @click="router.push(`/launch/${characterId}/offline`)"
        >
          {{ t('offlineVideo.offlineMode') }}
        </button>
        <button
          class="cv-pi-segment-item"
          type="button"
          :class="{ 'cv-pi-segment-item--active': !isOfflineLiveWorkspace }"
          @click="router.push(`/launch/${characterId}/live`)"
        >
          {{ t('offlineVideo.liveMode') }}
        </button>
        <button
          class="cv-pi-segment-item"
          type="button"
          :class="{ 'cv-pi-segment-item--active': isOfflineLiveWorkspace }"
          :disabled="isViduCharacter || isExternalAvatarCharacter"
          :title="isViduCharacter ? t('offlineVideo.viduUnsupported') : ''"
          @click="router.push(`/launch/${characterId}/offline-live`)"
        >
          {{ t('offlineVideo.offlineLiveMode') }}
        </button>
      </div>
      <p v-if="isOfflineLiveWorkspace" class="mt-3 text-xs text-cv-text-muted">
        {{ t('offlineVideo.offlineLiveHint') }}
      </p>
    </div>

    <main class="mx-auto flex w-full max-w-[1100px] flex-1 flex-col gap-8 px-12 pb-24">
      <div v-if="showLoading" class="py-24 text-center text-cv-text-secondary">{{ t('launch.loadingConfig') }}</div>

      <template v-else-if="store.current">
        <p
          v-if="cloudAvatarBlocked"
          class="rounded-cv-lg border border-cv-warning/30 bg-cv-warning/10 px-4 py-3 text-sm leading-6 text-cv-warning"
        >
          {{ t('localDeployment.cloudAvatarBlocked') }}
        </p>
        <div class="grid gap-8 lg:grid-cols-[300px_minmax(0,1fr)]">
          <aside class="character-panel">
            <div class="avatar-shell">
              <img
                v-if="characterCoverImage"
                :src="characterCoverImage"
                :alt="store.current.name"
                class="h-full w-full object-cover"
              >
              <div v-else class="flex h-full items-center justify-center text-sm text-cv-text-muted">
                {{ t('offlineVideo.noAvatar') }}
              </div>
            </div>
            <div>
              <h2 class="truncate text-xl font-bold text-[#fbf6ef]">{{ store.current.name }}</h2>
              <p class="mt-2 text-sm leading-6 text-[#8d96a6]">
                {{ store.current.description || t('characterCard.noDescription') }}
              </p>
            </div>
            <div class="info-grid">
              <span>{{ t('launch.voice') }}</span>
              <strong>{{ formatVoiceTypeDisplay(store.current.voice_type, t, locale) }}</strong>
              <span>{{ t('offlineVideo.avatarSource') }}</span>
              <strong>{{ store.current.avatar_backend }}</strong>
            </div>
            <button
              class="cv-pi-button cv-pi-button--compact"
              type="button"
              @click="router.push(`/characters/${characterId}/edit`)"
            >
              {{ t('launch.editCharacter') }}
            </button>
          </aside>

          <section class="production-panel">
            <div v-if="errorMessage" class="notice error">{{ errorMessage }}</div>

            <template v-if="isOfflineLiveWorkspace">
              <div class="idle-picker-card offline-live-pipeline-card">
                <div class="idle-picker-header">
                  <h3>{{ t('offlineVideo.offlineLivePipelineTitle') }}</h3>
                  <p>{{ t('offlineVideo.offlineLivePipelineHint') }}</p>
                </div>

                <div class="flex flex-col gap-4">
                  <div class="grid gap-3 md:grid-cols-[90px_minmax(0,1fr)_minmax(0,1fr)] md:items-end">
                    <span class="text-[13px] font-medium text-cv-text-secondary md:pb-3">LLM</span>
                    <label class="block">
                      <span class="text-[12px] font-medium text-cv-text-muted">Provider</span>
                      <CvSelect
                        v-model="offlineLiveLLMProvider"
                        :options="offlineLiveLLMProviderOptions"
                        class="mt-1.5"
                      />
                    </label>
                    <label class="block">
                      <span class="text-[12px] font-medium text-cv-text-muted">{{ t('common.model') }}</span>
                      <CvSelect
                        v-model="offlineLiveLLMModel"
                        :options="offlineLiveLLMModelOptions"
                        class="mt-1.5"
                      />
                    </label>
                  </div>
                  <p class="text-[11px] leading-5 text-cv-text-muted">
                    {{ t('offlineVideo.offlineLiveLlmHint') }}
                  </p>

                  <div class="grid gap-3 md:grid-cols-[90px_minmax(0,1fr)_minmax(0,1fr)] md:items-end">
                    <span class="text-[13px] font-medium text-cv-text-secondary md:pb-3">{{ t('characterEdit.omniModel') }}</span>
                    <label class="block">
                      <span class="text-[12px] font-medium text-cv-text-muted">Provider</span>
                      <input
                        type="text"
                        readonly
                        :value="t('offlineVideo.offlineLiveTtsOmniBadge')"
                        class="mt-1.5 h-[42px] w-full rounded-cv-md border border-cv-border bg-cv-elevated px-4 text-sm text-cv-text-secondary focus:outline-none"
                      >
                    </label>
                    <label class="block">
                      <span class="text-[12px] font-medium text-cv-text-muted">{{ t('common.model') }}</span>
                      <CvSelect
                        v-model="offlineLiveTTSModel"
                        :options="offlineLiveTTSModelOptions"
                        class="mt-1.5"
                      />
                    </label>
                  </div>

                  <div class="grid gap-3 md:grid-cols-[90px_minmax(0,1fr)] md:items-start">
                    <span class="text-[13px] font-medium text-cv-text-secondary md:pt-3">{{ t('characterEdit.lineVoice') }}</span>
                    <label class="block">
                      <div class="flex items-start gap-3">
                        <CvSelect
                          v-model="offlineLiveTTSVoice"
                          :options="offlineLiveTTSVoiceOptions"
                          :success="offlineLiveVoiceCheckSucceeded"
                          class="min-w-0 flex-1"
                        />
                        <button
                          type="button"
                          class="cv-pi-button cv-pi-button--compact h-[42px] shrink-0 disabled:cursor-not-allowed disabled:opacity-40"
                          :disabled="testingOfflineLiveVoice || !offlineLiveTTSVoice.trim()"
                          @click="handleOfflineLiveCheckVoice"
                        >
                          {{ t('common.check') }}
                        </button>
                      </div>
                      <p class="mt-2 text-[11px] leading-5 text-cv-text-muted">
                        {{ t('characterEdit.canPreviewAt') }}
                        <a
                          :href="QWEN_OMNI_VOICE_LIST_URL"
                          target="_blank"
                          rel="noopener noreferrer"
                          class="underline underline-offset-2 transition-colors hover:text-cv-text"
                        >
                          {{ t('characterEdit.qwenOmniVoiceList') }}
                        </a>
                        {{ t('characterEdit.previewVoice') }}
                      </p>
                      <p
                        v-if="offlineLiveVoiceTestMessage"
                        class="mt-2 whitespace-pre-wrap break-all text-[11px] leading-5 text-cv-danger"
                      >
                        {{ offlineLiveVoiceTestMessage }}
                      </p>
                    </label>
                  </div>
                </div>
              </div>

              <div class="idle-picker-card">
                <div class="idle-picker-header">
                  <div class="idle-picker-header-row">
                    <h3>{{ t('offlineVideo.offlineLiveSelectIdle') }}</h3>
                    <RouterLink :to="videoManagerPath" class="video-manager-link">
                      {{ t('offlineVideo.offlineLiveOpenVideoManager') }}
                    </RouterLink>
                  </div>
                  <p>{{ t('offlineVideo.offlineLiveSelectIdleHint') }}</p>
                  <p class="idle-picker-scope">{{ t('offlineVideo.offlineLiveIdleOnlyFromManager') }}</p>
                  <p class="idle-picker-scope">{{ t('offlineVideo.offlineLiveIdleQueueHint') }}</p>
                  <p class="idle-picker-count">
                    {{ selectedIdleVideoUrls.length === 1
                      ? t('offlineVideo.offlineLiveIdleSelectedOne')
                      : t('offlineVideo.offlineLiveIdleSelectedCount', { count: selectedIdleVideoUrls.length }) }}
                  </p>
                </div>

                <div v-if="managedVideoOptions.length === 0" class="notice warning">
                  {{ t('offlineVideo.offlineLiveNoVideos') }}
                </div>
                <div v-else-if="idleVideoOptions.length === 0" class="notice warning">
                  {{ t('offlineVideo.offlineLiveNoIdleFiltered') }}
                </div>

                <div v-else class="managed-video-scroll">
                  <div class="managed-video-grid">
                  <article
                    v-for="option in idleVideoOptions"
                    :key="`idle-${option.id}`"
                    class="managed-video-card"
                    :class="{ 'managed-video-card--active': selectedIdleVideoUrls.includes(option.url) }"
                  >
                    <button
                      type="button"
                      class="video-preview video-preview--playable"
                      :aria-label="t('offlineVideo.playVideoAria', { title: option.label })"
                      @click="openManagedVideoPreview(option)"
                    >
                      <video
                        :src="option.url"
                        class="video-preview-media"
                        muted
                        playsinline
                        preload="metadata"
                      />
                      <span class="video-preview-play-overlay" aria-hidden="true">
                        <span class="video-preview-play-icon">▶</span>
                      </span>
                    </button>
                    <div class="managed-video-card-body">
                      <strong>{{ option.label }}</strong>
                      <button
                        type="button"
                        class="managed-video-select-btn"
                        :class="{ 'managed-video-select-btn--active': selectedIdleVideoUrls.includes(option.url) }"
                        @click="toggleIdleVideo(option.url)"
                      >
                        {{ selectedIdleVideoUrls.includes(option.url) ? t('offlineVideo.offlineLiveSelected') : t('offlineVideo.offlineLivePickIdle') }}
                      </button>
                    </div>
                  </article>
                  </div>
                </div>
              </div>

              <div class="idle-picker-card">
                <div class="idle-picker-header">
                  <div class="idle-picker-header-row">
                    <h3>{{ t('offlineVideo.offlineLiveSelectAction') }}</h3>
                    <RouterLink :to="videoManagerPath" class="video-manager-link">
                      {{ t('offlineVideo.offlineLiveOpenVideoManager') }}
                    </RouterLink>
                  </div>
                  <p>{{ t('offlineVideo.offlineLiveSelectActionHint') }}</p>
                  <p class="idle-picker-scope">{{ t('offlineVideo.offlineLiveIdleOnlyFromManager') }}</p>
                  <p class="idle-picker-count">
                    {{ t('offlineVideo.offlineLiveActionSelectedCount', { count: selectedActionVideoUrls.length }) }}
                  </p>
                </div>

                <div v-if="managedVideoOptions.length === 0" class="notice warning">
                  {{ t('offlineVideo.offlineLiveNoVideos') }}
                </div>
                <div v-else-if="actionVideoOptions.length === 0" class="notice warning">
                  {{ t('offlineVideo.offlineLiveNoActionFiltered') }}
                </div>

                <div v-else class="managed-video-scroll">
                  <div class="managed-video-grid">
                  <article
                    v-for="option in actionVideoOptions"
                    :key="`action-${option.id}`"
                    class="managed-video-card"
                    :class="{ 'managed-video-card--active': selectedActionVideoUrls.includes(option.url) }"
                  >
                    <button
                      type="button"
                      class="video-preview video-preview--playable"
                      :aria-label="t('offlineVideo.playVideoAria', { title: option.label })"
                      @click="openManagedVideoPreview(option)"
                    >
                      <video
                        :src="option.url"
                        class="video-preview-media"
                        muted
                        playsinline
                        preload="metadata"
                      />
                      <span class="video-preview-play-overlay" aria-hidden="true">
                        <span class="video-preview-play-icon">▶</span>
                      </span>
                    </button>
                    <div class="managed-video-card-body">
                      <strong>{{ option.label }}</strong>
                      <button
                        type="button"
                        class="managed-video-select-btn"
                        :class="{ 'managed-video-select-btn--active': selectedActionVideoUrls.includes(option.url) }"
                        @click="toggleActionVideo(option.url)"
                      >
                        {{ selectedActionVideoUrls.includes(option.url) ? t('offlineVideo.offlineLiveSelected') : t('offlineVideo.offlineLivePickAction') }}
                      </button>
                    </div>
                  </article>
                  </div>
                </div>
              </div>

              <div class="idle-picker-card">
                <div class="idle-picker-header">
                  <div class="idle-picker-header-row">
                    <h3>{{ t('offlineVideo.offlineLiveSelectSing') }}</h3>
                    <RouterLink :to="videoManagerPath" class="video-manager-link">
                      {{ t('offlineVideo.offlineLiveOpenVideoManager') }}
                    </RouterLink>
                  </div>
                  <p>{{ t('offlineVideo.offlineLiveSelectSingHint') }}</p>
                  <p class="idle-picker-scope">{{ t('offlineVideo.offlineLiveSingQueueHint') }}</p>
                  <p class="idle-picker-scope">{{ t('offlineVideo.offlineLiveIdleOnlyFromManager') }}</p>
                  <p class="idle-picker-count">
                    {{ t('offlineVideo.offlineLiveSingSelectedCount', { count: selectedSingVideoUrls.length }) }}
                  </p>
                </div>

                <div v-if="managedVideoOptions.length === 0" class="notice warning">
                  {{ t('offlineVideo.offlineLiveNoVideos') }}
                </div>
                <div v-else-if="singVideoOptions.length === 0" class="notice warning">
                  {{ t('offlineVideo.offlineLiveNoSingFiltered') }}
                </div>

                <div v-else class="managed-video-scroll">
                  <div class="managed-video-grid">
                  <article
                    v-for="option in singVideoOptions"
                    :key="`sing-${option.id}`"
                    class="managed-video-card"
                    :class="{ 'managed-video-card--active': selectedSingVideoUrls.includes(option.url) }"
                  >
                    <button
                      type="button"
                      class="video-preview video-preview--playable"
                      :aria-label="t('offlineVideo.playVideoAria', { title: option.label })"
                      @click="openManagedVideoPreview(option)"
                    >
                      <video
                        :src="option.url"
                        class="video-preview-media"
                        muted
                        playsinline
                        preload="metadata"
                      />
                      <span class="video-preview-play-overlay" aria-hidden="true">
                        <span class="video-preview-play-icon">▶</span>
                      </span>
                    </button>
                    <div class="managed-video-card-body">
                      <strong>{{ option.label }}</strong>
                      <button
                        type="button"
                        class="managed-video-select-btn"
                        :class="{ 'managed-video-select-btn--active': selectedSingVideoUrls.includes(option.url) }"
                        @click="toggleSingVideo(option.url)"
                      >
                        {{ selectedSingVideoUrls.includes(option.url) ? t('offlineVideo.offlineLiveSelected') : t('offlineVideo.offlineLivePickSing') }}
                      </button>
                    </div>
                  </article>
                  </div>
                </div>
              </div>

              <div class="idle-picker-card">
                <div class="idle-picker-header">
                  <h3>{{ t('offlineVideo.offlineLiveDanmakuTitle') }}</h3>
                  <p>{{ t('offlineVideo.offlineLiveDanmakuHint') }}</p>
                </div>
                <div
                  class="cv-pi-segment danmaku-platform-segment h-[42px] w-[390px] max-w-full grid-cols-3"
                  role="tablist"
                  :aria-label="t('offlineVideo.offlineLiveDanmakuTitle')"
                >
                  <button
                    type="button"
                    class="cv-pi-segment-item"
                    :class="{ 'cv-pi-segment-item--active': danmakuPlatform === 'none' }"
                    @click="danmakuPlatform = 'none'"
                  >
                    {{ t('offlineVideo.offlineLiveDanmakuNone') }}
                  </button>
                  <button
                    type="button"
                    class="cv-pi-segment-item"
                    :class="{ 'cv-pi-segment-item--active': danmakuPlatform === 'bilibili' }"
                    @click="danmakuPlatform = 'bilibili'"
                  >
                    {{ t('offlineVideo.offlineLiveDanmakuBilibili') }}
                  </button>
                  <button
                    type="button"
                    class="cv-pi-segment-item"
                    :class="{ 'cv-pi-segment-item--active': danmakuPlatform === 'douyin' }"
                    @click="danmakuPlatform = 'douyin'"
                  >
                    {{ t('offlineVideo.offlineLiveDanmakuDouyin') }}
                  </button>
                </div>
                <div v-if="danmakuPlatform === 'bilibili'" class="danmaku-form-fields">
                  <label class="field-label" for="offline-live-bilibili-room">{{ t('offlineVideo.offlineLiveBilibiliRoomId') }}</label>
                  <input
                    id="offline-live-bilibili-room"
                    v-model="bilibiliRoomId"
                    class="danmaku-text-input"
                    type="number"
                    min="1"
                    :placeholder="t('offlineVideo.offlineLiveBilibiliRoomIdPlaceholder')"
                  >
                </div>
                <div v-else-if="danmakuPlatform === 'douyin'" class="danmaku-form-fields">
                  <label class="field-label" for="offline-live-douyin-room">{{ t('offlineVideo.offlineLiveDouyinRoom') }}</label>
                  <input
                    id="offline-live-douyin-room"
                    v-model="douyinRoom"
                    class="danmaku-text-input"
                    type="text"
                    :placeholder="t('offlineVideo.offlineLiveDouyinRoomPlaceholder')"
                  >
                  <p class="field-hint">{{ t('offlineVideo.offlineLiveDouyinRelayHint') }}</p>
                </div>
              </div>

              <div class="action-row">
                <button
                  class="cv-pi-button cv-pi-button--primary"
                  type="button"
                  :disabled="connecting || !canLaunch"
                  @click="launch"
                >
                  {{ connecting ? t('launch.launching') : t('offlineVideo.offlineLiveStart') }}
                </button>
              </div>

              <div
                v-if="previewingManagedVideo?.url"
                class="video-player-modal-backdrop"
                @click.self="closeManagedVideoPreview"
              >
                <section
                  class="video-player-modal"
                  role="dialog"
                  aria-modal="true"
                  :aria-label="t('offlineVideo.videoPlayerTitle')"
                >
                  <div class="video-player-modal-header">
                    <h2>{{ previewingManagedVideo?.label }}</h2>
                    <button
                      type="button"
                      class="video-player-modal-close"
                      :aria-label="t('offlineVideo.closeVideoPlayer')"
                      @click="closeManagedVideoPreview"
                    >
                      ×
                    </button>
                  </div>
                  <video
                    class="video-player-media"
                    :src="previewingManagedVideo?.url"
                    controls
                    autoplay
                    playsinline
                    preload="metadata"
                  />
                </section>
              </div>
            </template>

            <template v-else>
              <div v-if="runtimeConfigMismatch" class="notice warning">
                {{ t('launch.runtimeMismatch', { configured: configuredDefaultModel, active: activeAvatarModel }) }}
              </div>
              <div v-if="saveMessage" class="notice success">{{ saveMessage }}</div>

              <div v-if="!LOCAL_ONLY_DEPLOYMENT && isBaiduXilingCharacter" class="config-card">
                <div
                  v-for="row in baiduXilingInfoRows"
                  :key="row.label"
                  class="config-row"
                >
                  <p class="row-label">{{ row.label }}</p>
                  <p class="row-value" :title="row.value">{{ row.value }}</p>
                </div>
              </div>

              <div v-else-if="!LOCAL_ONLY_DEPLOYMENT && isXunfeiCharacter" class="config-card">
                <div
                  v-for="row in xunfeiInfoRows"
                  :key="row.label"
                  class="config-row"
                >
                  <p class="row-label">{{ row.label }}</p>
                  <p class="row-value" :title="row.value">{{ row.value }}</p>
                </div>
              </div>

              <div v-else-if="!LOCAL_ONLY_DEPLOYMENT && isViduCharacter" class="notice success">
                {{ t('launch.viduNativeRuntime') }}
              </div>

              <div v-for="section in configSections" :key="section.title" class="config-card">
                <button
                  class="config-header"
                  type="button"
                  :aria-expanded="!section.collapsed"
                  @click="section.collapsed = !section.collapsed"
                >
                  <span class="config-header-main">
                    <span class="config-title">{{ launchSectionTitle(section) }}</span>
                    <span
                      v-if="sectionHasRestartPending(section)"
                      class="restart-indicator"
                    >
                      <span class="restart-badge">{{ t('launch.restartRequired') }}</span>
                      <span
                        class="restart-help"
                        aria-hidden="true"
                      >
                        ?
                      </span>
                      <span role="tooltip" class="restart-tooltip">
                        {{ restartBadgeHint }}
                      </span>
                    </span>
                  </span>
                  <span
                    class="config-chevron"
                    :class="{ expanded: !section.collapsed }"
                    aria-hidden="true"
                  />
                </button>

                <div v-if="!section.collapsed">
                  <div v-for="param in section.params" :key="param.name" class="config-row">
                    <div class="min-w-0">
                      <p class="row-label strong">{{ param.name }}</p>
                      <p class="row-path">{{ param.path }}</p>
                    </div>
                    <div class="param-control">
                      <span v-if="param.readonly" class="readonly-value">{{ param.value }}</span>
                      <CvSelect
                        v-else-if="param.options && param.options.length > 0"
                        :modelValue="String(param.value)"
                        :options="param.options"
                        class="w-[200px]"
                        @update:modelValue="param.value = $event"
                      />
                      <input
                        v-else-if="typeof param.value === 'number'"
                        type="text"
                        inputmode="numeric"
                        :value="param.value"
                        :style="{ width: inputWidth(param.value) }"
                        class="param-input"
                        @input="param.value = Number(($event.target as HTMLInputElement).value) || 0"
                      >
                      <input
                        v-else
                        v-model="param.value"
                        type="text"
                        :style="{ width: inputWidth(param.value) }"
                        class="param-input"
                      >
                      <span v-if="param.readonly && param.requires_restart" class="lock-icon">
                        <svg class="inline h-3.5 w-3" viewBox="0 0 9 11" fill="none">
                          <ellipse cx="4.5" cy="3.5" rx="3" ry="3" stroke="#73737d" stroke-width="1.2" />
                          <rect x="0" y="5" width="9" height="6" rx="1" fill="#73737d" />
                        </svg>
                      </span>
                    </div>
                  </div>
                </div>
              </div>

              <div class="action-row">
                <button class="cv-pi-button" type="button" :disabled="!hasChanges || saving" @click="saveConfig">
                  {{ saving ? t('common.saving') : t('launch.saveConfig') }}
                </button>
                <button class="cv-pi-button cv-pi-button--primary" type="button" :disabled="connecting || !canLaunch" @click="launch">
                  {{ connecting ? t('launch.launching') : t('launch.launch') }}
                </button>
              </div>
            </template>
          </section>
        </div>
      </template>
    </main>
  </div>
</template>

<style scoped>
.character-panel,
.production-panel {
  border: 1px solid var(--color-cv-border);
  border-radius: 8px;
  background: var(--color-cv-surface);
}

.character-panel {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 22px;
}

.avatar-shell {
  aspect-ratio: 3 / 4;
  overflow: hidden;
  border: 1px solid #2b3542;
  background: #0b0d12;
}

.info-grid {
  display: grid;
  grid-template-columns: 96px minmax(0, 1fr);
  gap: 10px 14px;
  font-size: 13px;
}

.info-grid span {
  color: #798394;
}

.info-grid strong {
  min-width: 0;
  overflow: hidden;
  color: #f0f4f8;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.production-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 22px;
}

.idle-picker-card {
  display: flex;
  flex-direction: column;
  gap: 16px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: #0b0d12;
  padding: 20px;
}

.idle-picker-header h3 {
  margin: 0;
  color: #f4f6f8;
  font-size: 16px;
  font-weight: 650;
}

.idle-picker-header p {
  margin: 8px 0 0;
  color: #8d96a6;
  font-size: 13px;
  line-height: 1.5;
}

.idle-picker-count {
  margin-top: 6px !important;
  color: #8fe8ef !important;
  font-size: 12px !important;
}

.idle-picker-scope {
  margin: 6px 0 0;
  color: #6b7380;
  font-size: 12px;
  line-height: 1.45;
}

.idle-picker-header-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.idle-picker-header-row h3 {
  margin: 0;
}

.video-manager-link {
  flex-shrink: 0;
  color: #34e6f3;
  font-size: 12px;
  font-weight: 600;
  text-decoration: none;
}

.video-manager-link:hover {
  text-decoration: underline;
}

.managed-video-scroll {
  max-height: min(420px, 52vh);
  overflow-y: auto;
  padding-right: 4px;
  scrollbar-gutter: stable;
}

.managed-video-scroll::-webkit-scrollbar {
  width: 8px;
}

.managed-video-scroll::-webkit-scrollbar-thumb {
  border-radius: 4px;
  background: rgba(143, 232, 239, 0.35);
}

.managed-video-scroll::-webkit-scrollbar-track {
  background: rgba(255, 255, 255, 0.04);
}

.managed-video-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 14px;
}

.managed-video-card {
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: #10141b;
  transition: border-color 150ms ease, box-shadow 150ms ease;
}

.managed-video-card--active {
  border-color: rgba(143, 232, 239, 0.55);
  box-shadow: 0 0 0 1px rgba(52, 230, 243, 0.18);
}

.managed-video-card-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px 12px;
}

.managed-video-card-body strong {
  color: #e8edf3;
  font-size: 13px;
  font-weight: 650;
  line-height: 1.4;
}

.managed-video-select-btn {
  align-self: flex-start;
  border: 1px solid rgba(255, 255, 255, 0.14);
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.04);
  padding: 4px 10px;
  color: #b8c0cc;
  font-size: 12px;
  cursor: pointer;
}

.managed-video-select-btn--active {
  border-color: rgba(143, 232, 239, 0.55);
  background: rgba(52, 230, 243, 0.12);
  color: #8fe8ef;
}

.video-preview {
  position: relative;
  display: block;
  width: 100%;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  border: 0;
  border-bottom: 1px solid #242b36;
  background: #07080b;
  padding: 0;
}

.video-preview-media {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.video-preview--playable {
  cursor: pointer;
}

.video-preview--playable::after {
  position: absolute;
  inset: 0;
  background: linear-gradient(180deg, transparent 58%, rgba(0, 0, 0, 0.3));
  content: "";
  opacity: 0.72;
  pointer-events: none;
}

.video-preview-play-overlay {
  position: absolute;
  inset: 0;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.18);
  opacity: 0;
  transition: opacity 160ms ease, background 160ms ease;
}

.video-preview-play-icon {
  display: inline-flex;
  width: 46px;
  height: 46px;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(244, 247, 251, 0.62);
  background: rgba(7, 8, 11, 0.58);
  color: #f4f7fb;
  font-size: 16px;
  font-weight: 800;
}

.video-preview--playable:hover .video-preview-play-overlay,
.video-preview--playable:focus-visible .video-preview-play-overlay {
  background: rgba(0, 0, 0, 0.28);
  opacity: 1;
}

.video-player-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 80;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.72);
  padding: 24px;
}

.video-player-modal {
  width: min(920px, 100%);
  max-height: calc(100vh - 48px);
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 10px;
  background: #0b0d12;
}

.video-player-modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  padding: 12px 16px;
}

.video-player-modal-header h2 {
  margin: 0;
  color: #f4f6f8;
  font-size: 15px;
  font-weight: 650;
}

.video-player-modal-close {
  border: 0;
  background: transparent;
  color: #8d96a6;
  font-size: 24px;
  line-height: 1;
  cursor: pointer;
}

.video-player-media {
  display: block;
  width: 100%;
  max-height: calc(100vh - 120px);
  background: #000;
}

.notice {
  border: 1px solid;
  padding: 10px 12px;
  font-size: 13px;
  line-height: 20px;
}

.notice.warning {
  border-color: rgba(255, 179, 107, 0.3);
  background: rgba(255, 179, 107, 0.08);
  color: #ffca9a;
}

.notice.error {
  border-color: rgba(239, 68, 68, 0.35);
  background: rgba(239, 68, 68, 0.1);
  color: #fca5a5;
}

.notice.success {
  border-color: rgba(52, 230, 243, 0.25);
  background: rgba(52, 230, 243, 0.08);
  color: #8fe8ef;
}

.config-card {
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: #0b0d12;
}

.config-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.config-header {
  display: flex;
  min-width: 0;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  background: #0b0e14;
  padding: 16px 20px;
  text-align: left;
  cursor: pointer;
  transition: background 160ms ease;
}

.config-header:hover {
  background: #0f131a;
}

.config-header:focus-visible {
  outline: 2px solid rgba(52, 230, 243, 0.45);
  outline-offset: -2px;
}

.config-row {
  min-width: 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
  padding: 16px 20px;
}

.config-row:last-child {
  border-bottom: 0;
}

.config-header-main {
  position: relative;
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.config-title {
  color: #c8d0dc;
  font-size: 14px;
  font-weight: 800;
}

.config-chevron {
  display: inline-flex;
  width: 18px;
  height: 18px;
  flex: 0 0 18px;
  align-items: center;
  justify-content: center;
  transform: rotate(180deg);
  transition: transform 160ms ease;
}

.config-chevron::before {
  width: 8px;
  height: 8px;
  border-bottom: 2px solid #34e6f3;
  border-right: 2px solid #34e6f3;
  content: "";
  transform: rotate(-45deg);
}

.config-chevron.expanded {
  transform: rotate(90deg);
}

.restart-indicator {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.restart-badge {
  border: 1px solid rgba(255, 147, 70, 0.4);
  background: rgba(255, 147, 70, 0.12);
  padding: 2px 8px;
  color: #ff9346;
  font-size: 11px;
}

.restart-help {
  display: inline-flex;
  height: 18px;
  min-width: 18px;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(255, 147, 70, 0.45);
  border-radius: 999px;
  background: rgba(255, 147, 70, 0.06);
  color: #ff9346;
  font-size: 11px;
  font-weight: 700;
}

.restart-tooltip {
  pointer-events: none;
  visibility: hidden;
  position: absolute;
  left: 0;
  top: calc(100% + 6px);
  z-index: 30;
  width: min(288px, calc(100vw - 3rem));
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: #12161c;
  padding: 8px 12px;
  color: #b8c0cc;
  font-size: 11px;
  line-height: 1.6;
  opacity: 0;
  box-shadow: 0 18px 40px rgba(0, 0, 0, 0.3);
  transition: opacity 150ms ease;
}

.restart-indicator:hover .restart-tooltip,
.config-header:focus-visible .restart-tooltip {
  visibility: visible;
  opacity: 1;
}

.row-label {
  color: #505864;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0;
  text-transform: uppercase;
}

.row-label.strong {
  color: #c8d0dc;
  font-size: 13px;
  text-transform: none;
}

.row-path {
  margin-top: 4px;
  color: #505864;
  font-size: 11px;
}

.row-value,
.readonly-value {
  min-width: 0;
  overflow: hidden;
  color: #c8d0dc;
  font-size: 13px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.param-control {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.param-input {
  border: 1px solid rgba(72, 80, 92, 0.4);
  background: #0f1218;
  color: #a0a8b4;
  padding: 4px 8px;
  text-align: right;
  font-size: 13px;
  outline: none;
  transition: border-color 160ms ease;
}

.param-input:focus {
  border-color: rgba(52, 230, 243, 0.6);
}

.lock-icon {
  color: #505864;
  font-size: 12px;
}

.action-row {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 14px;
  padding-top: 4px;
}

.danmaku-platform-segment {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.danmaku-form-fields {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-width: 420px;
}

.field-label {
  color: #c4ccd8;
  font-size: 12px;
  font-weight: 800;
}

.danmaku-text-input {
  width: 100%;
  min-height: 44px;
  padding: 0 14px;
  border: 1px solid #303a49;
  background: #0b0d12;
  color: #f4f7fb;
  font-size: 14px;
  outline: none;
}

.danmaku-text-input:focus {
  border-color: #34e6f3;
}

.field-hint {
  margin: 0;
  color: #798394;
  font-size: 12px;
  line-height: 1.5;
}

@media (max-width: 860px) {
  .config-row {
    align-items: flex-start;
    flex-direction: column;
  }

  .param-control {
    width: 100%;
    justify-content: flex-start;
  }
}
</style>
