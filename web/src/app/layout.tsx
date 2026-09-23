import type { Metadata } from "next";
import { Geist_Mono, Outfit } from "next/font/google";

import { Providers } from "@/components/providers/providers";
import { TopNav } from "@/components/layout/top-nav";
import "./globals.css";

// Variable font: every weight from one file, no `weight` list needed.
const outfit = Outfit({
  variable: "--font-outfit",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "MovieApp",
  description: "A cinematic, real-time movie discovery and watch-party app.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html
      lang="en"
      className={`dark ${outfit.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="flex min-h-full flex-col bg-background font-sans text-foreground">
        <Providers>
          <TopNav />
          <main className="flex flex-1 flex-col">{children}</main>
        </Providers>
      </body>
    </html>
  );
}
