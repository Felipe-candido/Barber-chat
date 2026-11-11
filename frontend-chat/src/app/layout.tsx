import type { Metadata, Viewport } from "next";
import { connection } from "next/server";
import "@fontsource-variable/dm-sans";
import "@fontsource-variable/fraunces";
import "./globals.css";
import { FeedbackProvider } from "@/components/feedback-provider";
import { AdminAccessProvider } from "@/components/admin-access-provider";

export const metadata: Metadata = {
  title: {
    default: "Na Régua · Agenda e gestão de barbearias",
    template: "%s · Na Régua",
  },
  description: "Agendamento online e gestão da rotina da sua barbearia.",
  robots: { index: false, follow: false },
};
export const viewport: Viewport = {
  viewportFit: "cover",
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#f4f4f2" },
    { media: "(prefers-color-scheme: dark)", color: "#121415" },
  ],
};
export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  // Nonces must belong to this request, never to cached/prerendered HTML.
  await connection();
  return (
    <html lang="pt-BR">
      <body>
        <FeedbackProvider>
          <AdminAccessProvider>{children}</AdminAccessProvider>
        </FeedbackProvider>
      </body>
    </html>
  );
}
