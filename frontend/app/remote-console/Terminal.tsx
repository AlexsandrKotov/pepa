'use client';

import { useEffect, useRef, useState } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import '@xterm/xterm/css/xterm.css';
import { getBase } from '@/lib/api';

interface SSHHost {
  id: string;
  name: string;
  hostname: string;
  port: number;
  username: string;
  auth_method: string;
}

interface TerminalProps {
  host: SSHHost;
  password?: string;
  onDisconnect: () => void;
  onReconnect: () => void;
}

export function Terminal({ host, password, onDisconnect, onReconnect }: TerminalProps) {
  const termRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!termRef.current) return;

    // Initialize xterm
    const xterm = new XTerm({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: '"JetBrains Mono", "Fira Code", "Cascadia Code", Menlo, Monaco, monospace',
      scrollback: 10000,
      theme: {
        background: '#1a1b26',
        foreground: '#a9b1d6',
        cursor: '#c0caf5',
        selectionBackground: '#33467c',
        black: '#15161e',
        red: '#f7768e',
        green: '#9ece6a',
        yellow: '#e0af68',
        blue: '#7aa2f7',
        magenta: '#bb9af7',
        cyan: '#7dcfff',
        white: '#a9b1d6',
        brightBlack: '#414868',
        brightRed: '#f7768e',
        brightGreen: '#9ece6a',
        brightYellow: '#e0af68',
        brightBlue: '#7aa2f7',
        brightMagenta: '#bb9af7',
        brightCyan: '#7dcfff',
        brightWhite: '#c0caf5',
      },
      allowProposedApi: true,
    });

    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();

    xterm.loadAddon(fitAddon);
    xterm.loadAddon(webLinksAddon);
    xterm.open(termRef.current);

    // Fit to container
    setTimeout(() => {
      fitAddon.fit();
    }, 100);

    // Write connecting message
    xterm.writeln('\x1b[36mConnecting to ' + host.hostname + '...\x1b[0m');
    xterm.writeln('');

    // Build WebSocket URL — httpOnly cookie is sent automatically by the browser
    const baseUrl = getBase().replace(/^http/, 'ws');
    const params = new URLSearchParams({
      cols: String(xterm.cols),
      rows: String(xterm.rows),
    });

    const wsUrl = `${baseUrl}/api/v1/ssh-terminal/${host.id}?${params.toString()}`;

    // Connect WebSocket
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      setConnected(true);
      setError(null);
      xterm.writeln('\x1b[32mConnected!\x1b[0m');
      xterm.writeln('');

      // For LDAP passthrough, send password as first message (never in URL)
      if (password && host.auth_method === 'ldap_passthrough') {
        ws.send(JSON.stringify({ type: 'auth', password }));
      }
    };

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        xterm.write(new Uint8Array(event.data));
      } else {
        xterm.write(event.data);
      }
    };

    ws.onerror = () => {
      setError('Connection failed');
      xterm.writeln('');
      xterm.writeln('\x1b[31mConnection error. Check your credentials and try again.\x1b[0m');
    };

    ws.onclose = (event) => {
      setConnected(false);
      if (event.code !== 1000) {
        xterm.writeln('');
        xterm.writeln('\x1b[33mSession ended.\x1b[0m');
      }
    };

    // Send terminal input to WebSocket
    xterm.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data);
      }
    });

    // Handle resize via ResizeObserver with debounce to prevent feedback loop
    // (fitAddon.fit() changes dimensions -> triggers ResizeObserver -> loop)
    let resizeRafId: number | null = null;
    let lastCols = xterm.cols;
    let lastRows = xterm.rows;

    const resizeObserver = new ResizeObserver(() => {
      if (resizeRafId !== null) return;
      resizeRafId = requestAnimationFrame(() => {
        resizeRafId = null;
        fitAddon.fit();
        // Only send resize if dimensions actually changed
        if (xterm.cols !== lastCols || xterm.rows !== lastRows) {
          lastCols = xterm.cols;
          lastRows = xterm.rows;
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({
              type: 'resize',
              cols: xterm.cols,
              rows: xterm.rows,
            }));
          }
        }
      });
    });
    if (termRef.current) {
      resizeObserver.observe(termRef.current);
    }

    // Also handle window resize as fallback
    const handleWindowResize = () => {
      fitAddon.fit();
      if (xterm.cols !== lastCols || xterm.rows !== lastRows) {
        lastCols = xterm.cols;
        lastRows = xterm.rows;
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({
            type: 'resize',
            cols: xterm.cols,
            rows: xterm.rows,
          }));
        }
      }
    };
    window.addEventListener('resize', handleWindowResize);

    // Focus terminal
    xterm.focus();

    return () => {
      window.removeEventListener('resize', handleWindowResize);
      resizeObserver.disconnect();
      if (resizeRafId !== null) cancelAnimationFrame(resizeRafId);
      ws.close();
      xterm.dispose();
    };
  }, [host.id, host.hostname, password, host.auth_method]);

  const handleReconnect = () => {
    onReconnect();
  };

  return (
    <div className="flex flex-col h-full bg-[#1a1b26]">
      {/* Terminal Header */}
      <div className="flex items-center justify-between px-4 py-2 bg-[#16161e] border-b border-[#2a2b3d]">
        <div className="flex items-center gap-3">
          <div className={`w-2 h-2 rounded-full ${connected ? 'bg-green-400 animate-pulse' : 'bg-red-400'}`} />
          <span className="text-sm font-medium text-[#a9b1d6]">
            {host.username}@{host.hostname}:{host.port}
          </span>
          <span className="text-xs text-[#565f89]">({host.name})</span>
        </div>
        <div className="flex items-center gap-2">
          {!connected && (
            <button
              onClick={handleReconnect}
              className="px-3 py-1 text-xs font-medium bg-[#33467c] text-[#7aa2f7] rounded hover:bg-[#3d59a1] transition-colors"
            >
              Reconnect
            </button>
          )}
          <button
            onClick={onDisconnect}
            className="px-3 py-1 text-xs font-medium bg-[#2a2b3d] text-[#a9b1d6] rounded hover:bg-[#3d3f5a] transition-colors"
          >
            Disconnect
          </button>
        </div>
      </div>

      {/* Error Banner */}
      {error && (
        <div className="px-4 py-2 bg-red-500/10 border-b border-red-500/20">
          <p className="text-sm text-red-400">{error}</p>
        </div>
      )}

      {/* Terminal */}
      <div ref={termRef} className="flex-1 p-2 overflow-hidden" />

      {/* Status Bar */}
      <div className="flex items-center justify-between px-4 py-1 bg-[#16161e] border-t border-[#2a2b3d] text-xs text-[#565f89]">
        <span>{connected ? 'Connected' : 'Disconnected'}</span>
        <span>xterm.js | {host.auth_method === 'ldap_passthrough' ? 'LDAP' : host.auth_method.toUpperCase()}</span>
      </div>
    </div>
  );
}
