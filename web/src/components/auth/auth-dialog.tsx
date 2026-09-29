"use client";

import { useId, useState, type InputHTMLAttributes, type ReactNode } from "react";
import Image from "next/image";
import { AnimatePresence, motion } from "framer-motion";
import { Clapperboard, Eye, EyeOff, Loader2, Tv } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { ApiError } from "@/lib/api-client";
import { backdropUrl } from "@/lib/tmdb-image";
import { usePopularMovies } from "@/hooks/queries";
import { useLogin, useRegister } from "@/hooks/use-auth";
import { useT, type MessageKey } from "@/i18n";
import { useAuthPrompt, type AuthMode as Mode } from "@/store/auth-prompt-store";
import { useMediaModeStore } from "@/store/media-mode-store";

const COPY = {
  "sign-in": { title: "auth.signInTitle", description: "auth.signInDescription" },
  register: { title: "auth.registerTitle", description: "auth.registerDescription" },
} as const satisfies Record<Mode, { title: MessageKey; description: MessageKey }>;

/**
 * Sign-in / create-account modal. Any button can open it:
 *
 *   <AuthDialog trigger={<Button>Sign in</Button>} />
 *
 * Desktop is a split pane: a trending backdrop on the left, the form on a
 * glass panel on the right. Phones get the form only.
 */
export function AuthDialog({ trigger, defaultTab = "sign-in" }: { trigger: ReactNode; defaultTab?: Mode }) {
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState<Mode>(defaultTab);

  const onOpenChange = (next: boolean) => {
    setOpen(next);
    if (next) setMode(defaultTab);
  };

  return <AuthDialogFrame open={open} onOpenChange={onOpenChange} mode={mode} onModeChange={setMode} trigger={trigger} />;
}

/**
 * The single app-wide instance (mounted in Providers) that gated actions open
 * through the auth-prompt store; see useRequireAuth.
 */
export function GlobalAuthDialog() {
  const { open, mode, setOpen, setMode } = useAuthPrompt();
  return <AuthDialogFrame open={open} onOpenChange={setOpen} mode={mode} onModeChange={setMode} />;
}

function AuthDialogFrame({
  open,
  onOpenChange,
  mode,
  onModeChange: setMode,
  trigger,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: Mode;
  onModeChange: (mode: Mode) => void;
  trigger?: ReactNode;
}) {
  const { t } = useT();
  const setOpen = onOpenChange;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      {/* z-60: gated actions open this from inside other overlays (e.g. the
          comments sheet), so it must always stack above them. */}
      <DialogContent
        overlayClassName="z-[60]"
        className="z-[60] max-h-[calc(100dvh-2rem)] gap-0 overflow-hidden border-0 bg-zinc-950 p-0 ring-white/10 sm:max-w-4xl md:grid-cols-[1.1fr_1fr]"
      >
        <VisualPane />

        <div className="relative flex flex-col overflow-y-auto bg-zinc-950/90 px-6 py-8 backdrop-blur-2xl sm:px-10 sm:py-10">
          {/* Phones don't get the visual pane, so the brand lives here instead. */}
          <Logo className="mb-6 md:hidden" />

          <DialogTitle className="text-2xl font-bold tracking-tight text-white">{t(COPY[mode].title)}</DialogTitle>
          <DialogDescription className="mt-1.5 text-sm text-zinc-400">{t(COPY[mode].description)}</DialogDescription>

          <ModeToggle mode={mode} onChange={setMode} />

          {/* Fixed height fits the taller (register) form, so switching modes
              never makes the dialog jump in size. */}
          <div className="relative mt-6 min-h-[24.25rem] overflow-hidden">
            <AnimatePresence mode="wait" initial={false} custom={mode}>
              <motion.div
                key={mode}
                custom={mode}
                variants={slide}
                initial="enter"
                animate="center"
                exit="exit"
                transition={{ duration: 0.22, ease: "easeOut" }}
              >
                {mode === "sign-in" ? (
                  <SignInForm onSuccess={() => setOpen(false)} />
                ) : (
                  <RegisterForm onSuccess={() => setOpen(false)} />
                )}
                <SocialButtons />
              </motion.div>
            </AnimatePresence>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// Register sits to the right of sign-in in the toggle, so forms slide in the
// direction of travel.
const slide = {
  enter: (m: Mode) => ({ opacity: 0, x: m === "register" ? 32 : -32 }),
  center: { opacity: 1, x: 0 },
  exit: (m: Mode) => ({ opacity: 0, x: m === "register" ? -32 : 32 }),
};

/** The brand, in the app's current mode: KinoCut or KinoShow. */
function Logo({ className = "" }: { className?: string }) {
  const tv = useMediaModeStore((s) => s.mode) === "tv";
  const Icon = tv ? Tv : Clapperboard;
  return (
    <div className={`flex items-center gap-2 ${className}`}>
      <span className="flex size-9 items-center justify-center rounded-md bg-primary text-primary-foreground">
        <Icon className="size-5" strokeWidth={2.25} />
      </span>
      <span className="text-xl font-bold tracking-tight text-white">
        Kino<span className="text-primary">{tv ? "Show" : "Cut"}</span>
      </span>
    </div>
  );
}

/**
 * Trending backdrop behind a dark scrim, plus brand and tagline. Reuses the
 * popular-movies query the home page already holds in cache; picks one of the
 * top titles per open so the gateway feels alive.
 */
function VisualPane() {
  const { t } = useT();
  const { data } = usePopularMovies();
  const candidates = (data?.pages[0]?.results ?? []).filter((m) => m.backdrop_path).slice(0, 6);
  const [pick] = useState(() => Math.random());
  const movie = candidates.length ? candidates[Math.floor(pick * candidates.length)] : undefined;
  const backdrop = movie ? backdropUrl(movie.backdrop_path, "w1280") : null;

  return (
    <div className="relative hidden min-h-[36rem] overflow-hidden md:block">
      {/* Animated gradient: the fallback, and what shows while the image loads. */}
      <div className="auth-aurora absolute inset-0" aria-hidden />
      {backdrop && (
        <motion.div
          initial={{ opacity: 0, scale: 1.08 }}
          animate={{ opacity: 1, scale: 1 }}
          transition={{ duration: 1.2, ease: "easeOut" }}
          className="absolute inset-0"
        >
          <Image src={backdrop} alt="" fill sizes="480px" className="object-cover" priority />
        </motion.div>
      )}
      <div className="absolute inset-0 bg-gradient-to-t from-zinc-950 via-zinc-950/50 to-zinc-950/20" />
      <div className="absolute inset-0 bg-gradient-to-r from-transparent to-zinc-950/60" />

      <div className="relative flex h-full flex-col justify-between p-10">
        <Logo />
        <div>
          <p className="text-3xl font-bold leading-tight tracking-tight text-balance text-white">
            {t("auth.heroLine1")}
            <br />
            <span className="text-primary">{t("auth.heroLine2")}</span>
          </p>
          <p className="mt-3 max-w-xs text-sm text-zinc-300">
            {t("auth.heroBody")}
          </p>
          {movie && <p className="mt-8 text-xs uppercase tracking-widest text-zinc-500">{t("auth.nowTrending", { title: movie.title })}</p>}
        </div>
      </div>
    </div>
  );
}

function ModeToggle({ mode, onChange }: { mode: Mode; onChange: (m: Mode) => void }) {
  const { t } = useT();
  return (
    <div role="tablist" aria-label={t("auth.account")} className="mt-6 grid grid-cols-2 rounded-full bg-zinc-900/80 p-1 ring-1 ring-white/5">
      {(["sign-in", "register"] as const).map((m) => (
        <button
          key={m}
          type="button"
          role="tab"
          aria-selected={mode === m}
          onClick={() => onChange(m)}
          className={`relative rounded-full py-2 text-sm font-semibold outline-none transition-colors focus-visible:ring-2 focus-visible:ring-primary/60 ${
            mode === m ? "text-zinc-950" : "text-zinc-400 hover:text-white"
          }`}
        >
          {mode === m && (
            <motion.span
              layoutId="auth-mode-pill"
              className="absolute inset-0 rounded-full bg-white"
              transition={{ type: "spring", bounce: 0.2, duration: 0.4 }}
            />
          )}
          <span className="relative">{t(m === "sign-in" ? "auth.signIn" : "auth.createAccount")}</span>
        </button>
      ))}
    </div>
  );
}

/** Minimal input with a label that floats up on focus or once filled. */
function FloatingInput({
  label,
  hint,
  type = "text",
  ...props
}: { label: string; hint?: string } & InputHTMLAttributes<HTMLInputElement>) {
  const { t } = useT();
  const id = useId();
  const [reveal, setReveal] = useState(false);
  const isPassword = type === "password";

  return (
    <div>
      <div className="relative">
        <input
          id={id}
          type={isPassword && reveal ? "text" : type}
          placeholder=" "
          className={`peer h-14 w-full rounded-lg border border-white/10 bg-zinc-900/50 px-4 pt-5 pb-1.5 text-sm text-white outline-none transition-colors placeholder-transparent hover:border-white/20 focus:border-primary/70 focus:bg-zinc-900/80 focus:ring-2 focus:ring-primary/20 ${
            isPassword ? "pr-11" : ""
          }`}
          {...props}
        />
        <label
          htmlFor={id}
          className="pointer-events-none absolute left-4 top-2 text-xs text-zinc-400 transition-all peer-placeholder-shown:top-4.5 peer-placeholder-shown:text-sm peer-focus:top-2 peer-focus:text-xs peer-focus:text-primary"
        >
          {label}
        </label>
        {isPassword && (
          <button
            type="button"
            onClick={() => setReveal((r) => !r)}
            aria-label={t(reveal ? "auth.hidePassword" : "auth.showPassword")}
            className="absolute right-3 top-1/2 -translate-y-1/2 rounded p-1 text-zinc-500 outline-none transition-colors hover:text-white focus-visible:ring-2 focus-visible:ring-primary/60"
          >
            {reveal ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
          </button>
        )}
      </div>
      {hint && <p className="mt-1.5 px-1 text-xs text-zinc-500">{hint}</p>}
    </div>
  );
}

function SubmitButton({ pending, children }: { pending: boolean; children: ReactNode }) {
  return (
    <Button type="submit" size="lg" className="h-12 w-full text-base font-semibold" disabled={pending}>
      {pending && <Loader2 className="size-4 animate-spin" />}
      {children}
    </Button>
  );
}

/** Mock providers: the auth service only does username/password today. */
function SocialButtons() {
  const { t } = useT();
  const soon = (provider: string) => toast(t("auth.socialSoon", { provider }), { id: "social-auth-soon" });

  return (
    <>
      <div className="my-5 flex items-center gap-3 text-[11px] font-medium uppercase tracking-widest text-zinc-500">
        <span className="h-px flex-1 bg-white/10" />
        {t("auth.or")}
        <span className="h-px flex-1 bg-white/10" />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <SocialButton label="Google" onClick={() => soon("Google")}>
          <GoogleIcon />
        </SocialButton>
        <SocialButton label="Apple" onClick={() => soon("Apple")}>
          <AppleIcon />
        </SocialButton>
      </div>
    </>
  );
}

function SocialButton({ label, onClick, children }: { label: string; onClick: () => void; children: ReactNode }) {
  const { t } = useT();
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={t("auth.continueWith", { provider: label })}
      className="flex h-11 items-center justify-center gap-2 rounded-lg border border-white/10 bg-white/[0.03] text-sm font-medium text-white outline-none transition-colors hover:border-white/20 hover:bg-white/[0.07] focus-visible:ring-2 focus-visible:ring-primary/60"
    >
      {children}
      {label}
    </button>
  );
}

function GoogleIcon() {
  return (
    <svg viewBox="0 0 24 24" className="size-4" aria-hidden>
      <path fill="#EA4335" d="M12 10.2v3.9h5.5c-.24 1.26-.96 2.33-2.04 3.05l3.3 2.56c1.92-1.77 3.03-4.38 3.03-7.48 0-.72-.06-1.41-.18-2.07H12z" />
      <path fill="#34A853" d="M5.84 14.1l-.74.57-2.63 2.05C4.1 19.98 7.8 22 12 22c2.7 0 4.96-.89 6.61-2.42l-3.3-2.56c-.9.6-2.06.97-3.31.97-2.6 0-4.8-1.75-5.59-4.11z" />
      <path fill="#4A90E2" d="M2.47 6.28A9.96 9.96 0 0 0 2 12c0 1.62.39 3.15 1.07 4.5l3.37-2.61A5.98 5.98 0 0 1 6.1 12c0-.66.12-1.3.32-1.89z" />
      <path fill="#FBBC05" d="M12 5.98c1.47 0 2.78.5 3.82 1.5l2.86-2.86C16.95 3 14.7 2 12 2 7.8 2 4.1 4.02 2.47 7.28l3.37 2.61C6.64 7.53 8.84 5.98 12 5.98z" />
    </svg>
  );
}

function AppleIcon() {
  return (
    <svg viewBox="0 0 24 24" className="size-4 fill-current" aria-hidden>
      <path d="M16.37 12.62c-.02-2.2 1.8-3.26 1.88-3.31-1.02-1.5-2.62-1.7-3.19-1.72-1.36-.14-2.65.8-3.34.8-.69 0-1.75-.78-2.88-.76-1.48.02-2.85.86-3.61 2.19-1.54 2.67-.39 6.62 1.11 8.79.73 1.06 1.6 2.25 2.75 2.2 1.1-.04 1.52-.71 2.85-.71 1.33 0 1.71.71 2.88.69 1.19-.02 1.94-1.08 2.67-2.14.84-1.23 1.19-2.42 1.21-2.48-.03-.01-2.31-.89-2.33-3.55zM14.18 6.16c.61-.74 1.02-1.76.91-2.78-.88.04-1.94.59-2.57 1.32-.56.65-1.06 1.7-.93 2.7.98.08 1.98-.5 2.59-1.24z" />
    </svg>
  );
}

/** Turns an auth API failure into copy for the form. The service's own 400
 *  messages are already specific ("password must be at least 8 characters")
 *  and are shown as-is; the rest get friendlier wording. */
function errorMessage(err: unknown, t: (key: MessageKey) => string): string {
  if (!(err instanceof ApiError)) return t("auth.errorGeneric");
  switch (true) {
    case err.status === 0:
      return err.message; // unreachable: api-client's wording
    case err.status === 401:
      return t("auth.errorCredentials");
    case err.status === 409:
      return t("auth.errorTaken");
    case err.status === 429:
      return t("auth.errorThrottled");
    case err.status === 400:
      return err.message.charAt(0).toUpperCase() + err.message.slice(1) + ".";
    default:
      return t("auth.errorServer");
  }
}

function FormError({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-red-300">
      {children}
    </p>
  );
}

function SignInForm({ onSuccess }: { onSuccess: () => void }) {
  const { t } = useT();
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const mutation = useLogin();

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        mutation.mutate({ login, password }, { onSuccess });
      }}
    >
      <FloatingInput label={t("auth.usernameOrEmail")} autoComplete="username" required value={login} onChange={(e) => setLogin(e.target.value)} />
      <FloatingInput
        label={t("auth.password")}
        type="password"
        autoComplete="current-password"
        required
        value={password}
        onChange={(e) => setPassword(e.target.value)}
      />
      {mutation.isError && <FormError>{errorMessage(mutation.error, t)}</FormError>}
      <SubmitButton pending={mutation.isPending}>{t("auth.signIn")}</SubmitButton>
    </form>
  );
}

function RegisterForm({ onSuccess }: { onSuccess: () => void }) {
  const { t } = useT();
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const register = useRegister();
  const login = useLogin();
  const pending = register.isPending || login.isPending;

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        // Register, then immediately sign in with the same credentials so a
        // new user lands inside the app rather than at a second form.
        register.mutate(
          { username, email, password },
          { onSuccess: () => login.mutate({ login: username, password }, { onSuccess }) },
        );
      }}
    >
      <FloatingInput
        label={t("auth.username")}
        autoComplete="username"
        required
        minLength={3}
        maxLength={32}
        // Mirrors the Auth service's rule, so the browser flags it before a round trip.
        pattern="[A-Za-z0-9._\-]{3,32}"
        title={t("auth.usernameRule")}
        value={username}
        onChange={(e) => setUsername(e.target.value)}
      />
      <FloatingInput label={t("auth.email")} type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
      <FloatingInput
        label={t("auth.password")}
        type="password"
        autoComplete="new-password"
        required
        minLength={8}
        maxLength={72} // bcrypt's limit; the service rejects longer
        hint={t("auth.passwordHint")}
        value={password}
        onChange={(e) => setPassword(e.target.value)}
      />
      {register.isError && <FormError>{errorMessage(register.error, t)}</FormError>}
      {login.isError && (
        // The account exists at this point; only the automatic sign-in failed.
        <FormError>{t("auth.errorAutoLogin")}</FormError>
      )}
      <SubmitButton pending={pending}>{t("auth.createAccount")}</SubmitButton>
    </form>
  );
}
