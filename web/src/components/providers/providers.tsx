"use client";

import { useEffect, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import dynamic from "next/dynamic";

import { NavTracker } from "@/components/providers/nav-tracker";
import { Toaster } from "@/components/ui/sonner";
import { ApiError } from "@/lib/api-client";
import { hydrateAuthStore } from "@/store/auth-store";
import { useLibrarySync } from "@/store/library-store";

// Loaded on first need only; every gated action opens this one instance.
const GlobalAuthDialog = dynamic(() => import("@/components/auth/auth-dialog").then((m) => m.GlobalAuthDialog), {
  ssr: false,
});

/** One retry, and only for failures that might succeed on a second try (the
 *  network, or a 5xx). A 4xx is the server's final answer: retrying a wrong
 *  password or a duplicate sign-up only doubles the attempt against the auth
 *  rate limit and delays the error, and TanStack pauses retries while the
 *  window is unfocused, which can leave the form spinning. */
function retryTransient(failureCount: number, error: unknown): boolean {
  if (failureCount >= 1) return false;
  return !(error instanceof ApiError && error.status >= 400 && error.status < 500);
}

export function Providers({ children }: { children: React.ReactNode }) {
  // One QueryClient per browser tab, created lazily so it survives Fast
  // Refresh but is never accidentally shared across requests on the server.
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            // Movie data doesn't change every second; avoid refetching every
            // tab focus. Individual queries (e.g. live interaction counts)
            // can override this with a shorter staleTime.
            staleTime: 30_000,
            retry: retryTransient,
          },
          mutations: {
            // Rating/commenting hits the outbox and returns 202 almost
            // instantly; one retry covers a dropped connection without
            // making the user wait on a slow multi-retry backoff.
            retry: retryTransient,
          },
        },
      }),
  );

  useEffect(() => {
    hydrateAuthStore();
  }, []);
  // Mirrors the signed-in user's list and ratings from the API.
  useLibrarySync();

  return (
    <QueryClientProvider client={queryClient}>
      {children}
      <NavTracker />
      <GlobalAuthDialog />
      {/* Bottom-center: clear of the top nav and of dialog headers. */}
      <Toaster position="bottom-center" />
    </QueryClientProvider>
  );
}
