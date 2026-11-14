// Cache only public, data-free assets. Private pages and API responses stay online-only.
const CACHE_PREFIX = "na-regua-offline-";
// Increment when changing offline assets so installed apps replace their old cache.
const CACHE_NAME = `${CACHE_PREFIX}v2`;
const OFFLINE_URL = "/offline.html";
const PUBLIC_ASSETS = [
  OFFLINE_URL,
  "/offline.css",
  "/icons/icon-192.png",
  "/icons/icon-512.png",
  "/icons/maskable-512.png",
  "/icons/apple-touch-icon.png",
];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then(async (cache) => {
      await cache.addAll(PUBLIC_ASSETS);
      await self.skipWaiting();
    }),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      const names = await caches.keys();
      await Promise.all(
        names
          .filter((name) => name.startsWith(CACHE_PREFIX) && name !== CACHE_NAME)
          .map((name) => caches.delete(name)),
      );
      await self.clients.claim();
    })(),
  );
});

self.addEventListener("fetch", (event) => {
  const { request } = event;
  const url = new URL(request.url);
  if (request.method !== "GET" || url.origin !== self.location.origin) return;

  // The offline document needs its stylesheet/icon even when the network is unavailable.
  if (PUBLIC_ASSETS.includes(url.pathname) && !url.search) {
    event.respondWith(
      caches.open(CACHE_NAME).then(async (cache) => (await cache.match(request)) || fetch(request)),
    );
    return;
  }

  // Never cache/replay requests, Next.js RSC payloads, credentials or administrative HTML.
  if (request.mode !== "navigate" || url.pathname.startsWith("/api/")) return;
  event.respondWith(
    fetch(request).catch(async () => {
      const cache = await caches.open(CACHE_NAME);
      return (await cache.match(OFFLINE_URL)) || Response.error();
    }),
  );
});
