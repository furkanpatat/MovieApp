import type { Metadata } from "next";
import { Geist_Mono, Outfit } from "next/font/google";

import { Providers } from "@/components/providers/providers";
import { SiteFooter } from "@/components/layout/site-footer";
import { TopNav } from "@/components/layout/top-nav";
import { Assistant } from "@/components/assistant/assistant";
import { HtmlLang } from "@/components/layout/language-toggle";
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

// KinoCut (movies) ⇄ KinoShow (series): "Kino" is the Nordic word for cinema.
export const metadata: Metadata = {
  title: "KinoCut",
  description: "KinoCut & KinoShow: discover movies and series, and watch them together in real time.",
};

// `modal` is the @modal parallel route: a movie or series opened from within
// the app renders there, over `children`, which stays mounted underneath.
export default function RootLayout({
  children,
  modal,
}: Readonly<{ children: React.ReactNode; modal: React.ReactNode }>) {
  return (
    <html
      lang="en"
      className={`dark ${outfit.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="flex min-h-full flex-col bg-background font-sans text-foreground">
        <Providers>
          <TopNav />
          <main className="flex flex-1 flex-col">{children}</main>
          <SiteFooter />
          {modal}
          <Assistant />
          <HtmlLang />
        </Providers>
      </body>
    </html>
  );
}
