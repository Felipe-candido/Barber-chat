import type { Metadata } from "next";
import { SetPasswordPage } from "@/components/set-password-page";

export const metadata: Metadata = {
  title: "Definir senha",
  referrer: "no-referrer",
  robots: { index: false, follow: false },
};
export default function Page() {
  return <SetPasswordPage />;
}
