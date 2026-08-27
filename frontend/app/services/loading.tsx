import { SkeletonTable } from '@/components/Skeleton';

export default function Loading() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="space-y-2">
          <div className="animate-pulse rounded-md bg-[var(--border)]/60 h-5 w-24" />
          <div className="animate-pulse rounded-md bg-[var(--border)]/60 h-3 w-48" />
        </div>
        <div className="animate-pulse rounded-md bg-[var(--border)]/60 h-8 w-28 rounded-lg" />
      </div>
      <SkeletonTable rows={6} cols={6} />
    </div>
  );
}
