<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  advanceStandbyPlaylist,
  createStandbyPlaylist,
  peekNextStandbySrc,
  requestStandbySwitch,
  syncStandbyPlaylistOrder,
  type StandbyPlaylistState,
} from '../utils/standbyPlaylist'
const emit = defineEmits<{
  standbyFailed: []
  answerEnded: []
  standbySourceChange: [url: string]
}>()

const props = defineProps<{
  displayMode?: 'webrtc' | 'standby' | 'answer' | 'placeholder'
  standbySrc?: string
  standbySources?: string[]
  standbyImageSrc?: string
  answerSrc?: string
  answerLoop?: boolean
}>()

const FADE_MS = 480

const videoRef = ref<HTMLVideoElement | null>(null)
const standbyVideoRefs = ref<[HTMLVideoElement | null, HTMLVideoElement | null]>([null, null])
const answerVideoRef = ref<HTMLVideoElement | null>(null)
const standbyImageRef = ref<HTMLImageElement | null>(null)
const containerRef = ref<HTMLDivElement | null>(null)
const standbyPlaylist = ref<StandbyPlaylistState>(createStandbyPlaylist([]))
const currentDisplayMode = computed(() => props.displayMode || 'placeholder')
const activeAspectRatio = ref<number | null>(null)
const containerWidth = ref(0)
const containerHeight = ref(0)
let containerResizeObserver: ResizeObserver | null = null

/** Mounted answer source (kept briefly after clear so opacity can fade out). */
const mountedAnswerSrc = ref('')
/** True only after answer has a decoded frame — prevents black flash on show. */
const answerReady = ref(false)
/** Controls answer opacity independently of displayMode for exit fade. */
const answerShown = ref(false)
let answerFadeTimer: ReturnType<typeof setTimeout> | null = null
let endingAnswer = false
const answerLoopActive = ref(false)
/** Dual-buffer standby URLs for crossfade (index 0 | 1). */
const standbySlotSrc = ref<[string, string]>(['', ''])
const activeStandbySlot = ref<0 | 1>(0)
let standbyFadeTimer: ReturnType<typeof setTimeout> | null = null
let standbyCrossfadeInProgress = false
/** Incoming slot during standby switch (same reveal pattern as action/answer overlay). */
const standbyFadeIncoming = ref<0 | 1 | null>(null)
const standbyIncomingReady = ref(false)
const standbyIncomingReveal = ref(false)

const effectiveSources = computed(() => {
  if (props.standbySources?.length) return props.standbySources
  if (props.standbySrc) return [props.standbySrc]
  return []
})

const currentStandbySrc = computed(() => standbyPlaylist.value.currentSrc)

const shouldLoop = computed(() => effectiveSources.value.length <= 1)

const hasMountedStandbySlot = computed(() =>
  !!standbySlotSrc.value[0] || !!standbySlotSrc.value[1],
)

// Standby stays visible under answer so crossfade never reveals black stage.
const standbyLayerVisible = computed(() => {
  if (currentDisplayMode.value === 'standby' || currentDisplayMode.value === 'answer') {
    return true
  }
  if (!!mountedAnswerSrc.value) return true
  // Offline / cached idle: keep MP4 visible while displayMode catches up (avoid black flash).
  if (currentDisplayMode.value === 'placeholder' && hasMountedStandbySlot.value) {
    return true
  }
  return false
})

const answerLayerVisible = computed(() => answerShown.value && answerReady.value)

const stageStyle = computed(() => {
  const width = containerWidth.value
  const height = containerHeight.value
  const aspectRatio = activeAspectRatio.value
  if (!width || !height || !aspectRatio) {
    return undefined
  }

  const containerAspectRatio = width / height
  if (containerAspectRatio > aspectRatio) {
    return {
      width: `${Math.round(height * aspectRatio)}px`,
      height: `${height}px`,
    }
  }

  return {
    width: `${width}px`,
    height: `${Math.round(width / aspectRatio)}px`,
  }
})

function clearAnswerFadeTimer() {
  if (answerFadeTimer) {
    clearTimeout(answerFadeTimer)
    answerFadeTimer = null
  }
}

function clearStandbyFadeTimer() {
  if (standbyFadeTimer) {
    clearTimeout(standbyFadeTimer)
    standbyFadeTimer = null
  }
}

function setStandbyVideoRef(index: 0 | 1, el: unknown) {
  standbyVideoRefs.value[index] = el instanceof HTMLVideoElement ? el : null
}

function activeStandbyVideoEl(): HTMLVideoElement | null {
  return standbyVideoRefs.value[activeStandbySlot.value]
}

function pauseAllStandbyVideos() {
  for (const el of standbyVideoRefs.value) {
    el?.pause()
  }
}

function waitForStandbyLoaded(el: HTMLVideoElement): Promise<void> {
  if (el.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
    return Promise.resolve()
  }
  return new Promise((resolve) => {
    const done = () => {
      el.removeEventListener('loadeddata', done)
      el.removeEventListener('canplay', done)
      resolve()
    }
    el.addEventListener('loadeddata', done)
    el.addEventListener('canplay', done)
  })
}

async function resolveStandbyVideoRef(index: 0 | 1): Promise<HTMLVideoElement | null> {
  for (let attempt = 0; attempt < 8; attempt += 1) {
    await nextTick()
    const el = standbyVideoRefs.value[index]
    if (el) return el
  }
  return null
}

async function playStandbySlot(index: 0 | 1) {
  const el = await resolveStandbyVideoRef(index)
  if (!el || !standbySlotSrc.value[index]) return
  try {
    await el.play()
  } catch {
    /* autoplay restrictions */
  }
}

function clearStandbySlots() {
  clearStandbyFadeTimer()
  pauseAllStandbyVideos()
  standbySlotSrc.value = ['', '']
  standbyCrossfadeInProgress = false
  standbyFadeIncoming.value = null
  standbyIncomingReady.value = false
  standbyIncomingReveal.value = false
}

/** Same visibility rules as answer overlay: hidden until ready, then 480ms opacity fade via `.video-hidden`. */
function isStandbySlotVisible(index: 0 | 1): boolean {
  if (!standbyLayerVisible.value || !standbySlotSrc.value[index]) return false
  if (standbyFadeIncoming.value === index) {
    return standbyIncomingReady.value && standbyIncomingReveal.value
  }
  if (standbyCrossfadeInProgress && standbyFadeIncoming.value !== null && index === activeStandbySlot.value) {
    return true
  }
  return index === activeStandbySlot.value && !standbyCrossfadeInProgress
}

function inactiveStandbySlot(): 0 | 1 {
  return activeStandbySlot.value === 0 ? 1 : 0
}

function standbySlotStyle(index: 0 | 1) {
  const zIndex =
    standbyFadeIncoming.value === index
      ? 3
      : index === activeStandbySlot.value
        ? 2
        : 1
  return { zIndex }
}

function revealStandbyIncoming(incoming: 0 | 1) {
  if (standbyFadeIncoming.value !== incoming) return
  standbyIncomingReady.value = true
  syncActiveAspectRatio()
  standbyIncomingReveal.value = true
}

async function mountStandbyInitial(src: string) {
  clearStandbyFadeTimer()
  standbySlotSrc.value = [src, '']
  activeStandbySlot.value = 0
  await nextTick()
  void playStandbySlot(0)
  const el = await resolveStandbyVideoRef(0)
  if (el) {
    el.loop = shouldLoop.value
  }
  handleVideoIntrinsicSizeChange()
  emitStandbySource()
  void ensureNextStandbyPreloaded()
}

function waitNextPaint(): Promise<void> {
  return new Promise((resolve) => {
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  })
}

async function startStandbySlotPlayback(index: 0 | 1): Promise<boolean> {
  const el = standbyVideoRefs.value[index]
  if (!el || !standbySlotSrc.value[index]) return false
  el.loop = shouldLoop.value
  try {
    if (el.readyState < HTMLMediaElement.HAVE_CURRENT_DATA) {
      await waitForStandbyLoaded(el)
    }
    el.currentTime = 0
    await el.play()
    await waitNextPaint()
    return !el.paused
  } catch {
    await playStandbySlot(index)
    return !el.paused
  }
}

async function preloadStandbySlot(index: 0 | 1, url: string) {
  const trimmed = url.trim()
  if (!trimmed || standbyCrossfadeInProgress) return
  if (index === activeStandbySlot.value) return

  const existing = standbySlotSrc.value[index]
  if (existing === trimmed) {
    const el = standbyVideoRefs.value[index]
    if (el && el.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) return
  }

  standbySlotSrc.value[index] = trimmed
  await nextTick()
  const el = await resolveStandbyVideoRef(index)
  if (!el) return
  el.loop = shouldLoop.value
  if (el.readyState < HTMLMediaElement.HAVE_FUTURE_DATA) {
    await waitForStandbyLoaded(el)
  }
}

async function ensureNextStandbyPreloaded() {
  if (standbyCrossfadeInProgress || shouldLoop.value) return
  const next = peekNextStandbySrc(standbyPlaylist.value)
  if (!next || next === currentStandbySrc.value) return
  await preloadStandbySlot(inactiveStandbySlot(), next)
}

/** Standby switch — mirror action overlay: freeze outgoing, fade in incoming after canplay. */
async function crossfadeStandbyTo(nextSrc: string) {
  const trimmed = nextSrc.trim()
  if (!trimmed) return
  if (standbyCrossfadeInProgress) return

  clearStandbyFadeTimer()
  standbyCrossfadeInProgress = true
  standbyIncomingReady.value = false
  standbyIncomingReveal.value = false

  const outgoing: 0 | 1 = activeStandbySlot.value
  const incoming: 0 | 1 = outgoing === 0 ? 1 : 0
  standbyFadeIncoming.value = incoming

  const outEl = standbyVideoRefs.value[outgoing]
  if (outEl) {
    if (outEl.ended && Number.isFinite(outEl.duration) && outEl.duration > 0) {
      outEl.currentTime = Math.max(0, outEl.duration - 0.04)
    }
    outEl.pause()
  }

  if (standbySlotSrc.value[incoming] !== trimmed) {
    standbySlotSrc.value[incoming] = trimmed
    await nextTick()
  }

  const playing = await startStandbySlotPlayback(incoming)
  if (!playing) {
    standbyCrossfadeInProgress = false
    standbyFadeIncoming.value = null
    standbyIncomingReady.value = false
    standbyIncomingReveal.value = false
    void resumeStandbyPlayback()
    return
  }

  const inEl = standbyVideoRefs.value[incoming]
  if (inEl && inEl.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
    revealStandbyIncoming(incoming)
  }

  emitStandbySource()

  standbyFadeTimer = setTimeout(() => {
    standbyFadeTimer = null
    standbySlotSrc.value[outgoing] = ''
    activeStandbySlot.value = incoming
    standbyFadeIncoming.value = null
    standbyIncomingReady.value = false
    standbyIncomingReveal.value = false
    standbyCrossfadeInProgress = false
    handleVideoIntrinsicSizeChange()
    void playStandbySlot(incoming)
    void ensureNextStandbyPreloaded()
  }, FADE_MS)
}

function emitStandbySource() {
  const url = standbyPlaylist.value.currentSrc
  if (url) emit('standbySourceChange', url)
}

function onStandbyEnded(ev: Event) {
  const el = ev.target
  if (!(el instanceof HTMLVideoElement)) return
  const slot = standbyVideoRefs.value.indexOf(el) as 0 | 1 | -1
  if (slot < 0 || slot !== activeStandbySlot.value) return
  if (standbyCrossfadeInProgress) return
  advanceStandby()
}

/** Advance to the next idle clip in the rotating standby set. */
function advanceStandby() {
  standbyPlaylist.value = advanceStandbyPlaylist(standbyPlaylist.value)
  emitStandbySource()
}

function onStandbyCanPlay(ev: Event) {
  const el = ev.target
  if (!(el instanceof HTMLVideoElement)) return
  const idx = standbyVideoRefs.value.indexOf(el) as 0 | 1 | -1
  if (idx !== 0 && idx !== 1) return
  if (standbyFadeIncoming.value === idx && !standbyIncomingReveal.value) {
    revealStandbyIncoming(idx)
  }
}

function onStandbyLoadedData(ev: Event) {
  handleVideoIntrinsicSizeChange()
  onStandbyCanPlay(ev)
  const el = ev.target
  if (!(el instanceof HTMLVideoElement)) return
  const idx = standbyVideoRefs.value.indexOf(el)
  if (idx !== 0 && idx !== 1) return
  const slot: 0 | 1 = idx
  if (
    isStandbySlotVisible(slot)
    && standbyLayerVisible.value
    && slot === activeStandbySlot.value
    && el.paused
  ) {
    void el.play().catch(() => {})
  }
  if (slot === activeStandbySlot.value) {
    emitStandbySource()
  }
}

function queueStandbySwitch(url: string) {
  standbyPlaylist.value = requestStandbySwitch(standbyPlaylist.value, url)
}

function onStandbyError(ev: Event) {
  const el = ev.target
  if (el instanceof HTMLVideoElement) {
    const idx = standbyVideoRefs.value.indexOf(el)
    if (idx === 0 || idx === 1) {
      standbySlotSrc.value[idx] = ''
    }
  }
  if (el instanceof HTMLVideoElement || el instanceof HTMLImageElement) {
    emit('standbyFailed')
  }
}

async function resumeStandbyPlayback() {
  await nextTick()
  const el = activeStandbyVideoEl()
  if (!el || !standbyLayerVisible.value) return
  try {
    if (el.paused) {
      await el.play()
    }
  } catch {
    /* autoplay / visibility restrictions */
  }
}

async function playAnswerElementWithEl(el: HTMLVideoElement) {
  el.loop = answerLoopActive.value
  try {
    el.currentTime = 0
    await el.play()
    if (el.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
      onAnswerCanPlay()
    }
    return
  } catch {
    /* fall through to muted bootstrap */
  }
  try {
    el.muted = true
    await el.play()
    el.muted = false
    if (el.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
      onAnswerCanPlay()
    }
  } catch (err) {
    console.warn('[VideoPlayer] overlay play blocked:', err)
  }
}

/** User-gesture path must call play() synchronously — do not await nextTick first. */
function playAnswerElement(fromUserGesture: boolean) {
  const runAfterDom = () => {
    void (async () => {
      await nextTick()
      const el = answerVideoRef.value
      if (!el || !mountedAnswerSrc.value) return
      await playAnswerElementWithEl(el)
    })()
  }

  if (fromUserGesture) {
    const el = answerVideoRef.value
    const url = mountedAnswerSrc.value
    if (el && url) {
      el.loop = answerLoopActive.value
      el.currentTime = 0
      const playPromise = el.play()
      void playPromise
        .then(() => {
          if (el.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
            onAnswerCanPlay()
          }
        })
        .catch(() => {
          runAfterDom()
        })
      return
    }
  }

  runAfterDom()
}

function applyAnswerOverlay(url: string, loop: boolean, fromUserGesture: boolean) {
  const trimmed = url.trim()
  if (!trimmed) return
  if (trimmed === mountedAnswerSrc.value) {
    if (answerLoopActive.value !== loop) {
      answerLoopActive.value = loop
      const el = answerVideoRef.value
      if (el) el.loop = loop
    }
    if (fromUserGesture) {
      const el = answerVideoRef.value
      if (el && (el.paused || el.ended)) {
        playAnswerElement(true)
      }
    }
    return
  }
  clearAnswerFadeTimer()
  endingAnswer = false
  answerLoopActive.value = loop
  answerShown.value = false
  answerReady.value = false
  mountedAnswerSrc.value = trimmed
  if (fromUserGesture) {
    const el = answerVideoRef.value
    if (el) {
      el.src = trimmed
    }
  }
  playAnswerElement(fromUserGesture)
}

function beginAnswerOverlay(url: string, loop: boolean) {
  applyAnswerOverlay(url, loop, true)
}

function syncAnswerOverlay(url: string, loop: boolean) {
  applyAnswerOverlay(url, loop, false)
}

function haltAnswerVideoElement() {
  answerLoopActive.value = false
  const el = answerVideoRef.value
  if (!el) return
  el.loop = false
  el.pause()
}

function finishAnswerPlayback() {
  if (endingAnswer) return
  endingAnswer = true
  answerShown.value = false
  haltAnswerVideoElement()
  void resumeStandbyPlayback()
  clearAnswerFadeTimer()
  answerFadeTimer = setTimeout(() => {
    answerFadeTimer = null
    answerReady.value = false
    mountedAnswerSrc.value = ''
    endingAnswer = false
    void resumeStandbyPlayback()
    emit('answerEnded')
  }, FADE_MS)
}

function onAnswerEnded() {
  if (endingAnswer) return
  if (answerLoopActive.value) {
    const el = answerVideoRef.value
    if (el) {
      el.currentTime = 0
      void el.play().catch(() => {})
    }
    return
  }
  finishAnswerPlayback()
}

function onAnswerCanPlay() {
  answerReady.value = true
  syncActiveAspectRatio()
  if (mountedAnswerSrc.value && (props.answerSrc || currentDisplayMode.value === 'answer' || currentDisplayMode.value === 'standby')) {
    answerShown.value = true
  }
}

function onAnswerError() {
  console.warn('[VideoPlayer] overlay video failed:', mountedAnswerSrc.value)
  finishAnswerPlayback()
}

function syncContainerSize() {
  const el = containerRef.value
  containerWidth.value = el?.clientWidth ?? 0
  containerHeight.value = el?.clientHeight ?? 0
}

function getIntrinsicAspectRatio(el: HTMLVideoElement | HTMLImageElement | null): number | null {
  if (!el) return null
  if (el instanceof HTMLImageElement) {
    if (!el.naturalWidth || !el.naturalHeight) return null
    return el.naturalWidth / el.naturalHeight
  }
  const { videoWidth, videoHeight } = el
  if (!videoWidth || !videoHeight) return null
  return videoWidth / videoHeight
}

function syncActiveAspectRatio() {
  const primary =
    answerLayerVisible.value
      ? answerVideoRef.value
      : currentDisplayMode.value === 'standby' || standbyLayerVisible.value
        ? (activeStandbyVideoEl() || standbyImageRef.value)
        : videoRef.value

  const fallback = activeStandbyVideoEl() || answerVideoRef.value || videoRef.value
  const aspectRatio =
    getIntrinsicAspectRatio(primary) ??
    getIntrinsicAspectRatio(fallback)

  // Keep previous ratio if next clip metadata is not ready yet — avoids stage snap/flash.
  if (aspectRatio) {
    activeAspectRatio.value = aspectRatio
  }
}

function handleVideoIntrinsicSizeChange() {
  syncActiveAspectRatio()
}

function bindVideoSizeListeners(nextEl: HTMLVideoElement | null, prevEl: HTMLVideoElement | null) {
  if (prevEl) {
    prevEl.removeEventListener('loadedmetadata', handleVideoIntrinsicSizeChange)
    prevEl.removeEventListener('resize', handleVideoIntrinsicSizeChange)
  }
  if (nextEl) {
    nextEl.addEventListener('loadedmetadata', handleVideoIntrinsicSizeChange)
    nextEl.addEventListener('resize', handleVideoIntrinsicSizeChange)
  }
  syncActiveAspectRatio()
}

watch(
  effectiveSources,
  (srcs) => {
    standbyPlaylist.value = syncStandbyPlaylistOrder(standbyPlaylist.value, srcs)
    if (!standbyPlaylist.value.currentSrc && srcs.length > 0) {
      standbyPlaylist.value = createStandbyPlaylist(srcs)
    }
    emitStandbySource()
    void ensureNextStandbyPreloaded()
  },
  { immediate: true },
)

watch(
  () => [
    standbyPlaylist.value.pendingAfterCurrent[0],
    currentStandbySrc.value,
  ] as const,
  () => {
    void ensureNextStandbyPreloaded()
  },
)

watch(currentStandbySrc, (next, prev) => {
  if (!next) {
    clearStandbySlots()
    return
  }
  if (!prev) {
    void mountStandbyInitial(next)
    return
  }
  if (next === prev) return
  void crossfadeStandbyTo(next)
}, { immediate: true })

watch(videoRef, (nextEl, prevEl) => {
  bindVideoSizeListeners(nextEl, prevEl)
})

watch(
  () => standbyVideoRefs.value[0],
  (nextEl, prevEl) => {
    bindVideoSizeListeners(nextEl, prevEl)
  },
)

watch(
  () => standbyVideoRefs.value[1],
  (nextEl, prevEl) => {
    bindVideoSizeListeners(nextEl, prevEl)
  },
)

watch(answerVideoRef, (nextEl, prevEl) => {
  bindVideoSizeListeners(nextEl, prevEl)
})

watch(
  () => [standbyLayerVisible.value, activeStandbySlot.value, currentStandbySrc.value] as const,
  async ([visible, slot, src]) => {
    syncActiveAspectRatio()
    if (!visible || !src) return
    await playStandbySlot(slot)
  },
)

watch(
  () => [props.answerSrc, props.answerLoop] as const,
  ([src, loop]) => {
    if (!src) {
      if (!mountedAnswerSrc.value) return
      if (endingAnswer) {
        clearAnswerFadeTimer()
        endingAnswer = false
        answerShown.value = false
        haltAnswerVideoElement()
        answerReady.value = false
        mountedAnswerSrc.value = ''
        emit('answerEnded')
        return
      }
      clearAnswerFadeTimer()
      endingAnswer = false
      if (answerShown.value) {
        finishAnswerPlayback()
      } else {
        haltAnswerVideoElement()
        answerReady.value = false
        mountedAnswerSrc.value = ''
        emit('answerEnded')
      }
      return
    }

    // Same URL as mounted (e.g. after beginAnswerOverlay): sync loop only — never clear fade timers or re-play().
    if (src === mountedAnswerSrc.value) {
      if (answerLoopActive.value !== !!loop) {
        answerLoopActive.value = !!loop
        const el = answerVideoRef.value
        if (el) el.loop = !!loop
      }
      return
    }

    clearAnswerFadeTimer()
    endingAnswer = false
    applyAnswerOverlay(src, !!loop, false)
  },
)

watch(answerLayerVisible, (visible) => {
  if (visible) {
    pauseAllStandbyVideos()
    return
  }
  void resumeStandbyPlayback()
})

watch(
  () => currentDisplayMode.value,
  (mode) => {
    if (mode === 'answer' && answerReady.value && mountedAnswerSrc.value) {
      answerShown.value = true
      return
    }
    if (mode === 'standby') {
      void resumeStandbyPlayback()
    }
  },
)

onMounted(async () => {
  await nextTick()
  syncContainerSize()
  syncActiveAspectRatio()

  const initialStandby = currentStandbySrc.value
  if (initialStandby && !standbySlotSrc.value[0] && !standbySlotSrc.value[1]) {
    void mountStandbyInitial(initialStandby)
  }

  if (!containerRef.value || typeof ResizeObserver === 'undefined') {
    return
  }

  containerResizeObserver = new ResizeObserver(() => {
    syncContainerSize()
  })
  containerResizeObserver.observe(containerRef.value)
})

onUnmounted(() => {
  clearAnswerFadeTimer()
  clearStandbyFadeTimer()
  bindVideoSizeListeners(null, videoRef.value)
  bindVideoSizeListeners(null, standbyVideoRefs.value[0])
  bindVideoSizeListeners(null, standbyVideoRefs.value[1])
  bindVideoSizeListeners(null, answerVideoRef.value)
  containerResizeObserver?.disconnect()
})

defineExpose({
  videoRef,
  advanceStandby,
  beginAnswerOverlay,
  syncAnswerOverlay,
  queueStandbySwitch,
  resumeStandbyPlayback,
  getStandbySrc: () => standbyPlaylist.value.currentSrc,
})
</script>

<template>
  <div ref="containerRef" class="video-container">
    <div class="video-stage" :style="stageStyle">
      <video
        ref="videoRef"
        autoplay
        playsinline
        class="video-element"
        :class="{ 'video-hidden': currentDisplayMode !== 'webrtc' }"
      />
      <video
        v-if="standbySlotSrc[0]"
        :ref="(el) => setStandbyVideoRef(0, el)"
        :src="standbySlotSrc[0]"
        muted
        autoplay
        :loop="shouldLoop"
        playsinline
        preload="auto"
        class="video-element video-element--standby"
        :class="{ 'video-hidden': !isStandbySlotVisible(0) }"
        :style="standbySlotStyle(0)"
        @canplay="onStandbyCanPlay"
        @loadeddata="onStandbyLoadedData"
        @ended="onStandbyEnded"
        @error="onStandbyError"
      />
      <video
        v-if="standbySlotSrc[1]"
        :ref="(el) => setStandbyVideoRef(1, el)"
        :src="standbySlotSrc[1]"
        muted
        autoplay
        :loop="shouldLoop"
        playsinline
        preload="auto"
        class="video-element video-element--standby"
        :class="{ 'video-hidden': !isStandbySlotVisible(1) }"
        :style="standbySlotStyle(1)"
        @canplay="onStandbyCanPlay"
        @loadeddata="onStandbyLoadedData"
        @ended="onStandbyEnded"
        @error="onStandbyError"
      />
      <img
        v-if="standbyImageSrc && !standbySlotSrc[0] && !standbySlotSrc[1]"
        ref="standbyImageRef"
        :src="standbyImageSrc"
        class="video-element video-element--standby"
        :class="{ 'video-hidden': !standbyLayerVisible }"
        @load="handleVideoIntrinsicSizeChange"
        @error="onStandbyError"
      />
      <video
        ref="answerVideoRef"
        :src="mountedAnswerSrc || undefined"
        :loop="answerLoopActive"
        playsinline
        webkit-playsinline
        preload="auto"
        class="video-element video-element--answer"
        :class="{ 'video-hidden': !answerLayerVisible || !mountedAnswerSrc }"
        @canplay="onAnswerCanPlay"
        @loadeddata="onAnswerCanPlay"
        @ended="onAnswerEnded"
        @error="onAnswerError"
      />
    </div>
  </div>
</template>

<style scoped>
.video-container {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #1a1a1a;
  border-radius: 8px;
  overflow: hidden;
}

.video-stage {
  position: relative;
  width: 100%;
  height: 100%;
  max-width: 100%;
  max-height: 100%;
  border-radius: inherit;
  overflow: hidden;
  background: #000;
}

.video-element {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
  object-position: center;
}

.video-element--answer {
  z-index: 2;
  transition: opacity 480ms ease;
  will-change: opacity;
}

.video-element--standby {
  z-index: 1;
  transition: opacity 480ms ease;
  will-change: opacity;
}

.video-hidden {
  opacity: 0;
  pointer-events: none;
}
</style>
