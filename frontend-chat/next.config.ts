import type { NextConfig } from "next";
const config: NextConfig = {
  poweredByHeader: false,
  distDir: process.env.BARBER_PLAYWRIGHT === "1" ? ".next-playwright" : ".next",
};
export default config;
