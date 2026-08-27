'use client';
import { Suspense } from 'react';
import { useSearchParams } from 'next/navigation';
import S3ManageClient from './S3ManageClient';

function S3ManagePageContent() {
  const searchParams = useSearchParams();
  const connectionId = searchParams.get('connection') || undefined;
  const bucket = searchParams.get('bucket') || undefined;
  const prefix = searchParams.get('prefix') || undefined;

  return (
    <S3ManageClient
      initialConnectionId={connectionId}
      initialBucket={bucket}
      initialPrefix={prefix}
    />
  );
}

export default function S3ManagePage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center py-12"><div className="loading-spinner" /></div>}>
      <S3ManagePageContent />
    </Suspense>
  );
}
