import { AdminAccessGuard } from "@/components/admin-access-guard";
export default function Layout({ children }: { children: React.ReactNode }) {
  return <AdminAccessGuard>{children}</AdminAccessGuard>;
}
