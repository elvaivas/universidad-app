// Service worker: estáticos en caché; NAVEGACIÓN network-first (la app es
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
	if (url.origin !== location.origin) return;
	if (e.request.method !== 'GET') return;
	if (url.pathname.startsWith('/api/')) return;

	if (e.request.mode === 'navigate') {
		e.respondWith(
			fetch(e.request).catch(() => caches.match(e.request).then((hit) => hit || caches.match('/')))
		);
		return;
	}
	e.respondWith(caches.match(e.request).then((hit) => hit || fetch(e.request)));
});
