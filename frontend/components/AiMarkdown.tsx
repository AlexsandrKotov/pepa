import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

interface AiMarkdownProps {
  content: string;
}

export default function AiMarkdown({ content }: AiMarkdownProps) {
  return (
    <ReactMarkdown remarkPlugins={[remarkGfm]} components={{
      p: ({ children }) => <p className="mb-2 last:mb-0">{children}</p>,
      ul: ({ children }) => <ul className="mb-2 last:mb-0 space-y-1 list-disc list-inside">{children}</ul>,
      ol: ({ children }) => <ol className="mb-2 last:mb-0 space-y-1 list-decimal list-inside">{children}</ol>,
      code: ({ className, children, ...props }) => {
        const isInline = !className && !String(children).includes('\n');
        return isInline ? (
          <code className="bg-[var(--border-light)] px-1.5 py-0.5 rounded text-[11px] font-mono" {...props}>{children}</code>
        ) : (
          <pre className="bg-[var(--border-light)] p-2 rounded text-[11px] font-mono overflow-x-auto my-2"><code {...props}>{children}</code></pre>
        );
      },
    }}>
      {content}
    </ReactMarkdown>
  );
}
