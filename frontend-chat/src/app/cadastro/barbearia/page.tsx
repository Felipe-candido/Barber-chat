import type { Metadata } from "next";
import { ShopRegistrationPage } from "@/components/onboarding/shop-registration-page";
export const metadata: Metadata = { title: "Cadastrar barbearia", referrer: "no-referrer" };
export default function Page() {
  return <ShopRegistrationPage />;
}
