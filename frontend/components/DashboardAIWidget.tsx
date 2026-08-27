'use client';

import { useState, useRef, useEffect } from 'react';
import dynamic from 'next/dynamic';
import { ai, isAuthenticated, getStoredUser, type AIChatResponse } from '@/lib/api';

// Lazy-load heavy markdown renderer — only parsed when AI responds
const AiMarkdown = dynamic(() => import('./AiMarkdown'), { ssr: false });

interface Message {
  role: 'user' | 'assistant';
  content: string;
}

const LS_KEY = 'pepa-ai-widget-messages';
const MAX_WIDGET_MESSAGES = 50;

function loadWidgetMessages(): Message[] {
  if (typeof window === 'undefined') return [];
  const user = getStoredUser();
  if (!user?.id) return [];
  try {
    const raw = localStorage.getItem(`${LS_KEY}-${user.id}`);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) return parsed.slice(-MAX_WIDGET_MESSAGES);
  } catch { /* ignore */ }
  return [];
}

function saveWidgetMessages(msgs: Message[]) {
  if (typeof window === 'undefined') return;
  const user = getStoredUser();
  if (!user?.id) return;
  try {
    localStorage.setItem(`${LS_KEY}-${user.id}`, JSON.stringify(msgs.slice(-MAX_WIDGET_MESSAGES)));
  } catch { /* ignore */ }
}

export default function DashboardAIWidget() {
  const [isAuth, setIsAuth] = useState(false);
  const [isOpen, setIsOpen] = useState(false);
  const [isMinimized, setIsMinimized] = useState(false);
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [height, setHeight] = useState(400);
  const [width, setWidth] = useState(380);
  const endRef = useRef<HTMLDivElement>(null);
  const resizeRef = useRef<{ startY: number; startHeight: number; startX: number; startWidth: number } | null>(null);

  // Only show widget when authenticated
  useEffect(() => {
    setIsAuth(isAuthenticated());
  }, []);

  // Load persisted messages when opening
  useEffect(() => {
    if (isOpen && messages.length === 0) {
      const saved = loadWidgetMessages();
      if (saved.length > 0) {
        setMessages(saved);
      } else {
        setMessages([{ role: 'assistant', content: 'Hi! I\'m your PEPA AI assistant. Ask me about deployments, services, or platform status.' }]);
      }
    }
  }, [isOpen]); // eslint-disable-line react-hooks/exhaustive-deps

  // Persist messages after each change
  useEffect(() => {
    if (messages.length > 0) {
      saveWidgetMessages(messages);
    }
  }, [messages]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const send = async () => {
    if (!input.trim() || loading) return;
    const userMsg: Message = { role: 'user', content: input.trim() };
    setMessages(prev => [...prev, userMsg]);
    setInput('');
    setLoading(true);

    try {
      const res = await ai.chat({ message: userMsg.content, enable_tools: true }) as AIChatResponse;
      setMessages(prev => [...prev, { role: 'assistant', content: res.response }]);
    } catch {
      // Agent mode failed (likely model doesn't support function calling) — fall back to simple chat
      try {
        const res = await ai.chat({ message: userMsg.content, enable_tools: false }) as AIChatResponse;
        setMessages(prev => [...prev, { role: 'assistant', content: res.response }]);
      } catch (err) {
        const errorMsg = err instanceof Error ? err.message : 'Unknown error';
        setMessages(prev => [...prev, {
          role: 'assistant',
          content: `I'm having trouble connecting right now. ${errorMsg}`,
        }]);
      }
    } finally {
      setLoading(false);
    }
  };

  const handleMouseDown = (e: React.MouseEvent, direction: 'top' | 'right' | 'corner') => {
    e.preventDefault();
    resizeRef.current = {
      startY: e.clientY,
      startHeight: height,
      startX: e.clientX,
      startWidth: width,
    };

    const handleMouseMove = (e: MouseEvent) => {
      if (!resizeRef.current) return;
      const deltaY = e.clientY - resizeRef.current.startY;
      const deltaX = e.clientX - resizeRef.current.startX;

      if (direction === 'top') {
        setHeight(Math.max(300, Math.min(800, resizeRef.current.startHeight - deltaY)));
      } else if (direction === 'right') {
        setWidth(Math.max(320, Math.min(800, resizeRef.current.startWidth + deltaX)));
      } else if (direction === 'corner') {
        setHeight(Math.max(300, Math.min(800, resizeRef.current.startHeight - deltaY)));
        setWidth(Math.max(320, Math.min(800, resizeRef.current.startWidth + deltaX)));
      }
    };

    const handleMouseUp = () => {
      resizeRef.current = null;
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };

    document.body.style.cursor = direction === 'right' ? 'ew-resize' : direction === 'corner' ? 'nwse-resize' : 'ns-resize';
    document.body.style.userSelect = 'none';
    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
  };

  const quickQuestions = [
    'What services are deployed?',
    'Show cluster health',
    'Recent deployments',
  ];

  // Don't render anything if not authenticated
  if (!isAuth) return null;

  if (!isOpen) {
    return (
      <button
        onClick={() => setIsOpen(true)}
        className="fixed bottom-6 right-6 z-50 w-14 h-14 rounded-full bg-[var(--accent)] text-white shadow-lg hover:shadow-xl hover:scale-105 transition-all duration-200 flex items-center justify-center group"
      >
        <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
        </svg>
        <span className="absolute -top-1 -right-1 w-3 h-3 bg-green-500 rounded-full border-2 border-white dark:border-[var(--bg)]" />
      </button>
    );
  }

  return (
    <div
      className="fixed bottom-6 right-6 z-50 flex flex-col card shadow-2xl"
      style={{
        width: `${width}px`,
        height: isMinimized ? 'auto' : `${height}px`,
        borderRadius: '16px',
        transition: isMinimized ? 'all 0.2s ease' : 'none',
      }}
    >
      {/* Resize handle - top edge */}
      {!isMinimized && (
        <div
          onMouseDown={(e) => handleMouseDown(e, 'top')}
          className="absolute top-0 left-4 right-4 h-2 cursor-n-resize hover:bg-[var(--accent)] hover:opacity-30 rounded-t-2xl transition-colors z-10"
        />
      )}

      {/* Resize handle - right edge */}
      {!isMinimized && (
        <div
          onMouseDown={(e) => handleMouseDown(e, 'right')}
          className="absolute top-4 bottom-4 right-0 w-2 cursor-e-resize hover:bg-[var(--accent)] hover:opacity-30 rounded-r-2xl transition-colors z-10"
        />
      )}

      {/* Resize handle - corner */}
      {!isMinimized && (
        <div
          onMouseDown={(e) => handleMouseDown(e, 'corner')}
          className="absolute top-0 right-0 w-6 h-6 cursor-nwse-resize hover:bg-[var(--accent)] hover:opacity-30 rounded-tr-2xl transition-colors z-10 flex items-center justify-center"
        >
          <svg className="w-3 h-3 text-[var(--text-tertiary)] opacity-50" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M7 17L17 7M17 7H7M17 7V17" />
          </svg>
        </div>
      )}

      {/* Header */}
      <div className="px-4 py-3 border-b border-[var(--border-light)] flex items-center justify-between shrink-0" style={{ background: 'linear-gradient(135deg, var(--accent-subtle), transparent)' }}>
        <div className="flex items-center gap-2.5">
          <div className="w-8 h-8 rounded-full bg-[var(--accent)] flex items-center justify-center">
            <svg className="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
            </svg>
          </div>
          <div>
            <p className="text-[13px] font-semibold text-[var(--text-primary)]">PEPA AI</p>
            <p className="text-[10px] text-[var(--text-tertiary)]">Platform assistant</p>
          </div>
        </div>
        <div className="flex items-center gap-1">
          <button
            onClick={() => setIsMinimized(!isMinimized)}
            className="w-7 h-7 rounded-full flex items-center justify-center hover:bg-[var(--border-light)] transition-colors"
            title={isMinimized ? 'Expand' : 'Minimize'}
          >
            <svg className="w-3.5 h-3.5 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              {isMinimized ? (
                <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
              ) : (
                <path strokeLinecap="round" strokeLinejoin="round" d="M19 15l-7-7-7 7" />
              )}
            </svg>
          </button>
          <button
            onClick={() => setIsOpen(false)}
            className="w-7 h-7 rounded-full flex items-center justify-center hover:bg-[var(--border-light)] transition-colors"
            title="Close"
          >
            <svg className="w-3.5 h-3.5 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      </div>

      {/* Content */}
      {!isMinimized && (
        <>
          {/* Messages */}
          <div className="flex-1 overflow-y-auto p-3 space-y-2.5 min-h-0">
            {messages.length === 1 && (
              <div className="flex flex-wrap gap-1.5 pt-2">
                {quickQuestions.map(q => (
                  <button
                    key={q}
                    onClick={() => { setInput(q); }}
                    className="px-2.5 py-1.5 text-[11px] bg-[var(--bg)] text-[var(--text-secondary)] rounded-full hover:bg-[var(--border-light)] transition-colors border border-[var(--border-light)]"
                  >
                    {q}
                  </button>
                ))}
              </div>
            )}
            {messages.map((msg, i) => (
              <div key={i} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                <div className={`max-w-[85%] px-3 py-2 text-[12px] leading-relaxed rounded-2xl ${
                  msg.role === 'user'
                    ? 'bg-[var(--accent)] text-white rounded-br-md'
                    : 'bg-[var(--bg)] text-[var(--text-primary)] rounded-bl-md border border-[var(--border-light)]'
                }`}>
                  {msg.role === 'assistant' ? (
                    <AiMarkdown content={msg.content} />
                  ) : (
                    <span className="whitespace-pre-wrap">{msg.content}</span>
                  )}
                </div>
              </div>
            ))}
            {loading && (
              <div className="flex justify-start">
                <div className="bg-[var(--bg)] rounded-2xl rounded-bl-md px-3 py-2.5 border border-[var(--border-light)]">
                  <div className="flex gap-1">
                    <span className="w-1.5 h-1.5 bg-[var(--text-tertiary)] rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
                    <span className="w-1.5 h-1.5 bg-[var(--text-tertiary)] rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
                    <span className="w-1.5 h-1.5 bg-[var(--text-tertiary)] rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
                  </div>
                </div>
              </div>
            )}
            <div ref={endRef} />
          </div>

          {/* Input */}
          <div className="p-2.5 border-t border-[var(--border-light)] shrink-0">
            <div className="flex items-center gap-2">
              <input
                value={input}
                onChange={e => setInput(e.target.value)}
                onKeyDown={e => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    send();
                  }
                }}
                placeholder="Ask about your platform..."
                className="flex-1 px-3 py-2 text-[12px] bg-[var(--bg)] border border-[var(--border)] rounded-xl focus:outline-none focus:border-[var(--accent)] transition-colors"
                disabled={loading}
              />
              <button
                onClick={send}
                disabled={loading || !input.trim()}
                className="w-8 h-8 rounded-full bg-[var(--accent)] text-white flex items-center justify-center disabled:opacity-30 hover:opacity-85 transition-opacity"
              >
                <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 12L3.269 3.126A59.768 59.768 0 0121.485 12 59.77 59.77 0 013.27 20.876L5.999 12zm0 0h7.5" />
                </svg>
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
