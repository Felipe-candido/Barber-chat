import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./tests",
  fullyParallel: true,
  workers: 2,
  timeout: 30000,
  use: {
    baseURL: "http://127.0.0.1:3100",
    channel: "msedge",
    viewport: { width: 1440, height: 1000 },
    timezoneId: "America/Sao_Paulo",
    locale: "pt-BR",
    trace: "retain-on-failure",
  },
  webServer: {
    command: "node node_modules/next/dist/bin/next dev --webpack --hostname 127.0.0.1 --port 3100",
    url: "http://127.0.0.1:3100",
    reuseExistingServer: false,
    env: {
      NEXT_PUBLIC_API_BASE_URL: "http://127.0.0.1:8080",
      NEXT_PUBLIC_SHOP_SLUG: "barbearia-do-felipe",
      NEXT_PUBLIC_SUPABASE_URL: "https://test-project.supabase.co",
      NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY: "sb_publishable_test_key",
    },
    timeout: 60000,
  },
  reporter: [["list"]],
});
