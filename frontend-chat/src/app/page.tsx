import type { Metadata } from "next";
import { LandingPage } from "@/components/landing/landing-page";
import { PRODUCT_NAME, channelFrom } from "@/components/landing/landing-config";

const description =
  "Agenda online para barbearias: o cliente agenda pelo link, recebe confirmação e lembrete no WhatsApp, e você acompanha agenda, produtos e faturamento no celular.";

const siteURL = (process.env.NEXT_PUBLIC_SITE_URL ?? "").trim();

export const metadata: Metadata = {
  ...(siteURL ? { metadataBase: new URL(siteURL) } : {}),
  title: { absolute: `${PRODUCT_NAME} · Agenda online para barbearias` },
  description,
  // The rest of the app stays out of search results; the public home page is the exception.
  robots: { index: true, follow: true },
  openGraph: {
    type: "website",
    locale: "pt_BR",
    siteName: PRODUCT_NAME,
    title: `${PRODUCT_NAME} · Sua barbearia na régua`,
    description,
    ...(siteURL
      ? { images: [{ url: "/landing/d-agenda-semana.webp", width: 1600, height: 1000 }] }
      : {}),
  },
};

export default async function Home({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const params = await searchParams;
  return <LandingPage channel={channelFrom(params.origem ?? params.utm_source)} />;
}
