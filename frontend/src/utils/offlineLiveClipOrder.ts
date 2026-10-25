/** Preserve user click order when mapping selected video URLs to clip metadata. */

export type OfflineLiveClipRef = {
  id: string
  title: string
  url: string
}

export type ManagedVideoOptionLike = {
  id: string
  label: string
  url: string
}

export function mapSelectedUrlsToClips(
  selectedUrls: string[],
  options: ManagedVideoOptionLike[],
): OfflineLiveClipRef[] {
  if (selectedUrls.length === 0) return []
  const byUrl = new Map(options.map((item) => [item.url, item]))
  const clips: OfflineLiveClipRef[] = []
  for (const url of selectedUrls) {
    const option = byUrl.get(url)
    if (!option) continue
    clips.push({ id: option.id, title: option.label, url: option.url })
  }
  return clips
}

export function orderClipsByUrls(
  urls: string[],
  clips: OfflineLiveClipRef[],
): OfflineLiveClipRef[] {
  if (urls.length === 0) return clips
  const byUrl = new Map(clips.map((clip) => [clip.url, clip]))
  const ordered: OfflineLiveClipRef[] = []
  for (const url of urls) {
    const clip = byUrl.get(url)
    if (clip) ordered.push(clip)
  }
  for (const clip of clips) {
    if (!urls.includes(clip.url)) ordered.push(clip)
  }
  return ordered
}
