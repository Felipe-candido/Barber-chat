import type { MetadataRoute } from "next";

const siteURL = (process.env.NEXT_PUBLIC_SITE_URL ?? "").trim().replace(/\/+$/, "");

// Only the landing page is meant for search engines. Panels and account pages
// already send noindex; this keeps crawlers from spending requests on them.
export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        disallow: ["/admin", "/login", "/cadastro", "/confirmar-email", "/definir-senha", "/chat"],
      },
    ],
    ...(siteURL ? { sitemap: `${siteURL}/sitemap.xml` } : {}),
  };
}
