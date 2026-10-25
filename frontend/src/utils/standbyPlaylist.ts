/** Round-robin standby playlist with deferred switch (finish current clip first). */

export type StandbyPlaylistState = {
  order: string[]
  playedInRound: string[]
  currentSrc: string
  /** URLs to play immediately after the current clip ends (switch / reorder). */
  pendingAfterCurrent: string[]
}

function dedupeUrls(urls: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const url of urls) {
    const trimmed = url.trim()
    if (!trimmed || seen.has(trimmed)) continue
    seen.add(trimmed)
    out.push(trimmed)
  }
  return out
}

export function createStandbyPlaylist(order: string[]): StandbyPlaylistState {
  const urls = dedupeUrls(order)
  return {
    order: urls,
    playedInRound: [],
    currentSrc: urls[0] ?? '',
    pendingAfterCurrent: [],
  }
}

function nextUnplayedInRotation(
  order: string[],
  played: Set<string>,
  afterSrc: string,
): string | null {
  if (order.length === 0) return null
  const start = Math.max(0, order.indexOf(afterSrc))
  for (let step = 1; step <= order.length; step += 1) {
    const candidate = order[(start + step) % order.length]
    if (!played.has(candidate)) return candidate
  }
  return null
}

function buildTailAfterSwitch(
  order: string[],
  currentSrc: string,
  played: Set<string>,
  jumpTarget: string,
): string[] {
  const tail: string[] = [jumpTarget]
  const scheduled = new Set<string>([currentSrc, jumpTarget])
  let cursor = jumpTarget
  for (let step = 0; step < order.length; step += 1) {
    const next = nextUnplayedInRotation(order, played, cursor)
    if (!next || scheduled.has(next)) break
    tail.push(next)
    scheduled.add(next)
    cursor = next
  }
  return tail
}

/** Next URL after the current clip (pending queue, then rotation) without mutating state. */
export function peekNextStandbySrc(state: StandbyPlaylistState): string {
  if (state.order.length === 0) return ''
  if (state.order.length === 1) return state.order[0]
  if (state.pendingAfterCurrent.length > 0) {
    return state.pendingAfterCurrent[0]
  }
  const played = new Set(state.playedInRound)
  if (state.currentSrc) {
    played.add(state.currentSrc)
  }
  const next = nextUnplayedInRotation(state.order, played, state.currentSrc)
  if (next) return next
  return state.order[0] ?? ''
}

/** Mark current clip done and advance (pending queue, then unplayed rotation, then new round). */
export function advanceStandbyPlaylist(state: StandbyPlaylistState): StandbyPlaylistState {
  if (state.order.length === 0) {
    return { ...state, currentSrc: '', pendingAfterCurrent: [] }
  }
  if (state.order.length === 1) {
    return { ...state, currentSrc: state.order[0], pendingAfterCurrent: [], playedInRound: [] }
  }

  const played = new Set(state.playedInRound)
  if (state.currentSrc) {
    played.add(state.currentSrc)
  }

  if (state.pendingAfterCurrent.length > 0) {
    const [next, ...rest] = state.pendingAfterCurrent
    return {
      ...state,
      playedInRound: [...played],
      currentSrc: next,
      pendingAfterCurrent: rest,
    }
  }

  const next = nextUnplayedInRotation(state.order, played, state.currentSrc)
  if (next) {
    return {
      ...state,
      playedInRound: [...played],
      currentSrc: next,
      pendingAfterCurrent: [],
    }
  }

  const first = state.order[0]
  return {
    order: state.order,
    playedInRound: [],
    currentSrc: first,
    pendingAfterCurrent: [],
  }
}

/** Queue `targetUrl` after the current clip, then remaining unplayed clips this round. */
export function requestStandbySwitch(
  state: StandbyPlaylistState,
  targetUrl: string,
): StandbyPlaylistState {
  const target = targetUrl.trim()
  if (!target || !state.order.includes(target)) return state
  if (state.order.length <= 1) {
    return { ...state, currentSrc: target, pendingAfterCurrent: [], playedInRound: [] }
  }
  if (target === state.currentSrc && state.pendingAfterCurrent.length === 0) {
    return state
  }

  const played = new Set(state.playedInRound)
  const tail = buildTailAfterSwitch(state.order, state.currentSrc, played, target)
  return {
    ...state,
    pendingAfterCurrent: tail,
  }
}

/** Update selectable order while keeping the playing clip when possible. */
export function syncStandbyPlaylistOrder(
  state: StandbyPlaylistState,
  nextOrder: string[],
): StandbyPlaylistState {
  const order = dedupeUrls(nextOrder)
  if (order.length === 0) {
    return createStandbyPlaylist([])
  }
  if (!state.currentSrc || !order.includes(state.currentSrc)) {
    return createStandbyPlaylist(order)
  }
  const pendingAfterCurrent = state.pendingAfterCurrent.filter((url) => order.includes(url))
  const playedInRound = state.playedInRound.filter((url) => order.includes(url))
  return {
    order,
    playedInRound,
    currentSrc: state.currentSrc,
    pendingAfterCurrent,
  }
}
