import type { Metadata } from 'next';
import './globals.css';
import ClientLayout from '@/components/ClientLayout';
import { PermissionProvider } from '@/hooks/usePermission';
import DynamicComponents from '@/components/DynamicComponents';

export const metadata: Metadata = {
  title: 'PEPA — Platform Engineering & Pipeline Automator',
  description: 'PEPA — Platform Engineering & Pipeline Automator. Managing services, workflows, and platform operations.',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    // suppressHydrationWarning: the blocking script below sets data-theme/data-glass/style
    // on <html> before hydration, so client attributes intentionally differ from SSR output.
    <html lang="en" suppressHydrationWarning>
      <head>
        {/*
          Blocking script: apply saved theme + glass state BEFORE first paint to prevent FOUC.
          SECURITY WARNING: This uses dangerouslySetInnerHTML with a STATIC string literal only.
          NEVER include user-controlled or dynamic input here — this bypasses React's XSS protection.
        */}
        <script
          dangerouslySetInnerHTML={{
            __html: `(function(){try{var t=localStorage.getItem('pepa-theme')||'light';var d=t==='system'?(window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'):t;document.documentElement.setAttribute('data-theme',d);document.documentElement.style.colorScheme=d;if(d==='dark'){document.documentElement.style.background='#1a1d24'}}catch(e){}})();(function(){try{var g=localStorage.getItem('pepa-liquid-glass');if(!g)g='on';document.documentElement.setAttribute('data-glass',g)}catch(e){}})()`,
          }}
        />
      </head>
      <body>
        <PermissionProvider>
          <ClientLayout>{children}</ClientLayout>
          <DynamicComponents />
        </PermissionProvider>
      </body>
    </html>
  );
}
