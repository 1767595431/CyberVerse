/** Per-character offline live launch video picks (idle / action / sing). */

export type OfflineLiveLaunchPreference = {
  idleVideoJobId?: string | null
  actionVideoJobIds?: string[]
  singVideoJobIds?: string[]
}

const STORAGE_PREFIX = 'cyberverse.offlineLiveLaunch.v1:'

export function offlineLiveLaunchPreferenceKey(characterId: string): string {
  return `${STORAGE_PREFIX}${characterId}`
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
      idleVideoJobId: typeof parsed.idleVideoJobId === 'string' ? parsed.idleVideoJobId : null,
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
    idleVideoJobId: preference.idleVideoJobId ?? null,
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

  if (pref.idleVideoJobId === deletedJobId) {
    pref.idleVideoJobId = null
    changed = true
  }
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
    idleVideoJobId: pref.idleVideoJobId,
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
  const idleId = idleUrls.length === 1 ? byUrl.get(idleUrls[0]) : undefined
  return {
    idleVideoJobId: idleId ?? null,
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

  const idleUrls: string[] = []
  if (preference.idleVideoJobId && byId.has(preference.idleVideoJobId)) {
    const label = labelById.get(preference.idleVideoJobId) ?? ''
    if (isIdleLabel(label)) {
      idleUrls.push(byId.get(preference.idleVideoJobId)!)
    }
  }

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
