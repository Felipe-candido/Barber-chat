import { ShopPanel } from "@/components/shop-panel";

export default async function Layout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return (
    <ShopPanel key={slug} slug={slug}>
      {children}
    </ShopPanel>
  );
}
