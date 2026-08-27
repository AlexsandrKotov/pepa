'use client';

import { useEffect, useState, Suspense } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import { workflows, type Workflow } from '@/lib/api';
import PipelineBuilderPage from '../new/page';

function EditPipelineContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const workflowId = searchParams.get('id') || '';
  const [loading, setLoading] = useState(true);
  const [workflow, setWorkflow] = useState<Workflow | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!workflowId) {
      setLoading(false);
      return;
    }
    workflows.get(workflowId)
      .then(wf => {
        setWorkflow(wf);
        setLoading(false);
      })
      .catch(() => {
        setError('Failed to load workflow');
        setLoading(false);
      });
  }, [workflowId]);

  if (loading) {
    return <div className="flex items-center justify-center h-screen">Loading...</div>;
  }

  if (error || !workflowId || !workflow) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-center">
          <p className="text-red-600 mb-4">{error || 'No pipeline ID provided'}</p>
          <button
            onClick={() => router.push('/automation')}
            className="px-4 py-2 bg-blue-500 text-white rounded-md hover:bg-blue-600"
          >
            Back to Automation
          </button>
        </div>
      </div>
    );
  }

  // Load the saved spec onto the canvas; save goes through PUT /workflows/:id
  return <PipelineBuilderPage initialWorkflow={workflow} />;
}

export default function EditPipelinePage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center h-screen">Loading...</div>}>
      <EditPipelineContent />
    </Suspense>
  );
}
