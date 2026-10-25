import type { ComponentOption } from '../types'
import { CLOUD_AVATAR_BACKENDS, CLOUD_INFERENCE_PROVIDER_IDS, LOCAL_ONLY_DEPLOYMENT } from '../constants/localDeployment'
import type { AvatarBackend } from '../types'

export function isCloudAvatarBackend(backend?: string | null): boolean {
  return !!backend && CLOUD_AVATAR_BACKENDS.has(backend)
}

export function normalizeAvatarBackendForDeployment(backend?: string | null): AvatarBackend {
  if (LOCAL_ONLY_DEPLOYMENT || !backend || !CLOUD_AVATAR_BACKENDS.has(backend)) {
    return 'local_image'
  }
  return backend as AvatarBackend
}

export function filterLocalComponentOptions(items: ComponentOption[]): ComponentOption[] {
  if (!LOCAL_ONLY_DEPLOYMENT) return items
  return items.filter(item => !CLOUD_INFERENCE_PROVIDER_IDS.has(item.id))
}
