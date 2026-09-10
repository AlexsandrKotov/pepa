'use client';

import { useEffect, useRef, useState, memo } from 'react';

interface Ring {
  /** 0-100 */
  value: number;
  color: string;
  label: string;
  /** Background track color (optional, defaults to a dim version of color) */
  trackColor?: string;
}

interface ActivityRingsProps {
  rings: Ring[];
  size?: number;
  /** Text shown in the center of the rings */
  centerLabel?: string;
  centerValue?: string;
  className?: string;
}

/**
 * Apple Watch-style activity rings — concentric circular progress indicators.
 * SVG-based with animated fill on mount.
 */
function ActivityRingsInner({
  rings,
  size = 120,
  centerLabel,
  centerValue,
  className = '',
}: ActivityRingsProps) {
  const [animated, setAnimated] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // Animate rings on mount (or when value changes)
  useEffect(() => {
    // Small delay to trigger CSS transition
    const timer = setTimeout(() => setAnimated(true), 100);
    return () => clearTimeout(timer);
  }, [rings]);

  const strokeWidth = Math.max(6, size / 16);
  const gap = strokeWidth + 4;
  const center = size / 2;

  return (
    <div ref={containerRef} className={`relative inline-flex items-center justify-center ${className}`} style={{ '--ring-size': `${size}px` } as React.CSSProperties}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
        {rings.map((ring, idx) => {
          const radius = center - strokeWidth / 2 - idx * gap;
          const circumference = 2 * Math.PI * radius;
          const progress = Math.min(100, Math.max(0, ring.value));
          const dashOffset = circumference - (circumference * progress) / 100;
          const trackColor = ring.trackColor || `${ring.color}20`;

          return (
            <g key={idx}>
              {/* Track (background ring) */}
              <circle
                cx={center}
                cy={center}
                r={radius}
                fill="none"
                stroke={trackColor}
                strokeWidth={strokeWidth}
                strokeLinecap="round"
              />
              {/* Progress ring */}
              <circle
                cx={center}
                cy={center}
                r={radius}
                fill="none"
                stroke={ring.color}
                strokeWidth={strokeWidth}
                strokeLinecap="round"
                strokeDasharray={circumference}
                strokeDashoffset={animated ? dashOffset : circumference}
                transform={`rotate(-90 ${center} ${center})`}
                style={{
                  transition: `stroke-dashoffset 0.8s var(--ease-ios-snappy) ${idx * 0.1}s`,
                }}
              />
            </g>
          );
        })}
      </svg>

      {/* Center content */}
      {(centerValue || centerLabel) && (
        <div className="absolute inset-0 flex flex-col items-center justify-center pointer-events-none">
          {centerValue && (
            <span className="text-[calc(var(--ring-size)*0.22)] font-bold text-[var(--text-primary)] leading-none">
              {centerValue}
            </span>
          )}
          {centerLabel && (
            <span className="text-[calc(var(--ring-size)*0.1)] text-[var(--text-tertiary)] mt-0.5 leading-none">
              {centerLabel}
            </span>
          )}
        </div>
      )}
    </div>
  );
}

/** Single ring with label underneath — for dashboard stat cards */
export function ActivityRingStat({
  value,
  label,
  color,
  size = 64,
}: {
  value: number;
  label: string;
  color: string;
  size?: number;
}) {
  return (
    <div className="flex flex-col items-center gap-1.5">
      <ActivityRings
        rings={[{ value, color, label }]}
        size={size}
        centerValue={`${Math.round(value)}%`}
      />
      <span className="text-[11px] text-[var(--text-secondary)] font-medium">{label}</span>
    </div>
  );
}

const ActivityRings = memo(ActivityRingsInner);
ActivityRings.displayName = 'ActivityRings';

export default ActivityRings;
