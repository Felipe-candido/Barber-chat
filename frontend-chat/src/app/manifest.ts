import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    id: "/",
    name: "Na Regua — Agenda e gestão",
    short_name: "Na Regua",
    description: "Agendamento online e gestão da rotina da sua barbearia.",
    lang: "pt-BR",
    start_url: "/admin",
    scope: "/",
    display: "standalone",
    background_color: "#f4f4f2",
    theme_color: "#f4f4f2",
    icons: [
      { src: "/icons/icon-192.png", sizes: "192x192", type: "image/png", purpose: "any" },
      { src: "/icons/icon-512.png", sizes: "512x512", type: "image/png", purpose: "any" },
      { src: "/icons/maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
  };
}
