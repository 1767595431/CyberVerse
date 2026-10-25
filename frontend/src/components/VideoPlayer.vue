<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'

const emit = defineEmits<{
  standbyFailed: []
  answerEnded: []
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
const standbyVideoRef = ref<HTMLVideoElement | null>(null)
const answerVideoRef = ref<HTMLVideoElement | null>(null)
const standbyImageRef = ref<HTMLImageElement | null>(null)
const containerRef = ref<HTMLDivElement | null>(null)
const currentIndex = ref(0)
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

const effectiveSources = computed(() => {
  if (props.standbySources?.length) return props.standbySources
  if (props.standbySrc) return [props.standbySrc]
  return []
})

const currentStandbySrc = computed(() => {
  const srcs = effectiveSources.value
  if (srcs.length === 0) return ''
  return srcs[currentIndex.value % srcs.length]
})

const shouldLoop = computed(() => effectiveSources.value.length <= 1)

// Standby stays visible under answer so crossfade never reveals black stage.
const standbyLayerVisible = computed(() =>
  currentDisplayMode.value === 'standby'
  || currentDisplayMode.value === 'answer'
  || !!mountedAnswerSrc.value,
)

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

function onStandbyEnded() {
  advanceStandby()
}

/** Advance to the next idle clip in the rotating standby set. */
function advanceStandby() {
  const srcs = effectiveSources.value
  if (srcs.length <= 1) return
  currentIndex.value = (currentIndex.value + 1) % srcs.length
}

function onStandbyError(ev: Event) {
  const el = ev.target
  if (el instanceof HTMLVideoElement || el instanceof HTMLImageElement) {
    emit('standbyFailed')
  }
}

async function resumeStandbyPlayback() {
  await nextTick()
  const el = standbyVideoRef.value
  if (!el || !standbyLayerVisible.value) return
  try {
    if (el.paused) {
      await el.play()
    }
  } catch {
    /* autoplay / visibility restrictions */
  }
}

function finishAnswerPlayback() {
  if (endingAnswer) return
  endingAnswer = true
  answerShown.value = false
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
  if (props.answerLoop) {
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
  if (currentDisplayMode.value === 'answer' && mountedAnswerSrc.value) {
    answerShown.value = true
  }
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
        ? (standbyVideoRef.value || standbyImageRef.value)
        : videoRef.value

  const fallback = standbyVideoRef.value || answerVideoRef.value || videoRef.value
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

watch(effectiveSources, () => {
  currentIndex.value = 0
})

watch(videoRef, (nextEl, prevEl) => {
  bindVideoSizeListeners(nextEl, prevEl)
})

watch(standbyVideoRef, (nextEl, prevEl) => {
  bindVideoSizeListeners(nextEl, prevEl)
})

watch(answerVideoRef, (nextEl, prevEl) => {
  bindVideoSizeListeners(nextEl, prevEl)
})

watch(
  () => [standbyLayerVisible.value, currentStandbySrc.value] as const,
  async ([visible, src]) => {
    syncActiveAspectRatio()
    if (!visible || !src) return
    await nextTick()
    const el = standbyVideoRef.value
    if (!el) return
    try {
      await el.play()
    } catch {
      /* ignore autoplay restrictions */
    }
  },
)

watch(
  () => props.answerSrc,
  async (src, prev) => {
    clearAnswerFadeTimer()
    endingAnswer = false

    if (!src) {
      // Parent cleared while we may still be fading; if already idle, noop.
      if (!mountedAnswerSrc.value) return
      if (answerShown.value) {
        finishAnswerPlayback()
      } else {
        answerReady.value = false
        mountedAnswerSrc.value = ''
      }
      return
    }

    if (src === prev && src === mountedAnswerSrc.value) {
      return
    }

    // New answer clip: mount under opacity 0 until canplay, then fade in.
    answerShown.value = false
    answerReady.value = false
    mountedAnswerSrc.value = src
    await nextTick()
    const el = answerVideoRef.value
    if (!el) return
    try {
      el.currentTime = 0
      await el.play()
      if (el.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
        onAnswerCanPlay()
      }
    } catch {
      /* wait for canplay event */
    }
  },
)

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
  bindVideoSizeListeners(null, videoRef.value)
  bindVideoSizeListeners(null, standbyVideoRef.value)
  bindVideoSizeListeners(null, answerVideoRef.value)
  containerResizeObserver?.disconnect()
})

defineExpose({ videoRef, advanceStandby, resumeStandbyPlayback })
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
        v-if="currentStandbySrc"
        :key="currentStandbySrc"
        ref="standbyVideoRef"
        :src="currentStandbySrc"
        muted
        autoplay
        :loop="shouldLoop"
        playsinline
        preload="auto"
        class="video-element video-element--standby"
        :class="{ 'video-hidden': !standbyLayerVisible }"
        @ended="onStandbyEnded"
        @error="onStandbyError"
      />
      <img
        v-else-if="standbyImageSrc"
        ref="standbyImageRef"
        :src="standbyImageSrc"
        class="video-element video-element--standby"
        :class="{ 'video-hidden': !standbyLayerVisible }"
        @load="handleVideoIntrinsicSizeChange"
        @error="onStandbyError"
      />
      <video
        v-if="mountedAnswerSrc"
        ref="answerVideoRef"
        :src="mountedAnswerSrc"
        :loop="!!answerLoop"
        playsinline
        preload="auto"
        class="video-element video-element--answer"
        :class="{ 'video-hidden': !answerLayerVisible }"
        @canplay="onAnswerCanPlay"
        @loadeddata="onAnswerCanPlay"
        @ended="onAnswerEnded"
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
  transition: opacity 480ms ease;
  will-change: opacity;
}

.video-element--answer {
  z-index: 2;
}

.video-element--standby {
  z-index: 1;
}

.video-hidden {
  opacity: 0;
  pointer-events: none;
}
</style>
