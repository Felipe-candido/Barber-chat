import type { Metadata } from "next";
import { ShopSettingsPage } from "@/components/settings/shop-settings-page";

export const metadata: Metadata = { title: "Configurações" };
export default function Page() {
  return <ShopSettingsPage />;
}
