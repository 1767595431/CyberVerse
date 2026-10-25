export type OfflineLiveVideoCategory = 'idle' | 'action' | 'sing' | 'unknown'

const IDLE_PREFIXES = ['待机-', 'idle-', 'Idle-']
const ACTION_PREFIXES = ['动作-', 'action-', 'Action-']
const SING_PREFIXES = ['唱歌-', 'sing-', 'Sing-']

export function categorizeOfflineLiveVideoTitle(title: string): OfflineLiveVideoCategory {
  const trimmed = title.trim()
  if (IDLE_PREFIXES.some(prefix => trimmed.startsWith(prefix))) {
    return 'idle'
  }
  if (ACTION_PREFIXES.some(prefix => trimmed.startsWith(prefix))) {
    return 'action'
  }
  if (SING_PREFIXES.some(prefix => trimmed.startsWith(prefix))) {
    return 'sing'
  }
  return 'unknown'
}

export function isOfflineLiveIdleVideoTitle(title: string): boolean {
  return categorizeOfflineLiveVideoTitle(title) === 'idle'
}

export function isOfflineLiveActionVideoTitle(title: string): boolean {
  return categorizeOfflineLiveVideoTitle(title) === 'action'
}

export function isOfflineLiveSingVideoTitle(title: string): boolean {
  return categorizeOfflineLiveVideoTitle(title) === 'sing'
}

/** Video title for sing-drive clips: 唱歌-{song name}. */
export function buildOfflineLiveSingVideoTitle(songName: string): string {
  const base = songName.trim()
  if (!base) return ''
  if (SING_PREFIXES.some(prefix => base.startsWith(prefix))) {
    return base
  }
  return `唱歌-${base}`
}

export function suggestSingSongNameFromFileName(fileName: string): string {
  return fileName.trim().replace(/\.[^.]+$/, '').trim()
}

export function offlineLiveSongDisplayTitle(title: string): string {
  const trimmed = title.trim()
  for (const prefix of SING_PREFIXES) {
    if (trimmed.startsWith(prefix)) {
      return trimmed.slice(prefix.length).trim() || trimmed
    }
  }
  return trimmed
}
