// Service worker: app shell en caché; NAVEGACIÓN network-first (la app es
// dinámica y autenticada: servir HTML viejo del caché rompe la experiencia);
// el cache de navegación solo responde cuando no hay red (modo offline).
const CACHE = 'uniapp-shell-v2';
const SHELL = ['/', '/manifest.webmanifest', '/icon.svg'];

self.addEventListener('install', (e) => {
	e.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)));
	self.skipWaiting();
});

self.addEventListener('activate', (e) => {
	e.waitUntil(
		caches.keys()
			.then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
			.then(() => self.clients.claim())
	);
});

self.addEventListener('fetch', (e) => {
	const url = new URL(e.request.url);
	if (url.origin !== location.origin) return;            // solo mismo origen
	if (e.request.method !== 'GET') return;                // escrituras: siempre red
	if (url.pathname.startsWith('/api/')) return;          // API: siempre red

	if (e.request.mode === 'navigate') {
		// páginas: red primero; si falla (offline), intenta el cache
		e.respondWith(
			fetch(e.request).catch(() => caches.match(e.request).then((hit) => hit || caches.match('/')))
		);
		return;
	}
	// estáticos (icono, manifest): cache-first
	e.respondWith(caches.match(e.request).then((hit) => hit || fetch(e.request)));
});
