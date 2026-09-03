import type { NextConfig } from "next";

// OILCHANGE_STATIC=1 → static export for oilchange serve (no Node on desktop).
// Default (next dev / next start) keeps the App Router API for cloud/dev.
const staticExport = process.env.OILCHANGE_STATIC === "1";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  ...(staticExport
    ? {
        output: "export" as const,
        images: { unoptimized: true },
        trailingSlash: true,
      }
    : {}),
};

export default nextConfig;
