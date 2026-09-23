"use client";

import { Toaster as Sonner, type ToasterProps } from "sonner";

/**
 * shadcn-style Sonner toaster. The app is dark-only (`dark` is fixed on
 * <html>), so the theme is pinned instead of pulling in next-themes.
 */
export function Toaster(props: ToasterProps) {
  return (
    <Sonner
      theme="dark"
      className="toaster group"
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--border-radius": "var(--radius)",
        } as React.CSSProperties
      }
      {...props}
    />
  );
}
