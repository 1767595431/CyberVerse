/** Checklist for offline-live overlay / standby playback changes (see VideoPlayer + useChat). */

export const OFFLINE_LIVE_PLAYBACK_RULES = {
  /** User clicks must call VideoPlayer.beginAnswerOverlay in the same synchronous handler before state updates. */
  userGestureBeforeState: true,
  /** WS / queue updates rely on :answer-src → VideoPlayer watch (muted bootstrap allowed). */
  wsUsesAnswerSrcPropWatch: true,
  /** Do not use props.answerLoop in VideoPlayer ended handler; use internal answerLoopActive. */
  loopStateInternal: true,
  /** Dedupe WS events when the same clip URL is already playing. */
  dedupeWsByUrl: true,
  /** Never clear overlay URL to empty between immediate clip swaps. */
  noEmptyBetweenImmediateSwaps: true,
  /** Same mounted URL + answerSrc prop must not re-invoke play() after beginAnswerOverlay. */
  dedupePropWatchByUrl: true,
} as const
