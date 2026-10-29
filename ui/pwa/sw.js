/* The audio player lives in the page. This worker only caches the app and serves saved media. */
const SHELL = 'music-pwa-shell-v1'
const MEDIA = 'music-pwa-media-v1'
const ART = 'music-pwa-art-v1'
const ROOT = '/pwa/'

self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(SHELL).then((cache) => cache.addAll([
    ROOT, `${ROOT}manifest.webmanifest`, `${ROOT}icons/icon-192.png`, `${ROOT}icons/icon-512.png`,
  ])))
})

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

async function savedAudio(request) {
  const response = await (await caches.open(MEDIA)).match(new URL(request.url).pathname)
  if (!response) return new Response('Not downloaded', { status: 404 })
  const range = request.headers.get('Range')
  if (!range) return response
  const audio = await response.blob()
  const size = audio.size
  const match = /^bytes=(\d*)-(\d*)$/.exec(range)
  if (!match || (!match[1] && !match[2])) return new Response(null, { status: 416, headers: { 'Content-Range': `bytes */${size}` } })
  const start = match[1] ? Number(match[1]) : Math.max(0, size - Number(match[2]))
  const end = match[2] && match[1] ? Math.min(Number(match[2]), size - 1) : size - 1
  if (start >= size || start > end) return new Response(null, { status: 416, headers: { 'Content-Range': `bytes */${size}` } })
  return new Response(audio.slice(start, end + 1), {
    status: 206,
    headers: {
      'Content-Type': 'audio/mpeg', 'Accept-Ranges': 'bytes',
      'Content-Range': `bytes ${start}-${end}/${size}`, 'Content-Length': String(end - start + 1),
    },
  })
}

self.addEventListener('fetch', (event) => {
  const { request } = event
  if (request.method !== 'GET') return
  const url = new URL(request.url)
  if (url.origin !== self.location.origin) return

  if (url.pathname.startsWith(`${ROOT}offline-media/`)) {
    event.respondWith(savedAudio(request))
    return
  }
  if (url.pathname === '/rest/getCoverArt.view') {
    const username = url.searchParams.get('u')
    const cover = url.searchParams.get('id')
    if (username && cover) {
      event.respondWith((async () => {
        const cached = await (await caches.open(ART)).match(`${ROOT}offline-cover/${encodeURIComponent(username)}/${encodeURIComponent(cover)}`)
        return cached ?? fetch(request)
      })())
    }
    return
  }
  if (request.mode === 'navigate' && url.pathname.startsWith(ROOT)) {
    event.respondWith((async () => {
      try {
        const response = await fetch(request)
        if (response.ok) await (await caches.open(SHELL)).put(ROOT, response.clone())
        return response
      } catch {
        return (await (await caches.open(SHELL)).match(ROOT)) ?? Response.error()
      }
    })())
    return
  }
  if (url.pathname.startsWith(`${ROOT}assets/`) || url.pathname.startsWith(`${ROOT}fonts/`) || url.pathname.startsWith(`${ROOT}icons/`) || url.pathname === `${ROOT}manifest.webmanifest`) {
    event.respondWith((async () => {
      const cache = await caches.open(SHELL)
      const hit = await cache.match(request)
      if (hit) return hit
      const response = await fetch(request)
      if (response.ok) event.waitUntil(cache.put(request, response.clone()))
      return response
    })())
  }
})
