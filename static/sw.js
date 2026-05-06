const CACHE_NAME = 'bloom-offline-v2';
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

// Install: pre-cache the app shell
self.addEventListener('install', (event) => {
    event.waitUntil(
        caches.open(CACHE_NAME)
            .then((cache) => cache.addAll(APP_SHELL))
            .then(() => self.skipWaiting())
    );
});

// Activate: purge old caches
self.addEventListener('activate', (event) => {
    event.waitUntil(
        caches.keys().then((keys) =>
            Promise.all(keys.map((key) => {
                if (key !== CACHE_NAME) return caches.delete(key);
            }))
        ).then(() => self.clients.claim())
    );
});

// Fetch: offline-first for navigation and static assets
self.addEventListener('fetch', (event) => {
    const req = event.request;

    // Only handle GET requests
    if (req.method !== 'GET') return;

    // Navigation requests (page loads)
    if (req.mode === 'navigate') {
        event.respondWith(
            // Try network first for fresh content
            fetch(req).then((response) => {
                const copy = response.clone();
                caches.open(CACHE_NAME).then((cache) => cache.put(req, copy));
                return response;
            }).catch(() => {
                // Offline: serve from cache, fallback to /local
                return caches.match(req)
                    .then((cached) => cached || caches.match('/local'))
                    .then((fallback) => fallback || new Response(
                        '<html><body><h1>Offline</h1><p>Open <a href="/local">/local</a> to use Bloom offline.</p></body></html>',
                        { headers: { 'Content-Type': 'text/html' } }
                    ));
            })
        );
        return;
    }

    // Static assets: cache-first with network fallback & background update
    event.respondWith(
        caches.match(req).then((cached) => {
            // Return cached immediately, update in background
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
