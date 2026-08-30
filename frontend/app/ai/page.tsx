'use client';

import { useState, useEffect, useRef, type ReactNode } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { aiExtended, ai, getStoredUser, type AIChatResponse, type AIToolCall, type AIStatus, type AIStreamChunk } from '@/lib/api';
import { useRouter } from 'next/navigation';
import { Toast } from '@/components/Interactive';
import ConfirmModal from '@/components/ConfirmModal';

interface Message {
  role: 'user' | 'assistant';
  content: string;
  type?: string;
  toolCalls?: AIToolCall[];
  toolsUsed?: number;
}

interface Chat {
  id: string;
  title: string;
  createdAt: number;
  updatedAt: number;
  messages: Message[];
}

const AI_CHATS_LS_KEY = 'pepa-ai-chats';
const AI_CURRENT_CHAT_LS_KEY = 'pepa-ai-current-chat';
const AI_CHAT_LS_KEY = 'pepa-ai-chat-messages'; // legacy single-chat key, migrated on load
const AI_SYSTEM_INSTRUCTION_LS_KEY = 'pepa-ai-system-instruction';
const AI_MAX_MESSAGES = 200;
const AI_TITLE_MAX = 40;

const GREETING = `Hello! I'm the PEPA AI Assistant. I can help you with:

- **Platform data** — enable Agent mode to query your services, deployments, clusters, pipelines, Docker containers
- **DevOps knowledge** — Kubernetes, Docker, CI/CD, GitOps, Helm, Terraform
- **Config generation** — Helm values, K8s manifests, docker-compose, Terraform

Привет! Я ИИ-ассистент PEPA. Включите режим Agent для получения данных о ваших сервисах, деплоях, кластерах и пайплайнах.`;

const PROVIDER_COLORS: Record<string, string> = {
  openai: '#10A37F', anthropic: '#D97757', groq: '#F55036', qoder: '#8B5CF6', ollama: '#4A4A4A', lmstudio: '#6366F1',
};

function newChat(): Chat {
  const now = Date.now();
  return { id: `chat-${now}-${Math.random().toString(36).slice(2, 8)}`, title: 'New chat', createdAt: now, updatedAt: now, messages: [{ role: 'assistant', content: GREETING }] };
}

function loadChats(): Chat[] | null {
  if (typeof window === 'undefined') return null;
  const user = getStoredUser();
  if (!user?.id) return null;
  try {
    const raw = localStorage.getItem(`${AI_CHATS_LS_KEY}-${user.id}`);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed) && parsed.length > 0) {
        return parsed
          .filter((c: Chat) => c && c.id && Array.isArray(c.messages))
          .map((c: Chat) => ({ ...c, messages: c.messages.slice(-AI_MAX_MESSAGES) }));
      }
    }
    // Migrate legacy single-chat storage into a chat list
    const legacy = localStorage.getItem(`${AI_CHAT_LS_KEY}-${user.id}`);
    if (legacy) {
      const msgs = JSON.parse(legacy);
      if (Array.isArray(msgs) && msgs.length > 0) {
        const chat = newChat();
        chat.messages = msgs.slice(-AI_MAX_MESSAGES);
        const firstUser = chat.messages.find(m => m.role === 'user');
        if (firstUser) chat.title = firstUser.content.slice(0, AI_TITLE_MAX) + (firstUser.content.length > AI_TITLE_MAX ? '…' : '');
        localStorage.removeItem(`${AI_CHAT_LS_KEY}-${user.id}`);
        return [chat];
      }
    }
  } catch { /* ignore */ }
  return null;
}

function saveChats(chats: Chat[]) {
  if (typeof window === 'undefined') return;
  const user = getStoredUser();
  if (!user?.id) return;
  try {
    localStorage.setItem(`${AI_CHATS_LS_KEY}-${user.id}`, JSON.stringify(
      chats.map(c => ({ ...c, messages: c.messages.slice(-AI_MAX_MESSAGES) }))
    ));
  } catch { /* ignore */ }
}

function loadCurrentChatId(): string | null {
  if (typeof window === 'undefined') return null;
  const user = getStoredUser();
  if (!user?.id) return null;
  try { return localStorage.getItem(`${AI_CURRENT_CHAT_LS_KEY}-${user.id}`); } catch { return null; }
}

function saveCurrentChatId(id: string | null) {
  if (typeof window === 'undefined') return;
  const user = getStoredUser();
  if (!user?.id) return;
  try {
    if (id) localStorage.setItem(`${AI_CURRENT_CHAT_LS_KEY}-${user.id}`, id);
    else localStorage.removeItem(`${AI_CURRENT_CHAT_LS_KEY}-${user.id}`);
  } catch { /* ignore */ }
}

function formatChatTime(ts: number): string {
  const d = new Date(ts);
  const now = new Date();
  if (d.toDateString() === now.toDateString()) {
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
  return d.toLocaleDateString([], { day: 'numeric', month: 'short' });
}

export default function AIAssistantPage() {
  const [chats, setChats] = useState<Chat[]>([]);
  const [currentChatId, setCurrentChatId] = useState<string | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [tab, setTab] = useState<'chat' | 'history' | 'generate'>('chat');
  const [history, setHistory] = useState<Array<Record<string, unknown>>>([]);
  const [suggestions, setSuggestions] = useState<Array<Record<string, unknown>>>([]);
  const [genType, setGenType] = useState('pepa_service_blueprint');
  const [genName, setGenName] = useState('');
  const [genResult, setGenResult] = useState('');
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [mounted, setMounted] = useState(false);
  const [agentMode, setAgentMode] = useState<'chat' | 'agent'>('agent');
  const [providers, setProviders] = useState<AIStatus['providers']>([]);
  const [defaultProvider, setDefaultProvider] = useState('');
  const [selectedProvider, setSelectedProvider] = useState('');
  const [applying, setApplying] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Chat | null>(null);
  const [clearAllOpen, setClearAllOpen] = useState(false);
  const [systemInstruction, setSystemInstruction] = useState('');
  const [sysInstrOpen, setSysInstrOpen] = useState(false);
  const [providerDropdownOpen, setProviderDropdownOpen] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const providerDropdownRef = useRef<HTMLDivElement>(null);
  const chatsRef = useRef<Chat[]>([]);
  const currentChatIdRef = useRef<string | null>(null);
  const router = useRouter();
  chatsRef.current = chats;
  currentChatIdRef.current = currentChatId;

  const currentChat = chats.find(c => c.id === currentChatId) || null;

  // Type descriptions for help text — only types that can be deployed via Apply
  const typeDescriptions: Record<string, string> = {
    pepa_service_blueprint: 'Service template for PEPA\'s blueprint library',
    pepa_workflow: 'Automation workflow with build/test/deploy steps',
    pepa_scorecard: 'Scoring criteria for service evaluation',
    pepa_pipeline_source: 'Pipeline source (GitLab CI, Ansible, Terraform)',
    pepa_rbac_policy: 'Role-based access control policy',
    pepa_gitops: 'GitOps repository configuration (FluxCD/ArgoCD)',
  };

  // Load persisted chats and system instruction on mount
  useEffect(() => {
    const stored = loadChats();
    if (stored && stored.length > 0) {
      const sorted = [...stored].sort((a, b) => b.updatedAt - a.updatedAt);
      const savedId = loadCurrentChatId();
      const target = sorted.find(c => c.id === savedId) || sorted[0];
      setChats(sorted);
      setCurrentChatId(target.id);
      setMessages(target.messages);
    } else {
      const chat = newChat();
      setChats([chat]);
      setCurrentChatId(chat.id);
      setMessages(chat.messages);
    }
    // Load system instruction
    try {
      const savedInstr = localStorage.getItem(AI_SYSTEM_INSTRUCTION_LS_KEY);
      if (savedInstr) setSystemInstruction(savedInstr);
    } catch { /* ignore */ }
    // Load saved provider preference
    try {
      const savedProvider = localStorage.getItem('pepa-ai-provider');
      if (savedProvider) setSelectedProvider(savedProvider);
    } catch { /* ignore */ }
    setMounted(true);
    loadSuggestions();
    loadHistory();
    loadProviders();
  }, []);
  // Scroll to bottom when messages change
  useEffect(() => { messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [messages]);
  // Persist chats after each change
  useEffect(() => {
    if (chats.length > 0) {
      saveChats(chats);
    }
  }, [chats]);
  useEffect(() => {
    if (currentChatId) saveCurrentChatId(currentChatId);
  }, [currentChatId]);
  // Close provider dropdown on outside click
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (providerDropdownRef.current && !providerDropdownRef.current.contains(e.target as Node)) {
        setProviderDropdownOpen(false);
      }
    };
    if (providerDropdownOpen) document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [providerDropdownOpen]);

  const updateCurrentChat = (updater: (c: Chat) => Chat) => {
    setChats(prev => prev.map(c => c.id === currentChatIdRef.current ? updater(c) : c));
  };

  const handleNewChat = () => {
    const chat = newChat();
    setChats(prev => [chat, ...prev]);
    setCurrentChatId(chat.id);
    setMessages(chat.messages);
    setInput('');
    textareaRef.current?.focus();
  };

  const handleSelectChat = (chat: Chat) => {
    if (chat.id === currentChatId || loading) return;
    setCurrentChatId(chat.id);
    setMessages(chat.messages);
  };

  const handleDeleteChat = (chat: Chat) => {
    setChats(prev => {
      const rest = prev.filter(c => c.id !== chat.id);
      if (chat.id === currentChatIdRef.current) {
        const next = rest.length > 0 ? [...rest].sort((a, b) => b.updatedAt - a.updatedAt)[0] : newChat();
        if (rest.length === 0) rest.push(next);
        setCurrentChatId(next.id);
        setMessages(next.messages);
      }
      return rest;
    });
    setDeleteTarget(null);
    setToast({ message: 'Chat deleted', type: 'success' });
  };

  const handleClearAllChats = () => {
    const chat = newChat();
    setChats([chat]);
    setCurrentChatId(chat.id);
    setMessages(chat.messages);
    setClearAllOpen(false);
    setToast({ message: 'All chats deleted', type: 'success' });
  };

  const loadSuggestions = async () => {
    try { const data = await aiExtended.suggestions(); setSuggestions(data.suggestions || []); } catch { /* ignore */ }
  };

  const loadHistory = async () => {
    try { const data = await aiExtended.history(); setHistory(data.history || []); } catch { /* ignore */ }
  };

  const loadProviders = async () => {
    try {
      const data = await ai.status();
      setProviders(data.providers || []);
      setDefaultProvider(data.default_provider || '');
    } catch { /* ignore */ }
  };

  const handleSend = async () => {
    if (!input.trim() || loading) return;
    const userMsg = input.trim();
    setInput('');
    setMessages(prev => {
      const next = [...prev, { role: 'user' as const, content: userMsg }];
      updateCurrentChat(c => ({
        ...c,
        messages: next,
        updatedAt: Date.now(),
        title: c.title === 'New chat' ? userMsg.slice(0, AI_TITLE_MAX) + (userMsg.length > AI_TITLE_MAX ? '\u2026' : '') : c.title,
      }));
      return next;
    });
    setLoading(true);
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto';
    }
  
    try {
      // Build conversation history from current chat messages (excluding the greeting)
      const currentMessages = chatsRef.current.find(c => c.id === currentChatIdRef.current)?.messages || [];
      const history = currentMessages
        .filter(m => (m.role === 'user' || m.role === 'assistant') && m.content !== GREETING)
        .slice(-20) // last 20 messages for context
        .map(m => ({ role: m.role, content: m.content }));
  
      const chatReq = {
        message: userMsg,
        enable_tools: agentMode === 'agent',
        agent_mode: '',
        history,
        system_instruction: systemInstruction || undefined,
        provider: selectedProvider || undefined,
      };

      // Add a placeholder assistant message that will be updated as chunks arrive
      setMessages(prev => {
        const next = [...prev, { role: 'assistant' as const, content: '' }];
        updateCurrentChat(c => ({ ...c, messages: next, updatedAt: Date.now() }));
        return next;
      });

      const toolCallsAccum: AIToolCall[] = [];
      let needsApproval: AIChatResponse['needs_approval'] | undefined;

      await ai.chatStream(chatReq, (chunk: AIStreamChunk) => {
        if (chunk.type === 'text' && chunk.content) {
          setMessages(prev => {
            const next = [...prev];
            const last = next[next.length - 1];
            if (last?.role === 'assistant') {
              next[next.length - 1] = { ...last, content: last.content + chunk.content };
            }
            updateCurrentChat(c => ({ ...c, messages: next, updatedAt: Date.now() }));
            return next;
          });
        } else if (chunk.type === 'tool_call' || chunk.type === 'tool_result') {
          const meta = chunk.metadata || {};
          const toolName = (meta.tool_name as string) || '';
          // Merge tool call/result into accumulated tool calls
          const existing = toolCallsAccum.find(t => t.tool_name === toolName && !t.result);
          if (existing) {
            if (chunk.tool_result) {
              existing.result = chunk.tool_result.content;
              existing.error = chunk.tool_result.error;
            }
            existing.policy = (meta.policy as string) || existing.policy;
          } else if (chunk.type === 'tool_call') {
            toolCallsAccum.push({
              tool_name: toolName,
              tool_args: (meta.params as Record<string, unknown>) || {},
              policy: (meta.policy as string) || 'observe',
              timestamp: new Date().toISOString(),
            });
          }
          if (meta.needs_approval) {
            needsApproval = {
              tool_name: toolName,
              tool_args: (meta.params as Record<string, unknown>) || {},
              description: `Action ${toolName} requires approval`,
              reason: 'This action modifies platform state.',
            };
          }
          // Update the message with tool call info
          setMessages(prev => {
            const next = [...prev];
            const last = next[next.length - 1];
            if (last?.role === 'assistant') {
              next[next.length - 1] = {
                ...last,
                toolCalls: [...toolCallsAccum],
                toolsUsed: new Set(toolCallsAccum.map(t => t.tool_name)).size,
              };
            }
            updateCurrentChat(c => ({ ...c, messages: next, updatedAt: Date.now() }));
            return next;
          });
        } else if (chunk.type === 'error') {
          setMessages(prev => {
            const next = [...prev];
            const last = next[next.length - 1];
            if (last?.role === 'assistant') {
              next[next.length - 1] = { ...last, content: last.content + `\n\nError: ${chunk.content || chunk.error || 'Unknown error'}` };
            }
            updateCurrentChat(c => ({ ...c, messages: next, updatedAt: Date.now() }));
            return next;
          });
        }
      });

      // Final update: append approval notice if needed
      if (needsApproval) {
        const approval = needsApproval;
        setMessages(prev => {
          const next = [...prev];
          const last = next[next.length - 1];
          if (last?.role === 'assistant') {
            next[next.length - 1] = {
              ...last,
              content: last.content + `\n\n\u26a0\ufe0f Action \`${approval.tool_name}\` requires your approval before execution.`,
            };
          }
          updateCurrentChat(c => ({ ...c, messages: next, updatedAt: Date.now() }));
          return next;
        });
      }

      // Auto-navigate to the relevant page after successful mutating tool calls
      const toolNavMap: Record<string, string> = {
        create_blueprint: '/pipeline-blueprints',
        create_service: '/services',
        create_entity: '/entities',
        create_environment: '/environments',
        create_connection: '/connections',
        execute_workflow: '/workflows',
        trigger_pipeline: '/pipelines',
      };
      const succeededTool = toolCallsAccum.find(t => !t.error && toolNavMap[t.tool_name]);
      if (succeededTool) {
        const target = toolNavMap[succeededTool.tool_name];
        setToast({ message: `${succeededTool.tool_name} succeeded! Redirecting...`, type: 'success' });
        setTimeout(() => router.push(target), 1200);
      }

      loadHistory();
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : 'Unknown error';
      setMessages(prev => {
        const next = [...prev];
        const last = next[next.length - 1];
        if (last?.role === 'assistant' && last.content === '') {
          // Replace empty placeholder with error
          next[next.length - 1] = { ...last, content: `Sorry, I encountered an error: ${errorMsg}` };
        } else {
          next.push({ role: 'assistant' as const, content: `Sorry, I encountered an error: ${errorMsg}` });
        }
        updateCurrentChat(c => ({ ...c, messages: next, updatedAt: Date.now() }));
        return next;
      });
    } finally { setLoading(false); }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInput(e.target.value);
    const el = e.target;
    el.style.height = 'auto';
    el.style.height = Math.min(el.scrollHeight, 200) + 'px';
  };

  const handleGenerate = async () => {
    if (!genName.trim()) { setToast({ message: 'Enter a service name', type: 'error' }); return; }
    setLoading(true);
    try {
      const result = await aiExtended.generate({ type: genType, name: genName });
      setGenResult(result.content);
      setToast({ message: 'Configuration generated!', type: 'success' });
    } catch { setToast({ message: 'Generation failed', type: 'error' }); }
    finally { setLoading(false); }
  };

  const quickActions = [
    { label: 'List all services', query: 'List all my services including Docker services and their status' },
    { label: 'Refresh Docker', query: 'Refresh all Docker services and show their current status' },
    { label: 'Check deployments', query: 'Show me recent deployments and their status' },
    { label: 'Pipeline status', query: 'Show my pipelines and their recent runs' },
    { label: 'Create service', query: 'Create a new service called my-api with language Go' },
    { label: 'Deploy service', query: 'Deploy service to staging environment' },
    { label: 'Restart Docker svc', query: 'Restart a Docker service' },
    { label: 'Trigger pipeline', query: 'Trigger a new pipeline run' },
  ];

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 flex flex-col" style={{ height: 'calc(100vh - 48px)' }}>
        {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

        {/* Header */}
        <div className="page-animate shrink-0 mb-4">
          <h1 className="page-title-modern">{agentMode === 'agent' ? 'AI Agent' : 'AI Chat'}</h1>
          <p className="page-subtitle-modern">
            {agentMode === 'agent' ? 'Platform agent with access to your PEPA data' : 'Simple conversation with the AI assistant'}
            {selectedProvider && <span className="ml-2 text-[var(--accent)]">({selectedProvider})</span>}
          </p>
        </div>

        {/* Tabs */}
        <div className="flex gap-1 border-b border-[var(--border)] page-animate-up page-delay-1 shrink-0">
          {(['chat', 'generate', 'history'] as const).map(t => (
            <button key={t} onClick={() => setTab(t)} className={`px-4 py-2 text-[13px] font-medium border-b-2 transition-colors ${tab === t ? 'border-[var(--accent)] text-[var(--accent)]' : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}`}>{t.charAt(0).toUpperCase() + t.slice(1)}</button>
          ))}
        </div>

        {/* Chat Tab */}
        {tab === 'chat' && (
          <div className="flex-1 min-h-0 grid grid-cols-1 lg:grid-cols-4 gap-4 pt-4 page-animate-up page-delay-2">
            <div className="lg:col-span-3 card flex flex-col min-h-0" style={{ borderRadius: '12px' }}>
              <div className="card-header border-b border-[var(--border)] shrink-0 flex items-center justify-between">
                <div className="flex items-center gap-2 min-w-0">
                  <span className="text-[13px] font-medium text-[var(--text-primary)] truncate">{currentChat?.title || (agentMode === 'agent' ? 'Agent Chat' : 'Chat')}</span>
                  <span className="text-[11px] text-[var(--text-tertiary)] shrink-0">{agentMode === 'agent' ? 'Agent uses tools to access your PEPA data' : 'Simple conversation mode'}</span>
                </div>
                <div className="flex items-center gap-1.5 bg-[var(--bg)] rounded-lg p-0.5 border border-[var(--border)]">
                  <button
                    onClick={() => setAgentMode('chat')}
                    className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors ${agentMode === 'chat' ? 'bg-[var(--accent)] text-white' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}`}
                  >
                    Chat
                  </button>
                  <button
                    onClick={() => setAgentMode('agent')}
                    className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors ${agentMode === 'agent' ? 'bg-[var(--accent)] text-white' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}`}
                  >
                    Agent
                  </button>
                </div>
                {providers.length > 0 && (() => {
                  const activeProvider = selectedProvider || defaultProvider;
                  const dotColor = PROVIDER_COLORS[activeProvider] || '#8B5CF6';
                  return (
                  <div className="relative" ref={providerDropdownRef}>
                    <button
                      onClick={() => setProviderDropdownOpen(!providerDropdownOpen)}
                      className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-[11px] font-medium bg-[var(--bg)] border border-[var(--border)] text-[var(--text-primary)] hover:border-[var(--text-tertiary)] transition-colors cursor-pointer"
                      title="Select AI model/provider"
                    >
                      <span className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: dotColor }} />
                      <span className="max-w-[100px] truncate capitalize">{activeProvider || 'Select'}</span>
                      <svg className={`w-3 h-3 text-[var(--text-tertiary)] transition-transform ${providerDropdownOpen ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" /></svg>
                    </button>
                    {providerDropdownOpen && (
                      <div className="absolute right-0 top-full mt-1 w-52 bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-lg z-50 py-1">
                        {/* Default option */}
                        <button
                          onClick={() => { setSelectedProvider(''); try { localStorage.setItem('pepa-ai-provider', ''); } catch { /* ignore */ } setProviderDropdownOpen(false); }}
                          className={`w-full flex items-center gap-2.5 px-3 py-2 text-left text-[12px] hover:bg-[var(--bg)] transition-colors ${!selectedProvider ? 'bg-[var(--bg)]' : ''}`}
                        >
                          <span className="w-2 h-2 rounded-full bg-[var(--accent)]" />
                          <span className="flex-1 text-[var(--text-primary)] font-medium truncate">{defaultProvider || 'System default'}</span>
                          <span className="text-[10px] text-[var(--accent)] font-medium shrink-0">Default</span>
                        </button>
                        <div className="my-1 border-t border-[var(--border-light)]" />
                        {providers.map(p => (
                          <button
                            key={p.name}
                            onClick={() => { setSelectedProvider(p.name); try { localStorage.setItem('pepa-ai-provider', p.name); } catch { /* ignore */ } setProviderDropdownOpen(false); }}
                            className={`w-full flex items-center gap-2.5 px-3 py-2 text-left text-[12px] hover:bg-[var(--bg)] transition-colors ${selectedProvider === p.name ? 'bg-[var(--bg)]' : ''}`}
                          >
                            <span className="w-2 h-2 rounded-full shrink-0" style={{ backgroundColor: PROVIDER_COLORS[p.name] || '#8B5CF6' }} />
                            <span className="flex-1 text-[var(--text-primary)] capitalize truncate">{p.name}</span>
                            {!p.available && <span className="text-[10px] text-red-400 shrink-0">offline</span>}
                            {p.name === defaultProvider && <span className="text-[10px] text-[var(--text-tertiary)] shrink-0">default</span>}
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                  );
                })()}
              </div>
              <div className="flex-1 overflow-y-auto p-4 space-y-4 min-h-0">
                {messages.map((msg, i) => (
                  <div key={i} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                    <div className={`max-w-[85%] rounded-xl px-4 py-3 text-[13px] leading-relaxed ${
                      msg.role === 'user'
                        ? 'bg-[var(--accent)] text-white'
                        : 'bg-[var(--bg)] text-[var(--text-primary)] border border-[var(--border-light)]'
                    }`}>
                      {msg.role === 'assistant' ? (
                        <>
                          {/* Show tool calls */}
                          {msg.toolCalls && msg.toolCalls.length > 0 && (
                            <ToolCallsDisplay calls={msg.toolCalls} />
                          )}
                          {renderAiContent(msg.content)}
                        </>
                      ) : <span className="whitespace-pre-wrap">{msg.content}</span>}
                    </div>
                  </div>
                ))}
                {loading && (
                  <div className="flex justify-start">
                    <div className="bg-[var(--bg)] border border-[var(--border-light)] rounded-xl px-4 py-3 text-[13px] text-[var(--text-tertiary)]">
                      <span className="inline-flex items-center gap-1.5">
                        <span className="animate-bounce" style={{ animationDelay: '0ms' }}>&#9679;</span>
                        <span className="animate-bounce" style={{ animationDelay: '150ms' }}>&#9679;</span>
                        <span className="animate-bounce" style={{ animationDelay: '300ms' }}>&#9679;</span>
                        <span className="ml-1 text-[11px]">{agentMode === 'agent' ? 'Agent is thinking...' : 'Thinking...'}</span>
                      </span>
                    </div>
                  </div>
                )}
                <div ref={messagesEndRef} />
              </div>
              <div className="p-3 border-t border-[var(--border)] shrink-0">
                <div className="flex gap-2 items-end">
                  <textarea
                    ref={textareaRef}
                    value={input}
                    onChange={handleInputChange}
                    onKeyDown={handleKeyDown}
                    rows={1}
                    placeholder="Ask about services, deployments, clusters, pipelines..."
                    className="flex-1 px-3 py-2.5 border border-[var(--border)] rounded-lg text-[13px] bg-transparent text-[var(--text-primary)] focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent outline-none resize-none leading-relaxed"
                  />
                  <button onClick={handleSend} disabled={loading || !input.trim()} className="btn btn-primary btn-sm shrink-0 disabled:opacity-50">
                    {loading ? '...' : 'Send'}
                  </button>
                </div>
                <p className="text-[10px] text-[var(--text-tertiary)] mt-1.5">{agentMode === 'agent' ? 'Agent uses tools to query PEPA data. Press Enter to send.' : 'Chat mode — no tool access. Press Enter to send.'}</p>
              </div>
            </div>
            <div className="space-y-4 overflow-y-auto min-h-0">
              <div className="card">
                <div className="card-header flex items-center justify-between">
                  <span className="text-[13px] font-medium text-[var(--text-primary)]">Chats ({chats.length})</span>
                  {chats.length > 0 && (
                    <button onClick={() => setClearAllOpen(true)} className="text-[10px] text-[var(--text-tertiary)] hover:text-red-500 transition-colors">Delete all</button>
                  )}
                </div>
                <div className="p-3 space-y-1.5">
                  <button onClick={handleNewChat} className="w-full flex items-center justify-center gap-1.5 px-2.5 py-2 rounded-lg text-[12px] font-medium border border-dashed border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--accent)] hover:text-[var(--accent)] transition-colors">
                    <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
                    </svg>
                    New chat
                  </button>
                  <div className="space-y-1">
                    {[...chats].sort((a, b) => b.updatedAt - a.updatedAt).map(chat => (
                      <div
                        key={chat.id}
                        onClick={() => handleSelectChat(chat)}
                        className={`group flex items-center gap-2 px-2.5 py-2 rounded-lg cursor-pointer transition-colors ${chat.id === currentChatId ? 'bg-[var(--accent)]/10 border border-[var(--accent)]/30' : 'border border-transparent hover:bg-[var(--bg)]'}`}
                      >
                        <div className="flex-1 min-w-0">
                          <p className={`text-[12px] truncate ${chat.id === currentChatId ? 'font-medium text-[var(--text-primary)]' : 'text-[var(--text-secondary)]'}`}>{chat.title}</p>
                          <p className="text-[10px] text-[var(--text-tertiary)]">{mounted ? `${formatChatTime(chat.updatedAt)} · ${chat.messages.length} msgs` : ''}</p>
                        </div>
                        <button
                          onClick={(e) => { e.stopPropagation(); setDeleteTarget(chat); }}
                          title="Delete chat"
                          className="shrink-0 opacity-0 group-hover:opacity-100 p-1 rounded text-[var(--text-tertiary)] hover:text-red-500 hover:bg-red-500/10 transition-all"
                        >
                          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                            <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                          </svg>
                        </button>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
              <div className="card">
                <div className="card-header"><span className="text-[13px] font-medium text-[var(--text-primary)]">Quick Actions</span></div>
                <div className="p-3 space-y-1.5">
                  {quickActions.map((a, i) => (
                    <button key={i} onClick={() => { setInput(a.query); textareaRef.current?.focus(); }} className="w-full text-left px-2.5 py-2 rounded-lg text-[12px] text-[var(--text-secondary)] hover:bg-[var(--bg)] hover:text-[var(--text-primary)] transition-colors">{a.label}</button>
                  ))}
                </div>
              </div>
              <div className="card">
                <button
                  onClick={() => setSysInstrOpen(!sysInstrOpen)}
                  className="card-header w-full flex items-center justify-between cursor-pointer"
                >
                  <span className="text-[13px] font-medium text-[var(--text-primary)]">System Instruction</span>
                  <svg className={`w-3.5 h-3.5 text-[var(--text-tertiary)] transition-transform ${sysInstrOpen ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
                  </svg>
                </button>
                {sysInstrOpen && (
                  <div className="px-3 pb-3 space-y-2">
                    <textarea
                      value={systemInstruction}
                      onChange={e => {
                        setSystemInstruction(e.target.value);
                        try { localStorage.setItem(AI_SYSTEM_INSTRUCTION_LS_KEY, e.target.value); } catch { /* ignore */ }
                      }}
                      placeholder="Ты — умный ИИ-агент. Твоя задача — помогать пользователю, отвечать четко, по делу и предлагать решения проблем."
                      rows={4}
                      className="w-full px-2.5 py-2 border border-[var(--border)] rounded-lg text-[12px] bg-transparent text-[var(--text-primary)] focus:ring-1 focus:ring-[var(--accent)] focus:border-transparent outline-none resize-y leading-relaxed"
                    />
                    {systemInstruction && (
                      <button
                        onClick={() => {
                          setSystemInstruction('');
                          try { localStorage.removeItem(AI_SYSTEM_INSTRUCTION_LS_KEY); } catch { /* ignore */ }
                        }}
                        className="text-[10px] text-[var(--text-tertiary)] hover:text-red-500 transition-colors"
                      >
                        Clear instruction
                      </button>
                    )}
                    <p className="text-[10px] text-[var(--text-tertiary)]">Custom prompt that overrides the default agent behavior. Leave empty for default.</p>
                  </div>
                )}
              </div>
              {suggestions.length > 0 && (
                <div className="card">
                  <div className="card-header"><span className="text-[13px] font-medium text-[var(--text-primary)]">Suggestions</span></div>
                  <div className="p-3 space-y-2">
                    {suggestions.slice(0, 4).map((s, i) => (
                      <div key={i} className="px-2.5 py-2 rounded-lg bg-[var(--bg)]">
                        <p className="text-[12px] font-medium text-[var(--text-primary)]">{String(s.title)}</p>
                        <p className="text-[11px] text-[var(--text-tertiary)]">{String(s.description)}</p>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Generate Tab */}
        {tab === 'generate' && (
          <div className="flex-1 overflow-y-auto pt-4 space-y-4 page-animate-up page-delay-2">
            <div className="card">
              <div className="card-header"><span className="text-[13px] font-medium text-[var(--text-primary)]">Generate Configuration</span></div>
              <div className="card-body space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="label">Type</label>
                    <select value={genType} onChange={e => setGenType(e.target.value)} className="select">
                      <option value="pepa_service_blueprint">Service Blueprint</option>
                      <option value="pepa_workflow">Automation Workflow</option>
                      <option value="pepa_scorecard">Scorecard</option>
                      <option value="pepa_pipeline_source">Pipeline Source</option>
                      <option value="pepa_rbac_policy">RBAC Policy</option>
                      <option value="pepa_gitops">GitOps Repository</option>
                    </select>
                    {genType && typeDescriptions[genType] && (
                      <p className="text-[11px] text-[var(--text-tertiary)] mt-1">{typeDescriptions[genType]}</p>
                    )}
                  </div>
                  <div><label className="label">Service Name</label><input value={genName} onChange={e => setGenName(e.target.value)} placeholder="e.g. user-api" className="input" /></div>
                </div>
                <button onClick={handleGenerate} disabled={loading} className="btn btn-primary btn-sm">{loading ? 'Generating...' : 'Generate'}</button>
              </div>
            </div>
            {genResult && (
              <div className="card">
                <div className="card-header flex items-center justify-between">
                  <span className="text-[13px] font-medium text-[var(--text-primary)]">Generated {genType}</span>
                  <div className="flex gap-2">
                    <button onClick={() => { navigator.clipboard.writeText(genResult); setToast({ message: 'Copied!', type: 'success' }); }} className="btn btn-secondary btn-sm">Copy</button>
                    <button onClick={async () => {
                      setApplying(true);
                      try {
                        const res = await aiExtended.apply({ type: genType, content: genResult });
                        setToast({ message: res.message || 'Created!', type: 'success' });
                        setGenResult('');
                        setGenName('');
                        const redirectMap: Record<string, string> = {
                          pepa_service_blueprint: '/pipeline-blueprints',
                          pepa_workflow: '/workflows',
                          pepa_scorecard: '/scorecards',
                          pepa_pipeline_source: '/pipelines',
                          pepa_rbac_policy: '/roles',
                          pepa_gitops: '/gitops',
                        };
                        setTimeout(() => router.push(redirectMap[genType] || '/'), 800);
                      } catch (err) {
                        setToast({ message: err instanceof Error ? err.message : 'Failed to apply', type: 'error' });
                      } finally {
                        setApplying(false);
                      }
                    }} disabled={applying} className="btn btn-primary btn-sm">
                      {applying ? 'Applying...' : 'Apply to PEPA'}
                    </button>
                  </div>
                </div>
                <div className="p-4"><pre className="text-[12px] font-mono text-[var(--text-primary)] bg-[var(--bg)] rounded-lg p-4 overflow-x-auto whitespace-pre">{genResult}</pre></div>
              </div>
            )}
          </div>
        )}

        {/* Confirm dialogs */}
        <ConfirmModal
          open={deleteTarget !== null}
          title="Delete chat?"
          description={`"${deleteTarget?.title || ''}" and all its messages will be permanently deleted.`}
          confirmLabel="Delete"
          variant="danger"
          onConfirm={() => deleteTarget && handleDeleteChat(deleteTarget)}
          onCancel={() => setDeleteTarget(null)}
        />
        <ConfirmModal
          open={clearAllOpen}
          title="Delete all chats?"
          description="All chats and their messages will be permanently deleted. This cannot be undone."
          confirmLabel="Delete all"
          variant="danger"
          onConfirm={handleClearAllChats}
          onCancel={() => setClearAllOpen(false)}
        />

        {/* History Tab */}
        {tab === 'history' && (
          <div className="flex-1 overflow-y-auto pt-4 page-animate-up page-delay-2">
            <div className="card" style={{ borderRadius: '12px' }}>
              <div className="card-header"><span className="text-[13px] font-medium text-[var(--text-primary)]">Interaction History ({history.length})</span></div>
              <div className="divide-y divide-[var(--border-light)]">
                {history.map((h, i) => (
                  <div key={i} className="px-4 py-3">
                    <div className="flex items-center gap-2 mb-1">
                      <span className={`text-[10px] px-1.5 py-0.5 rounded ${String(h.type) === 'chat' ? 'bg-blue-500/10 text-blue-500' : String(h.type) === 'analyze' ? 'bg-red-500/10 text-red-500' : String(h.type) === 'recommend' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-violet-500/10 text-violet-500'}`}>{String(h.type)}</span>
                      <span className="text-[11px] text-[var(--text-tertiary)]">{mounted ? new Date(String(h.timestamp)).toLocaleString() : String(h.timestamp)}</span>
                      {Boolean(h.tools_used) && <span className="text-[10px] px-1.5 py-0.5 rounded bg-violet-500/10 text-violet-500">{String(h.tools_used)} tools</span>}
                    </div>
                    <p className="text-[12px] text-[var(--text-primary)] font-medium">{String(h.query)}</p>
                    <p className="text-[11px] text-[var(--text-secondary)] mt-1 line-clamp-2">{String(h.response).substring(0, 200)}...</p>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// ── Tool Calls Display ────────────────────────────────────────────────────────

function ToolCallsDisplay({ calls }: { calls: AIToolCall[] }) {
  const [expanded, setExpanded] = useState(false);
  if (calls.length === 0) return null;

  const successCount = calls.filter(c => !c.error).length;
  const errorCount = calls.filter(c => c.error).length;

  return (
    <div className="mb-3 rounded-lg border border-[var(--border)] overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between px-3 py-2 bg-[var(--border-light)] text-[11px] text-[var(--text-secondary)] hover:bg-[var(--border)] transition-colors"
      >
        <span className="flex items-center gap-1.5">
          <svg className="w-3.5 h-3.5 text-[var(--accent)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
          </svg>
          Agent used {calls.length} tool{calls.length > 1 ? 's' : ''}
          {successCount > 0 && <span className="text-emerald-500">({successCount} ok</span>}
          {errorCount > 0 && <span className="text-red-500">{successCount > 0 ? ', ' : '('}{errorCount} err)</span>}
          {successCount > 0 && errorCount === 0 && <span>)</span>}
        </span>
        <span className="text-[10px]">{expanded ? 'Hide' : 'Show'}</span>
      </button>
      {expanded && (
        <div className="divide-y divide-[var(--border-light)]">
          {calls.map((call, i) => (
            <div key={i} className="px-3 py-2 text-[11px]">
              <div className="flex items-center gap-2 mb-1">
                <span className={`font-mono font-medium ${call.error ? 'text-red-500' : 'text-emerald-500'}`}>
                  {call.error ? '\u2717' : '\u2713'}
                </span>
                <span className="font-mono text-[var(--text-primary)]">{call.tool_name}</span>
                <span className={`text-[9px] px-1 py-0.5 rounded ${
                  call.policy === 'observe' ? 'bg-blue-500/10 text-blue-500' :
                  call.policy === 'require_approval' ? 'bg-orange-500/10 text-orange-500' :
                  call.policy === 'forbidden' ? 'bg-red-500/10 text-red-500' :
                  'bg-emerald-500/10 text-emerald-500'
                }`}>{call.policy}</span>
              </div>
              {call.error && (
                <p className="text-red-400 text-[10px]">{call.error}</p>
              )}
              {call.result && !call.error && (
                <details className="mt-1">
                  <summary className="text-[10px] text-[var(--text-tertiary)] cursor-pointer hover:text-[var(--text-secondary)]">
                    View result ({call.result.length} chars)
                  </summary>
                  <pre className="mt-1 text-[10px] font-mono bg-[var(--bg)] rounded p-2 overflow-auto max-h-[200px] whitespace-pre-wrap text-[var(--text-secondary)]">
                    {truncateResult(call.result, 1000)}
                  </pre>
                </details>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function truncateResult(s: string, maxLen: number): string {
  if (s.length <= maxLen) return s;
  return s.substring(0, maxLen) + '\n... (truncated)';
}

// ── AI Markdown Renderer ──────────────────────────────────────────────────────

function renderAiContent(content: string) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        code: CodeBlock as never,
        pre: ({ children }) => <>{children}</>,
        p: ({ children }) => <p className="mb-2.5 last:mb-0 leading-relaxed">{children}</p>,
        ul: ({ children }) => <ul className="mb-2.5 last:mb-0 space-y-1 list-disc list-inside">{children}</ul>,
        ol: ({ children }) => <ol className="mb-2.5 last:mb-0 space-y-1 list-decimal list-inside">{children}</ol>,
        li: ({ children }) => <li className="leading-relaxed">{children}</li>,
        h1: ({ children }) => <h1 className="text-[16px] font-bold mb-2 mt-3 first:mt-0">{children}</h1>,
        h2: ({ children }) => <h2 className="text-[15px] font-semibold mb-2 mt-3 first:mt-0">{children}</h2>,
        h3: ({ children }) => <h3 className="text-[14px] font-semibold mb-1.5 mt-2 first:mt-0">{children}</h3>,
        h4: ({ children }) => <h4 className="text-[13px] font-semibold mb-1 mt-2 first:mt-0">{children}</h4>,
        a: ({ href, children }) => <a href={href} target="_blank" rel="noopener noreferrer" className="text-[var(--accent)] hover:underline">{children}</a>,
        strong: ({ children }) => <strong className="font-semibold">{children}</strong>,
        blockquote: ({ children }) => <blockquote className="border-l-2 border-[var(--accent)] pl-3 my-2 text-[var(--text-secondary)] italic">{children}</blockquote>,
        hr: () => <hr className="my-3 border-[var(--border)]" />,
        table: ({ children }) => (
          <div className="overflow-x-auto my-2.5">
            <table className="min-w-full text-[12px] border border-[var(--border)] rounded-lg">{children}</table>
          </div>
        ),
        thead: ({ children }) => <thead className="bg-[var(--border-light)]">{children}</thead>,
        th: ({ children }) => <th className="border border-[var(--border)] px-3 py-1.5 text-left font-semibold">{children}</th>,
        td: ({ children }) => <td className="border border-[var(--border)] px-3 py-1.5">{children}</td>,
      }}
    >
      {content}
    </ReactMarkdown>
  );
}

// ── Code Block with Copy Button ───────────────────────────────────────────────

function CodeBlock({ className, children, ...props }: {
  className?: string;
  children?: ReactNode;
  node?: unknown;
  [key: string]: unknown;
}) {
  const match = /language-(\w+)/.exec(className || '');
  const codeText = String(children).replace(/\n$/, '');
  const isInline = !match && !codeText.includes('\n');
  const lang = match?.[1] || 'code';
  const isLong = codeText.split('\n').length > 5;
  const [copied, setCopied] = useState(false);
  const [isOpen, setIsOpen] = useState(false);

  if (isInline) {
    return (
      <code
        className="bg-[var(--border-light)] text-[var(--text-primary)] px-1.5 py-0.5 rounded text-[12px] font-mono"
        {...props}
      >
        {children}
      </code>
    );
  }

  const handleCopy = async () => {
    await navigator.clipboard.writeText(codeText);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="my-3 rounded-lg overflow-hidden border border-[var(--border)]">
      <div className="flex items-center justify-between px-3 py-1.5 bg-[var(--border-light)]">
        <span className="text-[10px] font-mono text-[var(--text-tertiary)] uppercase">{lang}</span>
        <button
          onClick={handleCopy}
          className="text-[11px] text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-colors flex items-center gap-1"
        >
          {copied ? (
            <>
              <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
              </svg>
              Copied
            </>
          ) : (
            <>
              <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
              </svg>
              Copy
            </>
          )}
        </button>
      </div>
      {isLong ? (
        <details open={isOpen} onToggle={e => setIsOpen((e.target as HTMLDetailsElement).open)}>
          <summary className="px-4 py-1.5 text-[11px] text-[var(--text-tertiary)] cursor-pointer hover:text-[var(--text-secondary)] select-none bg-[#1e1e2e]">
            {isOpen ? 'Hide code' : 'Show code'}
          </summary>
          {isOpen && (
            <pre className="p-4 overflow-x-auto text-[12px] leading-relaxed bg-[#1e1e2e] text-[#cdd6f4] font-mono">
              <code {...props}>{codeText}</code>
            </pre>
          )}
        </details>
      ) : (
        <pre className="p-4 overflow-x-auto text-[12px] leading-relaxed bg-[#1e1e2e] text-[#cdd6f4] font-mono">
          <code {...props}>{codeText}</code>
        </pre>
      )}
    </div>
  );
}
