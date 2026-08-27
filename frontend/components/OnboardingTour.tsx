'use client';

import { useState, useEffect } from 'react';

interface TourStep {
  target: string; // CSS selector or 'center' for modal
  title: string;
  description: string;
  icon?: string;
}

interface Tour {
  id: string;
  steps: TourStep[];
}

const tours: Record<string, Tour> = {
  '/services': {
    id: 'services-tour',
    steps: [
      { target: 'center', title: 'Welcome to Services', description: 'This is where you manage all your deployed services. Each service tracks its deployments across environments.', icon: '📦' },
      { target: '[data-tour="services-filter"]', title: 'Filter Services', description: 'Use filters to find services by source, cluster, or status. Click the filter button to expand options.', icon: '🔍' },
      { target: '[data-tour="services-create"]', title: 'Create a Service', description: 'Click here to register a new service from a template or custom configuration.', icon: '➕' },
    ],
  },
  '/deploy': {
    id: 'deploy-tour',
    steps: [
      { target: 'center', title: 'Quick Deploy Wizard', description: 'Deploy any service template in 4 simple steps — no DevOps knowledge required!', icon: '🚀' },
      { target: 'center', title: 'Choose a Template', description: 'Start by selecting a service template. Templates come with pre-configured defaults for common services.', icon: '📄' },
      { target: 'center', title: 'Configure & Deploy', description: 'Set your target cluster, namespace, and any template-specific parameters, then deploy with one click.', icon: '⚡' },
    ],
  },
  '/connections': {
    id: 'connections-tour',
    steps: [
      { target: 'center', title: 'Connections Hub', description: 'Manage all your external integrations here — Kubernetes clusters, GitLab, Jira, CI/CD, AI providers, and more.', icon: '🔗' },
      { target: '[data-tour="connections-manage-clusters"]', title: 'Manage Clusters', description: 'Click here to add and manage Kubernetes clusters. This is the unified place for all cluster operations.', icon: '☸️' },
      { target: '[data-tour="connections-add"]', title: 'Add Connection', description: 'Connect new integrations like GitLab, Jira, or AI providers to extend your platform capabilities.', icon: '➕' },
    ],
  },
};

export default function OnboardingTour() {
  const [activeTour, setActiveTour] = useState<Tour | null>(null);
  const [currentStep, setCurrentStep] = useState(0);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    // Check if there's a tour for the current path
    const path = window.location.pathname;
    const tour = tours[path];
    if (tour && !localStorage.getItem(`pepa-tour-seen-${tour.id}`)) {
      // Delay showing the tour slightly
      const timer = setTimeout(() => {
        setActiveTour(tour);
        setCurrentStep(0);
        setVisible(true);
      }, 1500);
      return () => clearTimeout(timer);
    }
  }, []);

  const handleNext = () => {
    if (!activeTour) return;
    if (currentStep < activeTour.steps.length - 1) {
      setCurrentStep(prev => prev + 1);
    } else {
      handleFinish();
    }
  };

  const handleFinish = () => {
    if (activeTour) {
      localStorage.setItem(`pepa-tour-seen-${activeTour.id}`, 'true');
    }
    setVisible(false);
    setActiveTour(null);
    setCurrentStep(0);
  };

  const handleSkip = () => {
    handleFinish();
  };

  if (!visible || !activeTour) return null;

  const step = activeTour.steps[currentStep];
  const isLast = currentStep === activeTour.steps.length - 1;
  const progress = ((currentStep + 1) / activeTour.steps.length) * 100;

  return (
    <div className="fixed inset-0 z-[10000] flex items-center justify-center pointer-events-none">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/20 pointer-events-auto" onClick={handleSkip} />

      {/* Tour card */}
      <div className="relative bg-[var(--surface)] rounded-xl shadow-2xl border border-[var(--border)] max-w-[400px] w-full mx-4 pointer-events-auto overflow-hidden">
        {/* Progress bar */}
        <div className="h-1 bg-[var(--border-light)]">
          <div
            className="h-full bg-[var(--accent)] transition-all duration-300"
            style={{ width: `${progress}%` }}
          />
        </div>

        {/* Content */}
        <div className="p-6">
          <div className="flex items-start gap-3 mb-4">
            <span className="text-2xl">{step.icon || '💡'}</span>
            <div>
              <h3 className="text-[15px] font-semibold text-[var(--text-primary)]">{step.title}</h3>
              <p className="text-[13px] text-[var(--text-secondary)] mt-1 leading-relaxed">{step.description}</p>
            </div>
          </div>

          <div className="flex items-center justify-between">
            <span className="text-[11px] text-[var(--text-tertiary)]">
              {currentStep + 1} of {activeTour.steps.length}
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={handleSkip}
                className="text-[12px] text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] px-3 py-1.5 rounded-lg transition-colors"
              >
                Skip tour
              </button>
              <button
                onClick={handleNext}
                className="text-[12px] font-medium text-white bg-[var(--accent)] px-4 py-1.5 rounded-lg hover:opacity-90 transition-opacity"
              >
                {isLast ? 'Got it!' : 'Next'}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
