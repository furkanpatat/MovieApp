import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

// Styled to sit inside a chat bubble. Raw HTML in the model's output is not
// rendered (react-markdown's default), and unsafe URLs are stripped.
const components: Components = {
  p: ({ children }) => <p className="mb-2 last:mb-0">{children}</p>,
  strong: ({ children }) => <strong className="font-semibold text-white">{children}</strong>,
  em: ({ children }) => <em className="text-zinc-200 italic">{children}</em>,
  ul: ({ children }) => <ul className="mb-2 list-disc space-y-1 pl-4 last:mb-0 marker:text-primary">{children}</ul>,
  ol: ({ children }) => <ol className="mb-2 list-decimal space-y-1 pl-4 last:mb-0 marker:text-primary">{children}</ol>,
  li: ({ children }) => <li className="pl-0.5">{children}</li>,
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noopener noreferrer nofollow" className="text-primary underline underline-offset-2">
      {children}
    </a>
  ),
  code: ({ children }) => <code className="rounded bg-white/10 px-1 py-0.5 font-mono text-[0.85em]">{children}</code>,
  blockquote: ({ children }) => <blockquote className="mb-2 border-l-2 border-primary/60 pl-3 text-zinc-300">{children}</blockquote>,
  // The model is told not to use headings; if it does, keep them bubble-sized.
  h1: ({ children }) => <p className="mb-1 font-bold text-white">{children}</p>,
  h2: ({ children }) => <p className="mb-1 font-bold text-white">{children}</p>,
  h3: ({ children }) => <p className="mb-1 font-semibold text-white">{children}</p>,
  hr: () => <hr className="my-2 border-white/10" />,
};

export function ChatMarkdown({ children }: { children: string }) {
  return (
    <div className="break-words">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {children}
      </ReactMarkdown>
    </div>
  );
}
