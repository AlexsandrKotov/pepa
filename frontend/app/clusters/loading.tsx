import { SkeletonTable } from '@/components/Skeleton';

export default function Loading() {
  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <div className="animate-pulse rounded-md bg-[var(--border)]/60 h-5 w-20" />
        <div className="animate-pulse rounded-md bg-[var(--border)]/60 h-3 w-36" />
      </div>
      <SkeletonTable rows={4} cols={4} />
    </div>
  );
}
