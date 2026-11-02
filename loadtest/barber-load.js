// Barber-chat load test (k6). Run through run-loadtest.ps1, or directly:
//   k6 run -e API=http://127.0.0.1:8080 -e SLUG=minha-barbearia loadtest/barber-load.js
//
// Scenarios (each one turns on only when its inputs exist):
//   public  - 100 customers browsing the public booking page (services, professionals, availability)
//   booking - customers confirming appointments at the same time (needs WRITE=1; local database only)
//   panel   - staff using the admin panel (needs TOKEN = a Supabase access token)
//   front   - Next.js pages (needs FRONT=http://127.0.0.1:3000)
import http from "k6/http";
import { check, group, sleep } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";
import { randomIntBetween, randomItem, uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";
import { textSummary } from "https://jslib.k6.io/k6-summary/0.1.0/index.js";

const API = (__ENV.API || "http://127.0.0.1:8080").replace(/\/$/, "");
const FRONT = (__ENV.FRONT || "").replace(/\/$/, "");
const SLUG = __ENV.SLUG || "";
const TOKEN = __ENV.TOKEN || "";
const WRITE = __ENV.WRITE === "1";
const USERS = Number(__ENV.USERS || 100);
const DURATION = __ENV.DURATION || "3m";

// Statuses the API returns on purpose. They are counted, not treated as transport failures.
const rateLimited = new Counter("rate_limited_429");
const slotTaken = new Counter("slot_conflict_409");
const unavailable = new Counter("unavailable_503");
const serverErrors = new Counter("server_errors_5xx");
const bookingCreated = new Counter("bookings_created");
const unexpected = new Rate("unexpected_status");
const availabilityTime = new Trend("availability_ms", true);
const bookingTime = new Trend("booking_ms", true);

http.setResponseCallback(http.expectedStatuses({ min: 200, max: 299 }, 409, 429));

function ramp(target) {
  return [
    { duration: "30s", target: Math.ceil(target / 4) },
    { duration: "30s", target },
    { duration: DURATION, target },
    { duration: "20s", target: 0 },
  ];
}

const scenarios = {};
if (SLUG) {
  scenarios.public = { executor: "ramping-vus", exec: "publicFlow", startVUs: 0, stages: ramp(USERS) };
}
if (SLUG && WRITE) {
  scenarios.booking = {
    executor: "ramping-vus",
    exec: "bookingFlow",
    startVUs: 0,
    stages: ramp(Math.max(5, Math.ceil(USERS / 5))),
  };
}
if (SLUG && TOKEN) {
  scenarios.panel = {
    executor: "ramping-vus",
    exec: "panelFlow",
    startVUs: 0,
    stages: ramp(Math.max(5, Math.ceil(USERS / 4))),
  };
}
if (SLUG && FRONT) {
  scenarios.front = {
    executor: "ramping-vus",
    exec: "frontFlow",
    startVUs: 0,
    stages: ramp(Math.max(5, Math.ceil(USERS / 2))),
  };
}

export const options = {
  scenarios,
  // Spread virtual users across loopback addresses so each one looks like a different customer
  // to the per-IP limiter. Set by run-loadtest.ps1 -SpreadIPs.
  localIPs: __ENV.LOCAL_IPS || undefined,
  thresholds: {
    "http_req_duration{scenario:public}": ["p(95)<500"],
    "http_req_duration{scenario:panel}": ["p(95)<800"],
    "http_req_duration{scenario:front}": ["p(95)<1500"],
    booking_ms: ["p(95)<1500"],
    server_errors_5xx: ["count<1"],
    unexpected_status: ["rate<0.01"],
  },
  summaryTrendStats: ["avg", "med", "p(90)", "p(95)", "p(99)", "max"],
};

function track(res, ok = [200, 201]) {
  if (res.status === 429) rateLimited.add(1);
  else if (res.status === 409) slotTaken.add(1);
  else if (res.status === 503) unavailable.add(1);
  if (res.status >= 500) serverErrors.add(1);
  unexpected.add(!ok.includes(res.status) && res.status !== 429 && res.status !== 409);
  return res;
}

function json(res) {
  try {
    return res.json();
  } catch {
    return null;
  }
}

function localDate(offsetDays) {
  const d = new Date(Date.now() + offsetDays * 86400000 - 3 * 3600000); // America/Sao_Paulo
  return d.toISOString().slice(0, 10);
}

function randomPhone() {
  return "+55119" + String(randomIntBetween(10000000, 99999999));
}

function randomKey() {
  return (uuidv4() + uuidv4()).replace(/-/g, "");
}

// Resolve real IDs once, before the load starts.
export function setup() {
  if (!SLUG) {
    throw new Error("SLUG is required: the public link of a shop with services and a professional (/b/<slug>).");
  }
  const res = http.get(`${API}/api/v1/public/shops/${SLUG}/services`);
  if (res.status !== 200) throw new Error(`Could not list services for ${SLUG}: HTTP ${res.status} ${res.body}`);
  const body = json(res);
  const services = (Array.isArray(body) ? body : body && (body.services || body.items)) || [];
  if (!services.length) throw new Error(`Shop ${SLUG} has no active services.`);
  const options = [];
  for (const service of services) {
    const pros = json(http.get(`${API}/api/v1/public/shops/${SLUG}/professionals?service_id=${service.id}`));
    for (const professional of (pros && pros.professionals) || []) {
      options.push({ serviceId: service.id, professionalId: professional.id });
    }
  }
  if (!options.length) throw new Error(`No professional offers the services of ${SLUG}.`);
  console.log(`Using ${services.length} service(s) and ${options.length} service/professional pair(s).`);
  return { options };
}

export function publicFlow(data) {
  const pick = randomItem(data.options);
  group("public booking page", () => {
    track(http.get(`${API}/api/v1/public/shops/${SLUG}/services`, { tags: { name: "GET services" } }));
    sleep(randomIntBetween(1, 3));
    track(
      http.get(`${API}/api/v1/public/shops/${SLUG}/professionals?service_id=${pick.serviceId}`, {
        tags: { name: "GET professionals" },
      }),
    );
    sleep(randomIntBetween(1, 3));
    // Customers usually check two or three days before choosing.
    for (let i = 0; i < randomIntBetween(1, 3); i++) {
      const res = track(
        http.get(
          `${API}/api/v1/public/shops/${SLUG}/availability?service_id=${pick.serviceId}&professional_id=${pick.professionalId}&date=${localDate(randomIntBetween(0, 7))}`,
          { tags: { name: "GET availability" } },
        ),
      );
      if (res.status === 200) availabilityTime.add(res.timings.duration);
      sleep(randomIntBetween(2, 5));
    }
  });
  sleep(randomIntBetween(3, 8));
}

export function bookingFlow(data) {
  const pick = randomItem(data.options);
  const date = localDate(randomIntBetween(1, 5));
  const avail = track(
    http.get(
      `${API}/api/v1/public/shops/${SLUG}/availability?service_id=${pick.serviceId}&professional_id=${pick.professionalId}&date=${date}`,
      { tags: { name: "GET availability (booking)" } },
    ),
  );
  const slots = (json(avail) && json(avail).slots) || [];
  if (!slots.length) {
    sleep(2);
    return;
  }
  sleep(randomIntBetween(1, 3));
  const slot = randomItem(slots.slice(0, 4)); // several customers aim at the same early slots
  const res = track(
    http.post(
      `${API}/api/v1/public/shops/${SLUG}/appointments`,
      JSON.stringify({
        service_id: pick.serviceId,
        professional_id: pick.professionalId,
        starts_at: slot.starts_at,
        customer: { first_name: "Carga", last_name: `Teste ${__VU}`, phone: randomPhone(), phone_country: "BR" },
      }),
      {
        headers: { "Content-Type": "application/json", "Idempotency-Key": randomKey() },
        tags: { name: "POST appointment" },
      },
    ),
    [201, 200],
  );
  bookingTime.add(res.timings.duration);
  if (res.status === 201) bookingCreated.add(1);
  check(res, { "booking accepted or slot taken": (r) => [200, 201, 409].includes(r.status) });
  sleep(randomIntBetween(5, 10));
}

export function panelFlow() {
  const headers = { Authorization: `Bearer ${TOKEN}` };
  const base = `${API}/api/v1/admin/shops/${SLUG}`;
  group("admin panel", () => {
    track(http.get(`${API}/api/v1/admin/shops`, { headers, tags: { name: "GET my shops" } }));
    track(http.get(`${base}/appointments`, { headers, tags: { name: "GET agenda" } }));
    sleep(randomIntBetween(2, 4));
    track(http.get(`${base}/services`, { headers, tags: { name: "GET services (admin)" } }));
    track(http.get(`${base}/products`, { headers, tags: { name: "GET products" } }));
    sleep(randomIntBetween(2, 4));
    track(http.get(`${base}/reports/activity`, { headers, tags: { name: "GET reports" } }), [200, 404]);
  });
  sleep(randomIntBetween(5, 10));
}

export function frontFlow() {
  track(http.get(`${FRONT}/b/${SLUG}`, { tags: { name: "PAGE /b/slug" } }));
  sleep(randomIntBetween(1, 3));
  track(http.get(`${FRONT}/login`, { tags: { name: "PAGE /login" } }));
  sleep(randomIntBetween(3, 6));
}

export function handleSummary(summary) {
  const m = summary.metrics;
  const value = (name, stat = "count") => (m[name] && m[name].values[stat]) || 0;
  const p95 = (name) => Math.round(value(name, "p(95)"));
  const lines = [
    "",
    "Barber-chat load test summary",
    "==============================",
    `Requests: ${value("http_reqs")} (${value("http_reqs", "rate").toFixed(1)}/s)`,
    `p95 overall: ${p95("http_req_duration")} ms | availability p95: ${p95("availability_ms")} ms | booking p95: ${p95("booking_ms")} ms`,
    `429 rate limited: ${value("rate_limited_429")} | 409 slot taken: ${value("slot_conflict_409")} | 503 unavailable: ${value("unavailable_503")} | 5xx: ${value("server_errors_5xx")}`,
    `Bookings created: ${value("bookings_created")}`,
    "",
  ];
  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  return {
    stdout: textSummary(summary, { indent: " ", enableColors: true }) + "\n" + lines.join("\n"),
    [`loadtest/results/summary-${stamp}.json`]: JSON.stringify(summary, null, 2),
    [`loadtest/results/summary-${stamp}.txt`]: lines.join("\n"),
  };
}
