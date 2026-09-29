"use client";

import { useEffect, useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { AnimatePresence, motion } from "framer-motion";
import { ArrowUp, RotateCcw, X } from "lucide-react";
import { useMutation } from "@tanstack/react-query";

import { ConciergeMark, MessageBubble, TypingBubble } from "@/components/assistant/message-bubble";
import { Button } from "@/components/ui/button";
import { ApiError, apiFetch } from "@/lib/api-client";
import { useT } from "@/i18n";
import type { Dictionary } from "@/i18n/dictionaries/en";
import { cn } from "@/lib/utils";
import { useAssistantStore } from "@/store/assistant-store";
import { useAuthPrompt } from "@/store/auth-prompt-store";
import { useAuthStore } from "@/store/auth-store";
import { useTitleModalStore } from "@/store/modal-store";
import type { ChatRequest, ChatResponse, ChatTurn } from "@/types/assistant";

const MAX_HISTORY = 20; // the API's limit; older turns are dropped
const MAX_CHARS = 2000;
type Suggestion = keyof Dictionary["assistant"]["suggestions"];
const SUGGESTIONS: Suggestion[] = ["funny", "scifi", "scary", "action"];
// On a movie or person page the assistant knows what's on screen.
function suggestionsFor(pathname: string) {
  if (/^\/movies\/\d+$/.test(pathname)) return ["likeThis", "worthIt", ...SUGGESTIONS.slice(0, 2)] satisfies Suggestion[];
  if (/^\/person\/\d+$/.test(pathname)) return ["theirBest", "whereStart", ...SUGGESTIONS.slice(0, 2)] satisfies Suggestion[];
  return SUGGESTIONS;
}

const newId = () => (typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`);

/**
 * The AI movie assistant: a floating button (bottom-right) that opens a
 * glass side panel with the chat. Mounted once in the root layout, so the
 * conversation survives navigation (e.g. opening a recommended movie).
 */
export function Assistant() {
  const pathname = usePathname();
  const { t } = useT();
  const open = useAssistantStore((s) => s.open);
  const toggle = useAssistantStore((s) => s.toggle);
  const reset = useAssistantStore((s) => s.reset);
  const userId = useAuthStore((s) => s.userId);

  // A different (or no) user must never see the previous one's chat.
  const [chatOwner, setChatOwner] = useState(userId);
  if (userId !== chatOwner) {
    setChatOwner(userId);
    reset();
  }

  // The Discover feed has its own action rail in that corner; a title modal
  // covers the page.
  const onDiscover = pathname === "/discover" || pathname.startsWith("/discover/");
  const modalOpen = useTitleModalStore((s) => s.open);

  return (
    <>
      <AnimatePresence>
        {!open && !onDiscover && !modalOpen && (
          <motion.button
            type="button"
            onClick={toggle}
            aria-label={t("assistant.open")}
            initial={{ scale: 0.6, opacity: 0 }}
            animate={{ scale: 1, opacity: 1 }}
            exit={{ scale: 0.6, opacity: 0 }}
            whileHover={{ scale: 1.06 }}
            whileTap={{ scale: 0.94 }}
            transition={{ type: "spring", stiffness: 400, damping: 22 }}
            className="group fixed right-5 bottom-5 z-[60] flex h-14 items-center gap-3 rounded-full bg-zinc-950/80 p-2 font-semibold text-white shadow-2xl shadow-black/60 ring-1 ring-white/10 backdrop-blur-xl outline-none transition-colors hover:ring-primary/40 focus-visible:ring-2 focus-visible:ring-primary sm:pr-5"
          >
            <ConciergeMark />
            <span className="hidden text-sm tracking-tight sm:inline">{t("assistant.fab")}</span>
          </motion.button>
        )}
      </AnimatePresence>

      <AnimatePresence>{open && <AssistantPanel />}</AnimatePresence>
    </>
  );
}

function AssistantPanel() {
  const pathname = usePathname();
  const { t, locale } = useT();
  const setOpen = useAssistantStore((s) => s.setOpen);
  const demo = useAssistantStore((s) => s.demo);
  const setDemo = useAssistantStore((s) => s.setDemo);
  const turns = useAssistantStore((s) => s.turns);
  const add = useAssistantStore((s) => s.add);
  const remove = useAssistantStore((s) => s.remove);
  const reset = useAssistantStore((s) => s.reset);
  const isAuthed = useAuthStore((s) => s.hasHydrated && s.username !== null);
  const openAuth = useAuthPrompt((s) => s.openAuth);

  const [draft, setDraft] = useState("");
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  const chat = useMutation({
    mutationFn: (body: ChatRequest) => apiFetch<ChatResponse>("/api/v1/chat", { method: "POST", body }),
    retry: false, // a retry is the user's call (the button on a failed turn)
  });

  // `before` is the conversation to continue (all of it, unless retrying).
  const send = (text: string, before: ChatTurn[] = turns) => {
    const content = text.trim();
    if (!content || chat.isPending) return;
    const history = [...before.filter((t) => !t.failed), { id: newId(), role: "user" as const, content }];
    add(history[history.length - 1]);
    setDraft("");
    const conversation = useAssistantStore.getState().conversation;
    const current = () => useAssistantStore.getState().conversation === conversation;
    chat.mutate(
      {
        messages: history.slice(-MAX_HISTORY).map(({ role, content }) => ({ role, content })),
        // The concierge answers in the UI language.
        locale,
        // Where the user is, so "movies like this one" means something.
        context: { path: window.location.pathname + window.location.search },
      },
      {
        onSuccess: (res) => {
          if (!current()) return;
          setDemo(Boolean(res.demo));
          add({ id: newId(), role: "assistant", content: res.message, movies: res.movies });
        },
        onError: (err) => {
          if (!current() || (err instanceof ApiError && err.status === 401)) return; // stale, or the session ended (api-client said so)
          add({
            id: newId(),
            role: "assistant",
            failed: true,
            content: t(err instanceof ApiError && err.status === 429 ? "assistant.tooFast" : "assistant.failed"),
          });
        },
      },
    );
  };

  // Retry = drop the failed reply and the question it answered, ask again.
  const retry = (failed: ChatTurn) => {
    const i = turns.findIndex((t) => t.id === failed.id);
    const question = turns[i - 1];
    remove(failed.id);
    if (question?.role === "user") {
      remove(question.id);
      send(question.content, turns.slice(0, i - 1));
    }
  };

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [turns.length, chat.isPending]);

  useEffect(() => {
    inputRef.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [setOpen]);

  return (
    <motion.aside
      aria-label={t("assistant.panel")}
      initial={{ x: "100%", opacity: 0.6 }}
      animate={{ x: 0, opacity: 1 }}
      exit={{ x: "100%", opacity: 0.6 }}
      transition={{ type: "spring", stiffness: 320, damping: 34 }}
      className="fixed top-0 right-0 z-[60] flex h-dvh w-full flex-col border-l border-white/10 bg-zinc-950/90 shadow-2xl shadow-black/60 backdrop-blur-3xl sm:w-96"
    >
      {/* The header glows like a lit screen: an aurora (as in the auth dialog), fading
          into the panel, with a gold light-leak along its bottom edge. */}
      <header className="relative flex items-center gap-3 overflow-hidden px-4 py-4">
        <div className="concierge-aurora absolute -inset-4 blur-xl" aria-hidden />
        <div className="absolute inset-0 bg-gradient-to-b from-transparent to-zinc-950/50" aria-hidden />
        <div className="absolute inset-x-0 bottom-0 h-px bg-gradient-to-r from-transparent via-primary/60 to-transparent" aria-hidden />
        <ConciergeMark className="relative" />
        <div className="relative min-w-0 flex-1">
          <p className="text-[10px] font-semibold tracking-[0.25em] text-primary/90 uppercase">{t("assistant.eyebrow")}</p>
          <h2 className="text-base font-bold tracking-tight text-white">{t("assistant.title")}</h2>
          <p className="mt-0.5 flex items-center gap-1.5 text-[11px] text-zinc-400">
            <span className={cn("size-1.5 rounded-full", demo ? "bg-zinc-500" : "bg-emerald-400 shadow-[0_0_6px_rgb(52_211_153/0.8)]")} />
            {t(demo ? "assistant.demo" : "assistant.online")}
          </p>
        </div>
        {turns.length > 0 && (
          <Button variant="ghost" size="icon" className="relative" aria-label={t("assistant.newConversation")} title={t("assistant.newConversation")} onClick={reset}>
            <RotateCcw className="size-4" />
          </Button>
        )}
        <Button variant="ghost" size="icon" className="relative" aria-label={t("assistant.closePanel")} onClick={() => setOpen(false)}>
          <X className="size-5" />
        </Button>
      </header>

      <div ref={scrollRef} className="flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-5" aria-live="polite">
        {turns.length === 0 ? (
          <Welcome
            signedIn={isAuthed}
            suggestions={suggestionsFor(pathname).map((s) => t(`assistant.suggestions.${s}`))}
            onPick={(s) => send(s)}
            onSignIn={() => {
              setOpen(false); // the sign-in dialog sits below this panel
              openAuth();
            }}
          />
        ) : (
          turns.map((t) => <MessageBubble key={t.id} turn={t} onRetry={t.failed ? () => retry(t) : undefined} />)
        )}
        {chat.isPending && <TypingBubble />}
      </div>

      {isAuthed && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            send(draft);
          }}
          className="border-t border-white/10 p-3"
        >
          <div className="flex items-end gap-2 rounded-2xl border border-white/10 bg-white/5 p-2 transition-colors focus-within:border-primary/60">
            <textarea
              ref={inputRef}
              value={draft}
              onChange={(e) => setDraft(e.target.value.slice(0, MAX_CHARS))}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                  e.preventDefault();
                  send(draft);
                }
              }}
              rows={1}
              placeholder={t("assistant.placeholder")}
              aria-label={t("assistant.inputLabel")}
              className="field-sizing-content max-h-32 min-h-9 flex-1 resize-none bg-transparent px-2 py-1.5 text-sm outline-none placeholder:text-muted-foreground"
            />
            <Button type="submit" size="icon" className="size-9 shrink-0 rounded-xl" disabled={!draft.trim() || chat.isPending} aria-label={t("assistant.send")}>
              <ArrowUp className="size-4" />
            </Button>
          </div>
          <p className="mt-1.5 px-1 text-[11px] text-muted-foreground">{t("assistant.hint")}</p>
        </form>
      )}
    </motion.aside>
  );
}

function Welcome({
  signedIn,
  suggestions,
  onPick,
  onSignIn,
}: {
  signedIn: boolean;
  suggestions: string[];
  onPick: (s: string) => void;
  onSignIn: () => void;
}) {
  const { t } = useT();
  return (
    <div className="flex flex-col items-center px-2 pt-8 text-center">
      <ConciergeMark size="lg" />
      <h3 className="mt-6 text-xl font-bold tracking-tight">{t("assistant.welcomeTitle")}</h3>
      <p className="mt-1 text-sm text-muted-foreground">{t("assistant.welcomeBody")}</p>
      {signedIn ? (
        <div className="mt-6 flex flex-wrap justify-center gap-2">
          {suggestions.map((s) => (
            <button
              key={s}
              type="button"
              onClick={() => onPick(s)}
              className="rounded-full border border-white/10 bg-white/5 px-3 py-1.5 text-xs font-medium text-zinc-200 transition-colors hover:border-primary/50 hover:bg-primary/10 hover:text-foreground"
            >
              {s}
            </button>
          ))}
        </div>
      ) : (
        <Button className="mt-6" onClick={onSignIn}>
          {t("assistant.signInToChat")}
        </Button>
      )}
    </div>
  );
}
