import { Skeleton } from "@/components/ui/skeleton";

export function MovieCardSkeleton() {
  return (
    <div>
      <Skeleton className="aspect-2/3 w-full rounded-lg" />
      <div className="mt-2 space-y-1.5">
        <Skeleton className="h-3.5 w-4/5 rounded" />
        <Skeleton className="h-3 w-1/4 rounded" />
      </div>
    </div>
  );
}

/** A full grid of skeleton cards, matching the real grid's breakpoints. */
export function MovieGridSkeleton({ count = 15 }: { count?: number }) {
  return (
    <div className="grid grid-cols-2 gap-x-4 gap-y-8 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
      {Array.from({ length: count }).map((_, i) => (
        <MovieCardSkeleton key={i} />
      ))}
    </div>
  );
}
