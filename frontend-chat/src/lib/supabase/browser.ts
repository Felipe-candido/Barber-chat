import { createClient, type SupabaseClient } from "@supabase/supabase-js";
import { supabaseBrowserConfig } from "./config";

let browserClient: SupabaseClient | undefined;

export function getSupabaseBrowserClient(): SupabaseClient {
  if (typeof window === "undefined") {
    throw new Error("O cliente Supabase de autenticação só pode ser usado no navegador.");
  }
  if (!browserClient) {
    const { url, publishableKey } = supabaseBrowserConfig();
    browserClient = createClient(url, publishableKey, {
      auth: {
        autoRefreshToken: true,
        persistSession: true,
        detectSessionInUrl: true,
        // Supabase admin invites arrive in another browser and do not support PKCE.
        // Restrict implicit callback handling to the dedicated invitation page.
        flowType: window.location.pathname === "/definir-senha" ? "implicit" : "pkce",
      },
    });
  }
  return browserClient;
}
