import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Dev only: lets phones and other computers on the local network load the
  // dev server's scripts (opened as http://192.168.x.x:3000). The gateway
  // must allow that origin too: CORS_ALLOWED_ORIGINS in the root .env.
  allowedDevOrigins: ["192.168.*.*", "10.*.*.*", "*.local"],
  images: {
    // Movie posters/backdrops are served from TMDB's CDN by the Catalog
    // service (see services/catalog); allow-listed here so next/image can
    // optimize them once the catalog UI lands.
    remotePatterns: [
      {
        protocol: "https",
        hostname: "image.tmdb.org",
        pathname: "/t/p/**",
      },
    ],
  },
};

export default nextConfig;
