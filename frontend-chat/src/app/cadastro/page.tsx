import type { Metadata } from "next";
import { SignupPage } from "@/components/onboarding/signup-page";
export const metadata: Metadata = { title: "Criar conta" };
export default function Page() {
  return <SignupPage />;
}
