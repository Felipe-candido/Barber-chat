import type { MetadataRoute } from "next";

const siteURL = (process.env.NEXT_PUBLIC_SITE_URL ?? "").trim().replace(/\/+$/, "");

// The landing page is the only indexable page. Without NEXT_PUBLIC_SITE_URL
// there is no canonical address to publish, so the sitemap stays empty.
export default function sitemap(): MetadataRoute.Sitemap {
  return siteURL ? [{ url: `${siteURL}/`, changeFrequency: "monthly", priority: 1 }] : [];
}
