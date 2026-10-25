/** WebSocket origin for session chat. Uses same-origin on AutoDL / dev so /ws proxies to the API. */
export function resolveWebSocketBase(): string {
  const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const sameOrigin = `${wsProtocol}//${window.location.host}`
  const configured = (import.meta.env.VITE_WS_BASE as string | undefined)?.trim()
  if (!configured) {
    return sameOrigin
  }
  try {
    const pageHost = window.location.host
    const cfgHost = new URL(configured).host
    if (cfgHost === pageHost) {
      return configured.replace(/\/$/, '')
    }
  } catch {
    return sameOrigin
  }
  return sameOrigin
}
