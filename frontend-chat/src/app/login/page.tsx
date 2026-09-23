import type { Metadata } from "next";
import { LoginPage } from "@/components/login-page";
import "./login.css";

export const metadata: Metadata = { title: "Acesso administrativo" };

export default function Page() {
  return <LoginPage />;
}
