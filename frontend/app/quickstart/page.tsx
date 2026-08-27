'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { workflows, entities, services, type Workflow } from '@/lib/api';
import { friendlyError, type FriendlyError } from '@/lib/errors';

export default function QuickstartPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  
  // Section 1: Register service
  const [serviceName, setServiceName] = useState('');
  const [serviceCreated, setServiceCreated] = useState(false);
  const [serviceId, setServiceId] = useState<string | null>(null);
  const [creatingService, setCreatingService] = useState(false);
  const [serviceError, setServiceError] = useState('');
  
  // Section 2: Create pipeline
  const [pipelineCreated, setPipelineCreated] = useState(false);
  const [workflowId, setWorkflowId] = useState<string | null>(null);
  const [creatingPipeline, setCreatingPipeline] = useState(false);
  const [pipelineError, setPipelineError] = useState('');
  
  // Section 3: Run pipeline
  const [running, setRunning] = useState(false);
  const [runResult, setRunResult] = useState<{ success?: boolean; steps?: any[]; error?: string } | null>(null);
  
  // Section 4: Deploy
  const [deploying, setDeploying] = useState(false);
  const [deployResult, setDeployResult] = useState<{ success?: boolean; simulated?: boolean; message?: string } | null>(null);

  useEffect(() => {
    setLoading(false);
  }, []);

  const createService = async () => {
    if (!serviceName.trim()) return;
    setCreatingService(true);
    try {
      // Create entity
      const entity = await entities.create({
        type_key: 'service',
        name: serviceName,
        metadata: {
          description: `Service created via quickstart`,
          owner: 'quickstart-user',
        },
      });
      
      // Create service record
      const service = await services.create({
        entity_id: entity.id,
        name: serviceName,
        template_slug: 'static-site',
      });
      
      setServiceId(entity.id);
      setServiceCreated(true);
      setServiceError('');
    } catch (err) {
      const fe = friendlyError(err);
      setServiceError(fe.message);
    } finally {
      setCreatingService(false);
    }
  };

  const createPipeline = async () => {
    setCreatingPipeline(true);
    try {
      // Create a simple demo workflow
      const wf = await workflows.create({
        name: 'Hello PEPA',
        spec: {
          steps: [
            { name: 'prepare', type: 'task', params: { message: 'Preparing the demo run' } },
            { name: 'process', type: 'task', depends_on: ['prepare'], params: { message: 'Processing demo data' } },
            { name: 'finish', type: 'task', depends_on: ['process'], run_when: 'always', params: { message: 'Demo finished successfully' } },
          ],
        },
      });
      setWorkflowId(wf.id);
      setPipelineCreated(true);
      setPipelineError('');
    } catch (err) {
      const fe = friendlyError(err);
      setPipelineError(fe.message);
    } finally {
      setCreatingPipeline(false);
    }
  };

  const runPipeline = async () => {
    if (!workflowId) return;
    setRunning(true);
    setRunResult(null);
    try {
      await workflows.execute(workflowId);
      
      // Poll for completion
      for (let i = 0; i < 20; i++) {
        await new Promise(r => setTimeout(r, 1000));
        const execs = await workflows.executions(workflowId);
        const latest = execs.executions?.[0];
        
        if (latest && (latest.status === 'success' || latest.status === 'failed')) {
          const steps = await workflows.stepExecutions(latest.id);
          setRunResult({
            success: latest.status === 'success',
            steps: steps.step_executions || [],
            error: latest.status === 'failed' ? 'Workflow failed' : undefined,
          });
          break;
        }
      }
    } catch (err) {
      const fe = friendlyError(err);
      setRunResult({ error: fe.message });
    } finally {
      setRunning(false);
    }
  };

  const deployService = async () => {
    setDeploying(true);
    setDeployResult(null);
    try {
      // Create a deploy_sim workflow
      const deployWf = await workflows.create({
        name: `Deploy ${serviceName}`,
        spec: {
          steps: [
            {
              name: 'simulate-deploy',
              type: 'deploy_sim',
              params: {
                service_name: serviceName || 'my-service',
                namespace: 'default',
                image: 'latest',
              },
            },
          ],
        },
      });
      
      // Execute it
      await workflows.execute(deployWf.id);
      
      // Poll for result
      for (let i = 0; i < 20; i++) {
        await new Promise(r => setTimeout(r, 1000));
        const execs = await workflows.executions(deployWf.id);
        const latest = execs.executions?.[0];
        
        if (latest && latest.status === 'success') {
          const steps = await workflows.stepExecutions(latest.id);
          const stepOutput = steps.step_executions?.[0]?.output;
          let output = null;
          try {
            output = typeof stepOutput === 'string' ? JSON.parse(stepOutput) : stepOutput;
          } catch {}
          
          setDeployResult({
            success: true,
            simulated: output?.simulated || true,
            message: output?.message || 'Deployment simulated successfully',
          });
          break;
        } else if (latest && latest.status === 'failed') {
          setDeployResult({ success: false, message: 'Deployment failed' });
          break;
        }
      }
    } catch (err) {
      const fe = friendlyError(err);
      setDeployResult({ success: false, message: fe.message });
    } finally {
      setDeploying(false);
    }
  };

  if (loading) {
    return <div className="flex items-center justify-center h-96">Loading...</div>;
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6 max-w-4xl">
      <div className="page-animate mb-8">
        <h1 className="page-title-modern">Quickstart Guide</h1>
        <p className="page-subtitle-modern">
          Go from zero to "my service is deployed" in under 5 minutes. Complete each section below to get started.
        </p>
      </div>

      {/* Section 1: Register Service */}
      <div className="mb-8 p-6 bg-[var(--surface)] rounded-xl shadow-sm border page-animate-up page-delay-1 modern-card-hover" style={{ borderRadius: '12px' }}>
        <div className="flex items-start gap-4 mb-4">
          <div className={`w-8 h-8 rounded-full flex items-center justify-center text-white font-bold ${serviceCreated ? 'bg-green-500' : 'bg-blue-500'}`}>
            {serviceCreated ? '✓' : '1'}
          </div>
          <div className="flex-1">
            <h2 className="text-xl font-semibold mb-2">Register Your Service</h2>
            <p className="text-[var(--text-secondary)] mb-4">
              Create a service entry in the catalog. We'll use the static-site template to get you started quickly.
            </p>
            
            {!serviceCreated ? (
              <>
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={serviceName}
                    onChange={(e) => setServiceName(e.target.value)}
                    placeholder="Enter service name (e.g., my-awesome-app)"
                    className="flex-1 px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                    disabled={creatingService}
                  />
                  <button
                    onClick={createService}
                    disabled={!serviceName.trim() || creatingService}
                    className="px-4 py-2 bg-blue-500 text-white rounded-md hover:bg-blue-600 disabled:bg-[var(--border)] disabled:cursor-not-allowed"
                  >
                    {creatingService ? 'Creating...' : 'Create Service'}
                  </button>
                </div>
                {serviceError && <div className="mt-2 p-2 rounded-md bg-red-500/10 border border-red-500/20 text-sm text-red-400">{serviceError}</div>}
              </>
            ) : (
              <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-md">
                <p className="text-emerald-600">
                  ✓ Service "<strong>{serviceName}</strong>" created successfully!
                  <Link href={`/services?id=${serviceId}`} className="ml-2 text-blue-600 hover:underline">
                    View service →
                  </Link>
                </p>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Section 2: Create Pipeline */}
      <div className={`mb-8 p-6 bg-[var(--surface)] rounded-xl shadow-sm border page-animate-up page-delay-2 modern-card-hover ${!serviceCreated ? 'opacity-50' : ''}`} style={{ borderRadius: '12px' }}>
        <div className="flex items-start gap-4 mb-4">
          <div className={`w-8 h-8 rounded-full flex items-center justify-center text-white font-bold ${pipelineCreated ? 'bg-green-500' : serviceCreated ? 'bg-blue-500' : 'bg-gray-300'}`}>
            {pipelineCreated ? '✓' : '2'}
          </div>
          <div className="flex-1">
            <h2 className="text-xl font-semibold mb-2">Create a Pipeline</h2>
            <p className="text-[var(--text-secondary)] mb-4">
              We'll create a "Hello PEPA" pipeline for your service. This template has no external dependencies and runs entirely in the simulator.
            </p>
            
            {!pipelineCreated ? (
              <>
                <button
                  onClick={createPipeline}
                  disabled={!serviceCreated || creatingPipeline}
                  className="px-4 py-2 bg-blue-500 text-white rounded-md hover:bg-blue-600 disabled:bg-[var(--border)] disabled:cursor-not-allowed"
                >
                  {creatingPipeline ? 'Creating...' : 'Create Pipeline'}
                </button>
                {pipelineError && <div className="mt-2 p-2 rounded-md bg-red-500/10 border border-red-500/20 text-sm text-red-400">{pipelineError}</div>}
              </>
            ) : (
              <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-md">
                <p className="text-emerald-600">
                  ✓ Pipeline created! 
                  <Link href={`/workflows/${workflowId}`} className="ml-2 text-blue-600 hover:underline">
                    View workflow →
                  </Link>
                </p>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Section 3: Run Pipeline */}
      <div className={`mb-8 p-6 bg-[var(--surface)] rounded-xl shadow-sm border page-animate-up page-delay-3 modern-card-hover ${!pipelineCreated ? 'opacity-50' : ''}`} style={{ borderRadius: '12px' }}>
        <div className="flex items-start gap-4 mb-4">
          <div className={`w-8 h-8 rounded-full flex items-center justify-center text-white font-bold ${runResult?.success ? 'bg-green-500' : pipelineCreated ? 'bg-blue-500' : 'bg-gray-300'}`}>
            {runResult?.success ? '✓' : '3'}
          </div>
          <div className="flex-1">
            <h2 className="text-xl font-semibold mb-2">Run It</h2>
            <p className="text-[var(--text-secondary)] mb-4">
              Execute your pipeline and watch the step-by-step progress. The simulator will handle everything automatically.
            </p>
            
            {pipelineCreated && !runResult?.success && (
              <button
                onClick={runPipeline}
                disabled={running}
                className="px-4 py-2 bg-blue-500 text-white rounded-md hover:bg-blue-600 disabled:bg-[var(--border)] disabled:cursor-not-allowed"
              >
                {running ? 'Running...' : 'Run Pipeline'}
              </button>
            )}
            
            {runResult && (
              <div className={`p-3 border rounded-md ${runResult.success ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'}`}>
                {runResult.success ? (
                  <div>
                    <p className="text-emerald-600 font-semibold mb-2">✓ Pipeline executed successfully!</p>
                    {runResult.steps && runResult.steps.length > 0 && (
                      <div className="text-sm text-emerald-600">
                        <p className="font-semibold mb-1">Steps completed:</p>
                        <ul className="list-disc list-inside space-y-1">
                          {runResult.steps.map((step, i) => (
                            <li key={i}>{step.step_name} — {step.status}</li>
                          ))}
                        </ul>
                      </div>
                    )}
                  </div>
                ) : (
                  <p className="text-red-500">✗ {runResult.error || 'Pipeline failed'}</p>
                )}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Section 4: Deploy */}
      <div className={`mb-8 p-6 bg-[var(--surface)] rounded-xl shadow-sm border page-animate-up page-delay-4 modern-card-hover ${!runResult?.success ? 'opacity-50' : ''}`} style={{ borderRadius: '12px' }}>
        <div className="flex items-start gap-4 mb-4">
          <div className={`w-8 h-8 rounded-full flex items-center justify-center text-white font-bold ${deployResult?.success ? 'bg-green-500' : runResult?.success ? 'bg-blue-500' : 'bg-gray-300'}`}>
            {deployResult?.success ? '✓' : '4'}
          </div>
          <div className="flex-1">
            <h2 className="text-xl font-semibold mb-2">Deploy</h2>
            <p className="text-[var(--text-secondary)] mb-4">
              {deployResult?.simulated ? (
                <>Your service has been deployed (simulated). In a real environment with a connected Kubernetes cluster, this would deploy to ArgoCD or FluxCD.</>
              ) : (
                <>Deploy your service. Since no cluster is connected, we'll run a simulated deployment to show you how it works.</>
              )}
            </p>
            
            {runResult?.success && !deployResult?.success && (
              <button
                onClick={deployService}
                disabled={deploying}
                className="px-4 py-2 bg-blue-500 text-white rounded-md hover:bg-blue-600 disabled:bg-[var(--border)] disabled:cursor-not-allowed"
              >
                {deploying ? 'Deploying...' : 'Deploy Service'}
              </button>
            )}
            
            {deployResult && (
              <div className={`p-3 border rounded-md ${deployResult.success ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'}`}>
                {deployResult.success ? (
                  <div>
                    <p className="text-emerald-600 font-semibold mb-2">✓ {deployResult.simulated ? 'Simulated deployment' : 'Deployment'} successful!</p>
                    <p className="text-sm text-emerald-600">{deployResult.message}</p>
                  </div>
                ) : (
                  <p className="text-red-500">✗ {deployResult.message}</p>
                )}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Completion */}
      {deployResult?.success && (
        <div className="p-6 bg-gradient-to-r from-green-500 to-blue-500 text-white rounded-xl shadow-lg page-animate-up page-delay-5" style={{ borderRadius: '12px' }}>
          <h2 className="text-2xl font-bold mb-2">🎉 Congratulations!</h2>
          <p className="mb-4">
            You've successfully registered a service, created a pipeline, ran it, and deployed your service.
            You're now ready to explore more advanced features.
          </p>
          <div className="flex gap-3">
            <Link
              href="/services"
              className="px-4 py-2 bg-white text-blue-600 rounded-md hover:bg-white/90 font-semibold"
            >
              View Your Services
            </Link>
            <Link
              href="/automation"
              className="px-4 py-2 bg-white/20 text-white rounded-md hover:bg-white/30 font-semibold"
            >
              Explore Automation
            </Link>
          </div>
        </div>
      )}
      </div>
    </div>
  );
}
