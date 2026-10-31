import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./tests",
  fullyParallel: true,
  workers: 2,
  timeout: 60000,
  expect: { timeout: 15000 },
  use: {
    baseURL: "http://127.0.0.1:3100",
    channel: "msedge",
    viewport: { width: 1440, height: 1000 },
    timezoneId: "America/Sao_Paulo",
    locale: "pt-BR",
    trace: "retain-on-failure",
  },
  webServer: {
    // Opt in only after a successful test build; normal runs always compile first.
    command:
      (process.env.BARBER_PLAYWRIGHT_PREBUILT === "1" ? "" : "npm run build && ") +
      "node node_modules/next/dist/bin/next start --hostname 127.0.0.1 --port 3100",
    url: "http://127.0.0.1:3100",
    reuseExistingServer: false,
    env: {
      BARBER_PLAYWRIGHT: "1",
      NEXT_PUBLIC_API_BASE_URL: "http://127.0.0.1:8080",
      NEXT_PUBLIC_SUPABASE_URL: "https://test-project.supabase.co",
      NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY: "sb_publishable_test_key",
    },
    // Allow production compilation on slower local machines; test assertions keep their limits.
    timeout: 600000,
  },
  reporter: [["list"]],
});
