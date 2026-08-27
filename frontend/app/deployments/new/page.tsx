'use client';
import { Suspense } from 'react';
import { DeploymentsList } from '../page';

// Dedicated route for creating a new deployment — renders the create tab directly.
export default function NewDeploymentPage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center py-12"><div className="loading-spinner" /></div>}>
      <DeploymentsList autoCreate />
    </Suspense>
  );
}
