'use client';

import dynamic from 'next/dynamic';

// Lazy-load heavy global components — only loaded when actually used
const CommandPalette = dynamic(() => import('@/components/CommandPalette'), { ssr: false });
const OnboardingTour = dynamic(() => import('@/components/OnboardingTour'), { ssr: false });

export default function DynamicComponents() {
  return (
    <>
      <CommandPalette />
      <OnboardingTour />
    </>
  );
}
