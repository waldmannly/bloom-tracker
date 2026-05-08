// ─── Bloom PWA Service Worker ─────────────────────────────────────────────
// Increment this version to trigger update flow
const SW_VERSION = '3.1.0';
const CACHE_NAME = 'bloom-pwa-v3.1';
const APP_SHELL = [
    '/',
    '/local',
    '/login',
    '/register',
    '/privacy',
    '/methodology',
    '/static/style.css',
    '/static/app.js',
    '/static/local-mode.js',
    '/manifest.webmanifest'
];

// Install: pre-cache app shell, do NOT skipWaiting automatically
// (let the client decide when to activate via message)
self.addEventListener('install', (event) => {
    event.waitUntil(
        caches.open(CACHE_NAME)
            .then((cache) => cache.addAll(APP_SHELL))
    );
});

// Activate: purge old caches, claim all clients
self.addEventListener('activate', (event) => {
    event.waitUntil(
        caches.keys().then((keys) =>
            Promise.all(keys.map((key) => {
                if (key !== CACHE_NAME) return caches.delete(key);
            }))
        ).then(() => self.clients.claim())
    );
});

// Message handler: allow client to trigger skipWaiting
self.addEventListener('message', (event) => {
    if (event.data && event.data.type === 'SKIP_WAITING') {
        self.skipWaiting();
    }
    if (event.data && event.data.type === 'GET_VERSION') {
        event.ports[0].postMessage({ version: SW_VERSION });
    }
});

// Fetch: network-first for navigation, stale-while-revalidate for assets
self.addEventListener('fetch', (event) => {
    const req = event.request;

    // Only handle GET requests
    if (req.method !== 'GET') return;

    // Skip cross-origin requests
    if (!req.url.startsWith(self.location.origin)) return;

    // Navigation requests (page loads)
    if (req.mode === 'navigate') {
        event.respondWith(
            fetch(req).then((response) => {
                const copy = response.clone();
                caches.open(CACHE_NAME).then((cache) => cache.put(req, copy));
                return response;
            }).catch(() => {
                return caches.match(req)
                    .then((cached) => cached || caches.match('/local'))
                    .then((fallback) => fallback || new Response(
                        '<!DOCTYPE html><html><body><h1>Offline</h1><p>Open <a href="/local">/local</a> for offline mode.</p></body></html>',
                        { headers: { 'Content-Type': 'text/html' } }
                    ));
            })
        );
        return;
    }

    // Static assets: stale-while-revalidate
    event.respondWith(
        caches.match(req).then((cached) => {
            const fetchPromise = fetch(req).then((response) => {
                if (response && response.status === 200 && response.type === 'basic') {
                    const copy = response.clone();
                    caches.open(CACHE_NAME).then((cache) => cache.put(req, copy));
                }
                return response;
            }).catch(() => cached);

            return cached || fetchPromise;
        })
    );
});
