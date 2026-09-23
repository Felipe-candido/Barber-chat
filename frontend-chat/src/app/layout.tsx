import type { Metadata } from "next";
import "@fontsource-variable/dm-sans";
import "./globals.css";
import { DemoProvider } from "@/components/demo-provider";

export const metadata: Metadata = {
  title: {
    default: "Palma Barbearia · Seu estilo. Nosso cuidado.",
    template: "%s · Palma Barbearia",
  },
  description:
    "Agenda, serviços e um cuidado especial com o seu tempo. Palma Barbearia — demonstração de interface.",
  robots: { index: false, follow: false },
};
export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="pt-BR">
      <body>
        <DemoProvider>{children}</DemoProvider>
      </body>
    </html>
  );
}
