/**
 * Central helpers for offline-live overlay playback (see VideoPlayer + useChat).
 * Keep behavior in sync with AGENTS.local.md § Offline-live playback.
 */

/** True when prop watch should not remount or re-play (loop-only updates handled separately). */
export function shouldSkipAnswerSrcWatchUpdate(
  nextSrc: string,
  mountedSrc: string,
): boolean {
  return nextSrc.trim() === mountedSrc.trim()
}

/** Infer loop from overlay kind or song clip list when kind is briefly unset during queue handoff. */
export function offlineLiveOverlayShouldLoop(
  overlayKind: 'action' | 'sing' | 'answer' | null,
  overlaySrc: string,
  songClipUrls: readonly string[],
): boolean {
  if (overlayKind === 'sing') return true
  if (overlaySrc && songClipUrls.includes(overlaySrc)) return true
  return false
}
