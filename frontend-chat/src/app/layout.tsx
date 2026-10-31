import type { Metadata } from "next";
import "@fontsource-variable/dm-sans";
import "./globals.css";
import { FeedbackProvider } from "@/components/feedback-provider";
import { AdminAccessProvider } from "@/components/admin-access-provider";

export const metadata: Metadata = {
  title: {
    default: "Barber-chat · Agenda e gestão de barbearias",
    template: "%s · Barber-chat",
  },
  description: "Agendamento online e gestão da rotina da sua barbearia.",
  robots: { index: false, follow: false },
};
export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
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
