'use client';

import { useState, useEffect, useRef } from 'react';
import { login, bootstrapActivate, getBootstrapStatus, resetMyPassword, getMe, setStoredUser } from '@/lib/api';

type Phase = 'loading' | 'login' | 'bootstrap' | 'change-password' | 'connecting';

export default function LoginPage() {
  const [phase, setPhase] = useState<Phase>('loading');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [bootstrapToken, setBootstrapToken] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  // Change password state
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');

  // Refs to read actual DOM values — guards against browser autofill
  // not triggering React's onChange (controlled input desync).
  const emailRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const newPasswordRef = useRef<HTMLInputElement>(null);
  const confirmPasswordRef = useRef<HTMLInputElement>(null);

  // Check bootstrap status on mount — keep retrying if API is unreachable
  // so we never accidentally show the login form on first-run.
  useEffect(() => {
    let cancelled = false;
    let retries = 0;
    const maxRetries = 10; // up to ~30s total with getBootstrapStatus's internal retries

    const check = async () => {
      try {
        const { needed, in_progress } = await getBootstrapStatus();
        if (cancelled) return;
        if (needed || in_progress) {
          setPhase('bootstrap');
        } else {
          setPhase('login');
        }
      } catch {
        if (cancelled) return;
        retries++;
        if (retries >= maxRetries) {
          setPhase('connecting');
        } else {
          // Retry after a short delay
          setTimeout(() => check(), 2000);
        }
      }
    };
    check();
    return () => { cancelled = true; };
  }, []);

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      // Use ref values as fallback — browser autofill may not trigger
      // React's onChange, leaving state out of sync with the DOM.
      const actualEmail = emailRef.current?.value || email;
      const actualPassword = passwordRef.current?.value || password;
      const data = await login(actualEmail, actualPassword);
      if (data.must_change_password) {
        // Store the current password for the change-password step
        setCurrentPassword(actualPassword);
        setPhase('change-password');
        return;
      }
      // Notify layout components (TopBar, Sidebar) that auth state changed
      window.dispatchEvent(new Event('pepa:auth-changed'));
      // Use window.location for a clean full-page navigation after login.
      // This avoids race conditions between router.push() and router.refresh()
      // that can cause the dashboard to load before the auth cookie is ready.
      window.location.href = '/';
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  const handleBootstrap = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    // Use ref values as fallback — browser autofill may not trigger
    // React's onChange, leaving state out of sync with the DOM.
    const actualNew = newPasswordRef.current?.value || newPassword;
    const actualConfirm = confirmPasswordRef.current?.value || confirmPassword;

    if (!actualNew) {
      setError('Password is required');
      return;
    }
    if (actualNew !== actualConfirm) {
      setError('Passwords do not match');
      return;
    }

    setLoading(true);
    try {
      await bootstrapActivate(bootstrapToken, actualNew);
      // Refresh user data from server
      try {
        const { user, roles } = await getMe();
        setStoredUser({ id: user.id, email: user.email, name: user.name, roles });
      } catch { /* non-critical — bootstrapActivate already updated stored user */ }
      window.dispatchEvent(new Event('pepa:auth-changed'));
      window.location.href = '/';
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Activation failed');
    } finally {
      setLoading(false);
    }
  };

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    // Use ref values as fallback — browser autofill may not trigger
    // React's onChange, leaving state out of sync with the DOM.
    const actualNew = newPasswordRef.current?.value || newPassword;
    const actualConfirm = confirmPasswordRef.current?.value || confirmPassword;

    if (actualNew !== actualConfirm) {
      setError('Passwords do not match');
      return;
    }

    setLoading(true);
    try {
      // In bootstrap mode, currentPassword is empty (skipped server-side)
      await resetMyPassword(currentPassword, actualNew);
      // Refresh user data from server to ensure stored info is accurate
      try {
        const { user, roles } = await getMe();
        setStoredUser({ id: user.id, email: user.email, name: user.name, roles });
      } catch { /* non-critical — resetMyPassword already updated stored user */ }
      window.dispatchEvent(new Event('pepa:auth-changed'));
      window.location.href = '/';
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to change password');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex login-page-dark">
      {/* ── Left panel: branding ─────────────────────────────── */}
      <div className="hidden lg:flex lg:w-1/2 relative overflow-hidden login-brand-panel">
        {/* Animated mesh background */}
        <div className="absolute inset-0 login-mesh-bg" />
        {/* Floating geometric shapes */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div className="login-float-shape login-float-1" />
          <div className="login-float-shape login-float-2" />
          <div className="login-float-shape login-float-3" />
        </div>

        <div className="relative z-10 flex flex-col justify-between p-12 w-full">
          {/* Logo + wordmark */}
          <div className="flex items-center gap-3">
            <div className="w-10 h-10">
              <svg viewBox="0 0 64 64" fill="none" xmlns="http://www.w3.org/2000/svg">
                <defs>
                  <linearGradient id="lg-bg" x1="0" y1="0" x2="64" y2="64" gradientUnits="userSpaceOnUse">
                    <stop offset="0%" stopColor="#0052CC"/>
                    <stop offset="100%" stopColor="#6E56CF"/>
                  </linearGradient>
                </defs>
                <rect x="2" y="2" width="60" height="60" rx="14" fill="url(#lg-bg)"/>
                <rect x="2" y="2" width="60" height="30" rx="14" fill="#fff" opacity="0.06"/>
                <path d="M20 46V18h14a10 10 0 010 20H20" stroke="#fff" strokeOpacity="0.9" strokeWidth="4.5" strokeLinecap="round" strokeLinejoin="round" fill="none"/>
                <circle cx="44" cy="42" r="3.5" fill="#fff" fillOpacity="0.2"/>
                <circle cx="54" cy="34" r="3" fill="#fff" fillOpacity="0.15"/>
                <circle cx="54" cy="50" r="3" fill="#fff" fillOpacity="0.15"/>
                <line x1="47" y1="41" x2="51.5" y2="35.5" stroke="#fff" strokeWidth="1" strokeLinecap="round" opacity="0.25"/>
                <line x1="47" y1="43" x2="51.5" y2="48.5" stroke="#fff" strokeWidth="1" strokeLinecap="round" opacity="0.25"/>
              </svg>
            </div>
            <span className="text-xl font-semibold text-white tracking-tight">PEPA</span>
          </div>

          {/* Hero text */}
          <div className="max-w-md">
            <h2 className="text-3xl font-bold text-white leading-tight tracking-tight">
              Platform Engineering<br />& Pipeline Automator
            </h2>
            <p className="mt-4 text-base text-white/60 leading-relaxed">
              Your internal developer portal for managing services, workflows, and platform operations — all in one place.
            </p>
          </div>

          {/* Feature pills */}
          <div className="flex flex-wrap gap-3">
            {[
              { icon: 'M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4', label: 'Service Catalog' },
              { icon: 'M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12', label: 'CI/CD Pipelines' },
              { icon: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z', label: 'RBAC & Security' },
              { icon: 'M13 10V3L4 14h7v7l9-11h-7z', label: 'GitOps Workflows' },
            ].map((f) => (
              <div key={f.label} className="flex items-center gap-2 px-3 py-2 rounded-lg bg-white/[0.06] border border-white/[0.08] backdrop-blur-sm">
                <svg className="w-4 h-4 text-white/50" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d={f.icon} />
                </svg>
                <span className="text-sm text-white/70">{f.label}</span>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* ── Right panel: form ────────────────────────────────── */}
      <div className="flex-1 flex items-center justify-center p-6 sm:p-12 relative bg-[#0d1520]">
        {/* Subtle background for right side */}
        <div className="absolute inset-0 login-mesh-bg opacity-40" />

        <div className="w-full max-w-sm relative z-10 page-animate">
          {/* Mobile logo (visible on small screens only) */}
          <div className="flex items-center gap-2.5 mb-8 lg:hidden">
            <div className="w-8 h-8">
              <svg viewBox="0 0 64 64" fill="none" xmlns="http://www.w3.org/2000/svg">
                <defs>
                  <linearGradient id="mg-bg" x1="0" y1="0" x2="64" y2="64" gradientUnits="userSpaceOnUse">
                    <stop offset="0%" stopColor="#0052CC"/>
                    <stop offset="100%" stopColor="#6E56CF"/>
                  </linearGradient>
                </defs>
                <rect x="2" y="2" width="60" height="60" rx="14" fill="url(#mg-bg)"/>
                <rect x="2" y="2" width="60" height="30" rx="14" fill="#fff" opacity="0.06"/>
                <path d="M20 46V18h14a10 10 0 010 20H20" stroke="#fff" strokeOpacity="0.9" strokeWidth="4.5" strokeLinecap="round" strokeLinejoin="round" fill="none"/>
                <circle cx="44" cy="42" r="3.5" fill="#fff" fillOpacity="0.2"/>
                <circle cx="54" cy="34" r="3" fill="#fff" fillOpacity="0.15"/>
                <circle cx="54" cy="50" r="3" fill="#fff" fillOpacity="0.15"/>
                <line x1="47" y1="41" x2="51.5" y2="35.5" stroke="#fff" strokeWidth="1" strokeLinecap="round" opacity="0.25"/>
                <line x1="47" y1="43" x2="51.5" y2="48.5" stroke="#fff" strokeWidth="1" strokeLinecap="round" opacity="0.25"/>
              </svg>
            </div>
            <span className="text-lg font-semibold text-white tracking-tight">PEPA</span>
          </div>

          {/* Desktop heading */}
          <div className="hidden lg:block mb-8">
            <h1 className="text-xl font-semibold text-white tracking-tight">
              {phase === 'bootstrap' ? 'Activate your account' : phase === 'change-password' ? 'Set your password' : phase === 'connecting' ? 'Connecting...' : 'Welcome back'}
            </h1>
            <p className="text-sm text-white/50 mt-1">
              {phase === 'bootstrap' ? 'Enter the bootstrap token to get started.' : phase === 'change-password' ? 'Choose a strong password to secure your account.' : phase === 'connecting' ? 'Cannot reach the server. Check that it is running.' : 'Sign in to your PEPA account.'}
            </p>
          </div>

          {/* Loading state */}
          {phase === 'loading' && (
            <div className="flex items-center justify-center py-12">
              <div className="w-6 h-6 border-2 border-white/30 border-t-white rounded-full animate-spin" />
            </div>
          )}

          {/* Connecting / API unreachable state */}
          {phase === 'connecting' && (
            <div className="flex flex-col items-center justify-center py-8 space-y-4">
              <div className="w-8 h-8 border-2 border-amber-400/30 border-t-amber-400 rounded-full animate-spin" />
              <p className="text-sm text-white/50 text-center">Waiting for the PEPA API to become available...</p>
              <button
                onClick={() => { setPhase('loading'); window.location.reload(); }}
                className="text-sm text-white/60 hover:text-white underline underline-offset-2"
              >
                Reload page
              </button>
            </div>
          )}

          {/* Bootstrap token form */}
          {phase === 'bootstrap' && (
            <form onSubmit={handleBootstrap} className="space-y-5">
              <div className="bg-amber-500/10 border border-amber-500/20 text-amber-400 px-4 py-3 rounded-lg text-sm">
                <div className="flex items-start gap-2.5">
                  <svg className="w-5 h-5 shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
                  </svg>
                  <div>
                    <p className="font-medium">First-time setup</p>
                    <p className="mt-1 text-xs opacity-80">
                      Enter the bootstrap token from the server console to activate the admin account.
                      This token expires in 1 hour.
                    </p>
                  </div>
                </div>
              </div>

              {error && (
                <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded-lg text-sm">
                  {error}
                </div>
              )}

              <div>
                <label htmlFor="token" className="block text-sm font-medium text-white/60 mb-1.5">
                  Bootstrap Token
                </label>
                <input
                  id="token"
                  type="text"
                  value={bootstrapToken}
                  onChange={(e) => setBootstrapToken(e.target.value.trim())}
                  placeholder="Paste the token from server console..."
                  required
                  className="w-full px-3.5 py-2.5 border border-white/10 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-[#0052CC] focus:border-transparent font-mono bg-white/5 text-white placeholder-white/30"
                />
              </div>

              <div>
                <label htmlFor="newPassword" className="block text-sm font-medium text-white/60 mb-1.5">
                  New Password
                </label>
                <input
                  id="newPassword"
                  name="newPassword"
                  type="password"
                  ref={newPasswordRef}
                  autoComplete="new-password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  placeholder="Choose a strong password..."
                  required
                  className="w-full px-3.5 py-2.5 border border-white/10 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-[#0052CC] focus:border-transparent bg-white/5 text-white placeholder-white/30"
                />
              </div>

              <div>
                <label htmlFor="confirmPassword" className="block text-sm font-medium text-white/60 mb-1.5">
                  Confirm Password
                </label>
                <input
                  id="confirmPassword"
                  name="confirmPassword"
                  type="password"
                  ref={confirmPasswordRef}
                  autoComplete="new-password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  placeholder="Confirm your password..."
                  required
                  className="w-full px-3.5 py-2.5 border border-white/10 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-[#0052CC] focus:border-transparent bg-white/5 text-white placeholder-white/30"
                />
              </div>

              <button
                type="submit"
                disabled={loading}
                className="w-full bg-[var(--accent)] text-white py-2.5 px-4 rounded-lg text-sm font-medium hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-opacity"
              >
                {loading ? 'Activating...' : 'Activate Admin Account'}
              </button>

              <p className="text-xs text-white/30 text-center">
                The token was printed in the server console at startup.
              </p>
            </form>
          )}

          {/* Normal login form */}
          {phase === 'login' && (
            <form onSubmit={handleLogin} className="space-y-5">
              {error && (
                <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded-lg text-sm">
                  {error}
                </div>
              )}

              <div>
                <label htmlFor="email" className="block text-sm font-medium text-white/60 mb-1.5">
                  Email
                </label>
                <div className="relative">
                  <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-white/30" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75" />
                  </svg>
                  <input
                    id="email"
                    name="email"
                    type="email"
                    ref={emailRef}
                    autoComplete="username"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="admin@local"
                    required
                    className="w-full pl-10 pr-3.5 py-2.5 border border-white/10 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-[#0052CC] focus:border-transparent bg-white/5 text-white placeholder-white/30"
                  />
                </div>
              </div>

              <div>
                <label htmlFor="password" className="block text-sm font-medium text-white/60 mb-1.5">
                  Password
                </label>
                <div className="relative">
                  <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-white/30" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
                  </svg>
                  <input
                    id="password"
                    name="password"
                    type="password"
                    ref={passwordRef}
                    autoComplete="current-password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                    className="w-full pl-10 pr-3.5 py-2.5 border border-white/10 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-[#0052CC] focus:border-transparent bg-white/5 text-white placeholder-white/30"
                  />
                </div>
              </div>

              <button
                type="submit"
                disabled={loading}
                className="w-full bg-[var(--accent)] text-white py-2.5 px-4 rounded-lg text-sm font-medium hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-opacity"
              >
                {loading ? (
                  <span className="flex items-center justify-center gap-2">
                    <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                    Signing in...
                  </span>
                ) : 'Sign in'}
              </button>
            </form>
          )}

          {/* Change password form */}
          {phase === 'change-password' && (
            <form onSubmit={handleChangePassword} className="space-y-5">
              <div className="bg-amber-500/10 border border-amber-500/20 text-amber-400 px-4 py-3 rounded-lg text-sm">
                <div className="flex items-start gap-2.5">
                  <svg className="w-5 h-5 shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
                  </svg>
                  <div>
                    <p className="font-medium">Set your password</p>
                    <p className="mt-1 text-xs opacity-80">
                      You are using a temporary/bootstrap credential. Please set a strong password to secure your account.
                      The password must contain at least 8 characters with uppercase, lowercase, digit, and special character.
                    </p>
                  </div>
                </div>
              </div>

              {error && (
                <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded-lg text-sm">
                  {error}
                </div>
              )}

              <div>
                <label htmlFor="newPassword" className="block text-sm font-medium text-white/60 mb-1.5">
                  New Password
                </label>
                <div className="relative">
                  <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-white/30" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
                  </svg>
                  <input
                    id="newPassword"
                    name="new-password"
                    type="password"
                    ref={newPasswordRef}
                    autoComplete="new-password"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    required
                    className="w-full pl-10 pr-3.5 py-2.5 border border-white/10 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-[#0052CC] focus:border-transparent bg-white/5 text-white placeholder-white/30"
                  />
                </div>
              </div>

              <div>
                <label htmlFor="confirmPassword" className="block text-sm font-medium text-white/60 mb-1.5">
                  Confirm Password
                </label>
                <div className="relative">
                  <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-white/30" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
                  </svg>
                  <input
                    id="confirmPassword"
                    name="confirm-password"
                    type="password"
                    ref={confirmPasswordRef}
                    autoComplete="new-password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    required
                    className="w-full pl-10 pr-3.5 py-2.5 border border-white/10 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-[#0052CC] focus:border-transparent bg-white/5 text-white placeholder-white/30"
                  />
                </div>
              </div>

              <button
                type="submit"
                disabled={loading}
                className="w-full bg-[var(--accent)] text-white py-2.5 px-4 rounded-lg text-sm font-medium hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-opacity"
              >
                {loading ? (
                  <span className="flex items-center justify-center gap-2">
                    <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                    Setting password...
                  </span>
                ) : 'Set Password & Continue'}
              </button>
            </form>
          )}

          {/* Footer */}
          <div className="mt-8 pt-6 border-t border-white/10">
            <p className="text-xs text-white/30 text-center">
              PEPA &mdash; Platform Engineering & Pipeline Automator
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
