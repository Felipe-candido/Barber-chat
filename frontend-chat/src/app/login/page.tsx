import type { Metadata } from "next";
import { LoginPage } from "@/components/login-page";

export const metadata: Metadata = { title: "Acesso administrativo" };

export default function Page() {
  return <LoginPage />;
}
