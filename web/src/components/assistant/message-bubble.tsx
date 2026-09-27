import { Aperture, RotateCcw } from "lucide-react";

import { ChatMarkdown } from "@/components/assistant/chat-markdown";
import { MovieCardMini } from "@/components/movies/movie-card-mini";
import { useT } from "@/i18n";
import { cn } from "@/lib/utils";
import { useAssistantStore } from "@/store/assistant-store";
import type { ChatTurn } from "@/types/assistant";
import type { Movie } from "@/types/movie";

/** A horizontal strip of mini cards inside a bubble. */
function MovieStrip({ movies }: { movies: Movie[] }) {
  const setOpen = useAssistantStore((s) => s.setOpen);
  // The movie opens in a modal the panel would cover: step aside.
  const onNavigate = () => setOpen(false);
  return (
    <div className="hide-scrollbar -mx-3 mt-3 flex snap-x snap-mandatory gap-3 overflow-x-auto px-3 pb-1">
      {movies.map((m) => (
        <MovieCardMini key={m.id} movie={m} onNavigate={onNavigate} />
      ))}
    </div>
  );
}

/** One chat message: its text, then any rich blocks (movie cards today). */
export function MessageBubble({ turn, onRetry }: { turn: ChatTurn; onRetry?: () => void }) {
  const { t } = useT();
  const mine = turn.role === "user";
  if (mine) {
    return (
      <div className="flex justify-end">
        <p className="max-w-[85%] rounded-2xl rounded-br-sm bg-zinc-800 px-3.5 py-2 text-sm font-medium tracking-tight whitespace-pre-wrap break-words text-white shadow-lg shadow-black/30">
          {turn.content}
        </p>
      </div>
    );
  }
  return (
    <div className="flex gap-2.5">
      <ConciergeMark size="sm" />
      {/* Borderless glass: the panel's blur shows through. */}
      <div
        className={cn(
          "min-w-0 flex-1 rounded-2xl rounded-tl-sm bg-white/[0.04] px-3.5 py-2.5 font-sans text-[0.9rem] leading-relaxed text-zinc-200",
          turn.failed && "bg-red-500/10 text-red-200",
        )}
      >
        {turn.failed ? <p className="whitespace-pre-wrap break-words">{turn.content}</p> : <ChatMarkdown>{turn.content}</ChatMarkdown>}
        {turn.movies && turn.movies.length > 0 && <MovieStrip movies={turn.movies} />}
        {turn.failed && onRetry && (
          <button
            type="button"
            onClick={onRetry}
            className="mt-2 inline-flex items-center gap-1 text-xs font-semibold text-red-100 underline-offset-4 hover:underline"
          >
            <RotateCcw className="size-3" />
            {t("common.tryAgain")}
          </button>
        )}
      </div>
    </div>
  );
}

const markSizes = {
  sm: { box: "size-8", icon: "size-4" },
  md: { box: "size-10", icon: "size-5" },
  lg: { box: "size-20", icon: "size-10" },
};

/**
 * The concierge's mark: a lens aperture on a gold-edged dark disc. The
 * aperture turns slowly and the disc "breathes" a soft glow. Decorative.
 */
export function ConciergeMark({ size = "md", className }: { size?: keyof typeof markSizes; className?: string }) {
  const s = markSizes[size];
  return (
    <span
      aria-hidden
      className={cn(
        "concierge-breathe relative flex shrink-0 items-center justify-center rounded-full bg-[radial-gradient(circle_at_30%_25%,#3f3f46,#09090b_70%)]",
        s.box,
        className,
      )}
    >
      {/* Gold rim: a conic gradient masked to a thin ring. */}
      <span className="absolute inset-0 rounded-full bg-[conic-gradient(from_200deg,#fbbf24,#b45309,#fde68a,#fbbf24)] p-px [mask:linear-gradient(#000_0_0)_content-box_exclude,linear-gradient(#000_0_0)]" />
      <Aperture className={cn("concierge-spin text-primary drop-shadow-[0_0_6px_rgb(251_191_36/0.7)]", s.icon)} strokeWidth={1.75} />
    </span>
  );
}

/** Three softly glowing dots, swelling in turn, while the concierge answers. */
export function TypingBubble() {
  const { t } = useT();
  return (
    <div className="flex items-center gap-2.5" role="status" aria-label={t("assistant.typing")}>
      <ConciergeMark size="sm" />
      <div className="flex items-center gap-1.5 rounded-2xl rounded-tl-sm bg-white/[0.04] px-4 py-3.5">
        {[0, 180, 360].map((d) => (
          <span key={d} className="concierge-dot size-1.5 rounded-full bg-primary" style={{ animationDelay: `${d}ms` }} />
        ))}
      </div>
    </div>
  );
}
