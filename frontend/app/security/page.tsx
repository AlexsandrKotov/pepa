'use client';

import PermissionGuard from '@/components/PermissionGuard';
import SecurityClient from './SecurityClient';

export default function SecurityPage() {
  return (
    <PermissionGuard resource="settings" action="read">
      <div className="space-y-6">
        <SecurityClient />
      </div>
    </PermissionGuard>
  );
}
