import type { NextConfig } from "next";

const nextConfig: NextConfig = {
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
