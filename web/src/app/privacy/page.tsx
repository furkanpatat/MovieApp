import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = {
  title: "Privacy Policy · KinoCut",
  description: "What KinoCut collects, why, and how to delete your account.",
};

/** When this policy last changed (a fixed date: update it with the text). */
const UPDATED = "September 29, 2026";

/**
 * The privacy policy for the KinoCut web app and the KinoCut mobile apps
 * (linked from the footer, the app's Profile and its store listings). It
 * describes what the services actually store: keep it in step with them.
 */
export default function PrivacyPage() {
  return (
    <article className="mx-auto w-full max-w-3xl space-y-10 px-4 py-16 text-sm leading-relaxed text-zinc-300 sm:py-24">
      <header className="space-y-2">
        <h1 className="text-3xl font-bold tracking-tight text-white sm:text-4xl">Privacy Policy</h1>
        <p className="text-zinc-500">Last updated: {UPDATED}</p>
        <p>
          This policy covers KinoCut (KinoCut ⇄ KinoShow): the website at kinora.duckdns.org and the KinoCut apps for
          Android and iOS. KinoCut is a personal, non-commercial project. It shows no ads, uses no analytics or
          tracking, and never sells your data.
        </p>
      </header>

      <Section title="1. What we collect">
        <ul className="list-disc space-y-2 pl-6">
          <li>
            <b className="text-white">Your account:</b> username, email address, and your password, which is stored
            only as a bcrypt hash (never the password itself).
          </li>
          <li>
            <b className="text-white">What you do in KinoCut:</b> your ratings and likes, comments, your list, and the
            titles you mark as watched.
          </li>
          <li>
            <b className="text-white">Sessions:</b> a session cookie on the website; on the apps, a sign-in token kept
            in your device&apos;s secure storage (the server keeps only a hash of it).
          </li>
          <li>
            <b className="text-white">Server logs:</b> for security and to prevent abuse (such as rate limiting), our
            servers log requests, including your IP address.
          </li>
        </ul>
        <p>We don&apos;t collect your location, contacts, photos or any other data from your device.</p>
      </Section>

      <Section title="2. What others can see">
        <p>
          Your comments and your username are visible to everyone. The titles you mark as watched appear on your
          public profile. Your list, your ratings and your email address are private. Watch Party chat goes only to
          the people in the room and isn&apos;t stored.
        </p>
        <p>
          Comments must be civil. You can report any comment, or block its author to hide all their comments, from the
          menu next to it. We review reports and remove comments and accounts that break these rules.
        </p>
      </Section>

      <Section title="3. Services we rely on">
        <ul className="list-disc space-y-2 pl-6">
          <li>
            <b className="text-white">TMDB</b> and <b className="text-white">OMDb</b> provide movie and series data and
            images. Your personal data is not sent to them. This application uses TMDB and the TMDB APIs but is not
            endorsed, certified, or otherwise approved by TMDB.
          </li>
          <li>
            <b className="text-white">Groq</b> powers the AI assistant: the messages you send it (and the page
            you&apos;re on) are sent to Groq to generate an answer. We don&apos;t store these conversations. Don&apos;t
            share personal information with the assistant.
          </li>
          <li>
            <b className="text-white">YouTube</b> plays the trailers. When a trailer plays, YouTube (Google) may collect
            data under its own privacy policy.
          </li>
          <li>
            <b className="text-white">Oracle Cloud</b> hosts our servers and database (United States).
          </li>
        </ul>
      </Section>

      <Section title="4. How long we keep it">
        <p>
          We keep your data while you have an account. When you delete your account, it is removed at once. Database
          backups are kept for 14 days, so deleted data disappears from them within 14 days. Server logs are kept only
          for a limited time.
        </p>
      </Section>

      <Section title="5. Deleting your account">
        <p>
          You can delete your account at any time: in the app or on the website, open <b className="text-white">Profile</b>{" "}
          and choose <b className="text-white">Delete account</b>, then confirm with your password. This permanently
          removes your account, your sessions, your ratings, comments, list and watched titles. It can&apos;t be
          undone.
        </p>
      </Section>

      <Section title="6. Security">
        <p>
          All traffic is encrypted (HTTPS). Passwords are hashed with bcrypt, sign-in tokens are stored only as
          hashes, and our database is not reachable from the internet.
        </p>
      </Section>

      <Section title="7. Children">
        <p>KinoCut is not directed at children under 13, and we don&apos;t knowingly collect their data.</p>
      </Section>

      <Section title="8. Changes and contact">
        <p>
          If this policy changes, we&apos;ll update it here and change the date above. Questions or requests about your
          data:{" "}
          <a href="https://github.com/furkanpatat/MovieApp/issues" className="text-primary underline-offset-4 hover:underline">
            github.com/furkanpatat/MovieApp/issues
          </a>
          .
        </p>
      </Section>
    </article>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-3">
      <h2 className="text-xl font-semibold text-white">{title}</h2>
      {children}
    </section>
  );
}
