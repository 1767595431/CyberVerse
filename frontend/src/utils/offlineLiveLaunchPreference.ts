/** Per-character offline live launch video picks (idle / action / sing). */

export type OfflineLiveLaunchPreference = {
  /** @deprecated use idleVideoJobIds */
  idleVideoJobId?: string | null
  idleVideoJobIds?: string[]
  actionVideoJobIds?: string[]
  singVideoJobIds?: string[]
}

const STORAGE_PREFIX = 'cyberverse.offlineLiveLaunch.v1:'

export function offlineLiveLaunchPreferenceKey(characterId: string): string {
  return `${STORAGE_PREFIX}${characterId}`
}

function normalizeIdleJobIds(parsed: OfflineLiveLaunchPreference): string[] {
  if (Array.isArray(parsed.idleVideoJobIds)) {
    return parsed.idleVideoJobIds.filter((id): id is string => typeof id === 'string')
  }
  if (typeof parsed.idleVideoJobId === 'string' && parsed.idleVideoJobId) {
    return [parsed.idleVideoJobId]
  }
  return []
}

export function loadOfflineLiveLaunchPreference(
  characterId: string,
): OfflineLiveLaunchPreference {
  if (typeof window === 'undefined' || !characterId) return {}
  try {
    const raw = window.localStorage.getItem(offlineLiveLaunchPreferenceKey(characterId))
    if (!raw) return {}
    const parsed = JSON.parse(raw) as OfflineLiveLaunchPreference
    return {
      idleVideoJobIds: normalizeIdleJobIds(parsed),
      actionVideoJobIds: Array.isArray(parsed.actionVideoJobIds)
        ? parsed.actionVideoJobIds.filter((id): id is string => typeof id === 'string')
        : [],
      singVideoJobIds: Array.isArray(parsed.singVideoJobIds)
        ? parsed.singVideoJobIds.filter((id): id is string => typeof id === 'string')
        : [],
    }
  } catch {
    return {}
  }
}

export function saveOfflineLiveLaunchPreference(
  characterId: string,
  preference: OfflineLiveLaunchPreference,
): void {
  if (typeof window === 'undefined' || !characterId) return
  const payload: OfflineLiveLaunchPreference = {
    idleVideoJobIds: [...(preference.idleVideoJobIds ?? [])],
    actionVideoJobIds: [...(preference.actionVideoJobIds ?? [])],
    singVideoJobIds: [...(preference.singVideoJobIds ?? [])],
  }
  window.localStorage.setItem(
    offlineLiveLaunchPreferenceKey(characterId),
    JSON.stringify(payload),
  )
}

/** Remove a deleted offline video job from saved launch picks. */
export function pruneOfflineLiveLaunchJob(characterId: string, deletedJobId: string): void {
  if (!characterId || !deletedJobId) return
  const pref = loadOfflineLiveLaunchPreference(characterId)
  let changed = false

  const nextIdle = (pref.idleVideoJobIds ?? []).filter((id) => {
    if (id === deletedJobId) {
      changed = true
      return false
    }
    return true
  })
  const nextActions = (pref.actionVideoJobIds ?? []).filter((id) => {
    if (id === deletedJobId) {
      changed = true
      return false
    }
    return true
  })
  const nextSings = (pref.singVideoJobIds ?? []).filter((id) => {
    if (id === deletedJobId) {
      changed = true
      return false
    }
    return true
  })

  if (!changed) return
  saveOfflineLiveLaunchPreference(characterId, {
    idleVideoJobIds: nextIdle,
    actionVideoJobIds: nextActions,
    singVideoJobIds: nextSings,
  })
}

export type OfflineLiveManagedVideoRef = {
  id: string
  url: string
}

export function buildOfflineLiveLaunchPreferenceFromUrls(
  options: OfflineLiveManagedVideoRef[],
  idleUrls: string[],
  actionUrls: string[],
  singUrls: string[],
): OfflineLiveLaunchPreference {
  const byUrl = new Map(options.map((item) => [item.url, item.id]))
  const idleVideoJobIds = idleUrls
    .map((url) => byUrl.get(url))
    .filter((id): id is string => !!id)
  return {
    idleVideoJobIds,
    actionVideoJobIds: actionUrls
      .map((url) => byUrl.get(url))
      .filter((id): id is string => !!id),
    singVideoJobIds: singUrls
      .map((url) => byUrl.get(url))
      .filter((id): id is string => !!id),
  }
}

export function resolveOfflineLiveLaunchVideoUrls(
  options: OfflineLiveManagedVideoRef[],
  preference: OfflineLiveLaunchPreference,
  isIdleLabel: (label: string) => boolean,
  isActionLabel: (label: string) => boolean,
  isSingLabel: (label: string) => boolean,
  labelById: Map<string, string>,
): {
  idleUrls: string[]
  actionUrls: string[]
  singUrls: string[]
  prunedPreference: OfflineLiveLaunchPreference
} {
  const byId = new Map(options.map((item) => [item.id, item.url]))

  const idleUrls = (preference.idleVideoJobIds ?? [])
    .filter((id) => {
      const label = labelById.get(id) ?? ''
      return byId.has(id) && isIdleLabel(label)
    })
    .map((id) => byId.get(id)!)

  const actionUrls = (preference.actionVideoJobIds ?? [])
    .filter((id) => {
      const label = labelById.get(id) ?? ''
      return byId.has(id) && isActionLabel(label)
    })
    .map((id) => byId.get(id)!)

  const singUrls = (preference.singVideoJobIds ?? [])
    .filter((id) => {
      const label = labelById.get(id) ?? ''
      return byId.has(id) && isSingLabel(label)
    })
    .map((id) => byId.get(id)!)

  const prunedPreference = buildOfflineLiveLaunchPreferenceFromUrls(
    options,
    idleUrls,
    actionUrls,
    singUrls,
  )

  return { idleUrls, actionUrls, singUrls, prunedPreference }
}
