import { useState, useEffect } from 'react';

/**
 * Detects whether the user is on macOS (including iPadOS which reports as Mac).
 * Returns `true` on Mac/iPad, `false` on Windows/Linux/Android.
 * SSR-safe — defaults to `false` until the component mounts.
 */
export default function useIsMac(): boolean {
  const [isMac, setIsMac] = useState(false);

  useEffect(() => {
    const ua = navigator.userAgent || '';
    // navigator.platform is deprecated but still widely supported;
    // fall back to userAgent check for newer browsers.
    const platform = navigator.platform || '';
    setIsMac(
      platform.startsWith('Mac') ||
      ua.includes('Macintosh') ||
      (ua.includes('iPad') && ua.includes('like Mac'))
    );
  }, []);

  return isMac;
}
