'use client';

import type { CSSProperties } from 'react';

interface BrandIconProps {
  name: string;
  size?: number;
  className?: string;
  style?: CSSProperties;
  monochrome?: boolean;
}

/**
 * Real brand SVG icons for services and tools used across the platform.
 * Each icon is a simplified, recognizable brand mark rendered as an inline SVG.
 * Uses authentic brand colors for a professional, lively appearance.
 */
export default function BrandIcon({ name, size = 16, className = '', style, monochrome = false }: BrandIconProps) {
  const iconData = BRAND_ICONS[name.toLowerCase()] || BRAND_ICONS['default'];
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      className={className}
      style={style}
      xmlns="http://www.w3.org/2000/svg"
    >
      {monochrome ? (
        <>
          <style>{`path, circle, ellipse, rect { fill: currentColor !important; } ellipse[fill="none"], path[fill="none"] { stroke: currentColor !important; fill: none !important; }`}</style>
          {iconData}
        </>
      ) : iconData}
    </svg>
  );
}

/** Brand icon paths keyed by lowercase name with authentic brand colors */
const BRAND_ICONS: Record<string, React.ReactNode> = {
  // GitLab — fox face with orange gradient
  gitlab: (
    <>
      <path d="M12 21.35l-.57-.36L4.64 16.4a1.1 1.1 0 01-.4-1.24l1.46-4.5L7.08 6.5a.55.55 0 011.05 0l1.38 4.16h5.02L15.9 6.5a.55.55 0 011.05 0l1.38 4.16 1.46 4.5a1.1 1.1 0 01-.4 1.24l-6.79 4.59-.6.36z" fill="#FC6D26" />
      <path d="M12 21.35l-.57-.36L4.64 16.4a1.1 1.1 0 01-.4-1.24l1.46-4.5h12.6l1.46 4.5a1.1 1.1 0 01-.4 1.24l-6.79 4.59-.6.36z" fill="#E24329" />
    </>
  ),

  // GitHub — octocat in dark gray
  github: (
    <path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.477 2 12c0 4.42 2.865 8.166 6.839 9.489.5.092.682-.217.682-.482 0-.237-.009-.866-.013-1.7-2.782.604-3.369-1.34-3.369-1.34-.454-1.156-1.11-1.463-1.11-1.463-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.831.092-.646.35-1.086.636-1.337-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0112 6.836c.85.004 1.705.115 2.504.337 1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.163 22 16.418 22 12c0-5.523-4.477-10-10-10z" fill="#24292F" />
  ),

  // Jira — diamond in brand blue
  jira: (
    <>
      <path d="M11.571 11.513H0a5.18 5.18 0 005.175 5.175h1.993v1.993A5.18 5.18 0 0012.343 24V12.343z" fill="#2684FF" />
      <path d="M23.165 11.513H11.571V0a5.18 5.18 0 00-5.175 5.175v1.993H4.403A5.18 5.18 0 00-.772 12.343H11.571V24a5.18 5.18 0 005.175-5.175v-1.993h1.993a5.18 5.18 0 005.175-5.175z" fill="#0052CC" />
    </>
  ),

  // Slack — four-color hash mark
  slack: (
    <>
      <path d="M5.042 15.165a2.528 2.528 0 01-2.52 2.523A2.528 2.528 0 010 15.165a2.527 2.527 0 012.522-2.52h2.52v2.52z" fill="#E01E5A" />
      <path d="M6.313 15.165a2.527 2.527 0 012.521-2.52 2.527 2.527 0 012.521 2.52v6.313A2.528 2.528 0 018.834 24a2.528 2.528 0 01-2.521-2.522v-6.313z" fill="#36C5F3" />
      <path d="M8.834 5.042a2.528 2.528 0 01-2.521-2.52A2.528 2.528 0 018.834 0a2.528 2.528 0 012.521 2.522v2.52H8.834z" fill="#2EB67D" />
      <path d="M8.834 6.313a2.528 2.528 0 012.521 2.521 2.528 2.528 0 01-2.521 2.521H2.522A2.528 2.528 0 010 8.834a2.528 2.528 0 012.522-2.521h6.312z" fill="#ECB22E" />
    </>
  ),

  // Kubernetes — helm/wheel in brand blue
  kubernetes: (
    <path d="M12 0C5.373 0 0 5.373 0 12s5.373 12 12 12 12-5.373 12-12S18.627 0 12 0zm0 1.846c5.164 0 9.385 3.9 10.06 8.923h-3.693a6.476 6.476 0 00-1.176-3.398l2.612-2.612A10.134 10.134 0 0012 1.846zm0 2.308a7.846 7.846 0 016.77 3.884l-1.847 1.846A5.538 5.538 0 0012 7.846a5.538 5.538 0 00-4.923 2.038L5.23 8.038A7.846 7.846 0 0112 4.154zM5.538 10.154a5.538 5.538 0 001.308 7.5l-2.612 2.612A10.134 10.134 0 011.846 12c0-.636.058-1.26.17-1.866l3.523.02zm12.924 0h3.523c.112.606.17 1.23.17 1.866 0 3.22-1.508 6.092-3.846 7.954l-2.612-2.612a5.538 5.538 0 001.765-7.208zM12 9.692a2.308 2.308 0 110 4.616 2.308 2.308 0 010-4.616zm-3.692 8.77a5.538 5.538 0 007.384 0l2.612 2.612A10.134 10.134 0 0112 22.154a10.134 10.134 0 01-6.304-1.08l2.612-2.612z" fill="#326CE5" />
  ),

  // Docker — whale in brand blue
  docker: (
    <path d="M13.983 11.078h2.118a.186.186 0 00.186-.185V9.006a.186.186 0 00-.186-.186h-2.118a.185.185 0 00-.185.185v1.888c0 .102.083.185.185.185zm-2.954-2.117h2.118a.186.186 0 00.186-.186V6.889a.186.186 0 00-.186-.186h-2.118a.185.185 0 00-.185.186v1.887c0 .102.083.185.185.185zm-2.954 0h2.119a.185.185 0 00.185-.185V6.89a.185.185 0 00-.185-.186H8.075a.185.185 0 00-.185.185v1.888c0 .102.083.185.185.185zm-2.954 0h2.12a.185.185 0 00.184-.185V6.89a.185.185 0 00-.185-.186h-2.119a.185.185 0 00-.185.185v1.888c0 .102.083.185.185.185zm-2.718 0h2.119a.186.186 0 00.185-.186V6.889a.186.186 0 00-.185-.186H2.407a.186.186 0 00-.186.186v1.887c0 .102.083.185.186.185zm14.54 2.117h2.118a.185.185 0 00.185-.185V9.006a.186.186 0 00-.185-.186h-2.118a.185.185 0 00-.186.185v1.888c0 .102.083.185.186.185zm-2.954 0h2.119a.185.185 0 00.185-.185V9.006a.185.185 0 00-.185-.186h-2.119a.186.186 0 00-.185.185v1.888c0 .102.083.185.185.185zm-2.954 0h2.119a.186.186 0 00.186-.185V9.006a.186.186 0 00-.186-.186h-2.118a.185.185 0 00-.185.185v1.888c0 .102.083.185.185.185zm0-2.117h2.119a.186.186 0 00.186-.186V6.889a.186.186 0 00-.186-.186h-2.118a.185.185 0 00-.185.186v1.887c0 .102.083.185.185.185zm-2.954 0h2.119a.185.185 0 00.185-.186V6.889a.185.185 0 00-.185-.186H8.075a.185.185 0 00-.185.186v1.887c0 .102.083.185.185.185zm10.994 0h2.119a.185.185 0 00.184-.186V6.889a.185.185 0 00-.184-.186h-2.12a.186.186 0 00-.185.186v1.887c0 .102.083.185.186.185zm-1.353 5.586c-.393-.262-.872-.397-1.437-.397H.453a.186.186 0 00-.186.186v2.02c0 2.814 1.074 5.06 3.193 6.69 1.895 1.458 4.432 2.262 7.54 2.393 1.243.053 2.44-.059 3.572-.332 2.154-.518 3.97-1.54 5.396-3.032 1.307-1.37 2.163-3.063 2.55-5.035.053-.27-.04-.488-.262-.618l-.002-.001-.001-.001a.186.186 0 00-.093-.025z" fill="#2496ED" />
  ),

  // Git — branch in orange
  git: (
    <path d="M15 12c0-1.654-1.346-3-3-3s-3 1.346-3 3c0 .995.49 1.876 1.24 2.424l-2.79 4.184a1.502 1.502 0 101.66.083l2.75-4.125c.046.003.093.006.14.006s.094-.003.14-.006l2.75 4.125a1.502 1.502 0 101.66-.083l-2.79-4.184C14.51 13.876 15 12.995 15 12zm-3 1a1 1 0 110-2 1 1 0 010 2z" fill="#F05032" />
  ),

  // Terraform — T-shaped mark in brand purple
  terraform: (
    <path d="M10.332 12.646l-1.157-.658V8.803l4.65 2.676v1.327l-1.167.665v-1.33l-2.326-1.334v1.839zm5.826 3.338l-4.65 2.676V17.33l2.326-1.334v-1.328l-2.326 1.334v-1.327l4.65-2.677v1.33l-2.326 1.334v1.328l2.326-1.334v1.328zm0-6.676l-4.65 2.677v1.327l2.324-1.333v-1.328l-2.324 1.334v-1.328l4.65-2.677v1.33l-2.326 1.334v1.327l2.326-1.333v1.327zM8.006 8.803L3.356 11.48v5.353l1.157.665v-5.353l2.326-1.334v5.353l1.167.665V8.803zm-3.493 9.367l4.65 2.676 4.65-2.676v-1.328l-2.324 1.334v1.327l-2.326 1.334-2.326-1.334v-1.327l-2.324-1.334v1.328z" fill="#7B42BC" />
  ),

  // Ansible — A mark in brand red
  ansible: (
    <path d="M12 0C5.373 0 0 5.373 0 12s5.373 12 12 12 12-5.373 12-12S18.627 0 12 0zm5.568 18.216c-.264.456-.756.684-1.26.684H7.692c-.504 0-.996-.228-1.26-.684L4.164 14.4c-.264-.456-.264-1.02 0-1.476L9.432 4.2c.264-.456.756-.684 1.26-.684h2.616c.504 0 .996.228 1.26.684l5.268 8.724c.264.456.264 1.02 0 1.476l-2.268 3.816z" fill="#EE0000" />
  ),

  // Trivy — shield with magnifier in cyan
  trivy: (
    <>
      <path d="M12 2L3 7v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V7l-9-5zm0 2.18l7 3.82v5c0 4.52-3.13 8.69-7 9.93-3.87-1.24-7-5.41-7-9.93V8l7-3.82z" fill="#1904DA" />
      <circle cx="12" cy="12" r="3" fill="none" stroke="#00BFF3" strokeWidth="1.5" />
      <path d="M14.5 14.5l2 2" stroke="#00BFF3" strokeWidth="1.5" strokeLinecap="round" />
    </>
  ),

  // ArgoCD — arrow/loop in teal
  argocd: (
    <path d="M12 0C5.373 0 0 5.373 0 12s5.373 12 12 12 12-5.373 12-12S18.627 0 12 0zm0 3.6l6.8 11.4H5.2L12 3.6zM7.2 13.2h9.6L12 5.4 7.2 13.2z" fill="#EF7B4D" />
  ),

  // FluxCD — diamond in blue
  fluxcd: (
    <path d="M12 2L2 12l10 10 10-10L12 2zm0 2.828L19.172 12 12 19.172 4.828 12 12 4.828z" fill="#5468FF" />
  ),

  // AI — brain/sparkle in purple
  ai: (
    <path d="M12 2a1 1 0 011 1v2a1 1 0 11-2 0V3a1 1 0 011-1zm0 15a1 1 0 011 1v2a1 1 0 11-2 0v-2a1 1 0 011-1zm7.071-9.071a1 1 0 010 1.414l-1.414 1.414a1 1 0 11-1.414-1.414l1.414-1.414a1 1 0 011.414 0zM7.757 16.243a1 1 0 010 1.414l-1.414 1.414a1 1 0 11-1.414-1.414l1.414-1.414a1 1 0 011.414 0zm10.486 2.828a1 1 0 01-1.414 0l-1.414-1.414a1 1 0 111.414-1.414l1.414 1.414a1 1 0 010 1.414zM6.343 7.757a1 1 0 01-1.414 0L3.515 6.343a1 1 0 011.414-1.414l1.414 1.414a1 1 0 010 1.414zM21 12a1 1 0 01-1 1h-2a1 1 0 110-2h2a1 1 0 011 1zM6 12a1 1 0 01-1 1H3a1 1 0 110-2h2a1 1 0 011 1zm6-3a3 3 0 100 6 3 3 0 000-6z" fill="#8B5CF6" />
  ),

  // Storage — database/stack in blue-gray
  storage: (
    <path d="M3 6c0-1.657 4.03-3 9-3s9 1.343 9 3v2c0 1.657-4.03 3-9 3S3 9.657 3 8V6zm0 4c0 1.657 4.03 3 9 3s9-1.343 9-3v2c0 1.657-4.03 3-9 3s-9-1.343-9-3v-2zm0 4c0 1.657 4.03 3 9 3s9-1.343 9-3v2c0 1.657-4.03 3-9 3s-9-1.343-9-3v-2z" fill="#64748B" />
  ),

  // CI/CD — infinity loop in green
  cicd: (
    <path d="M18.084 9.906c-1.386 0-2.643.648-3.468 1.674L12 14.586l-2.616-3.006A4.434 4.434 0 005.916 9.906 4.434 4.434 0 001.5 14.34a4.434 4.434 0 004.416 4.434c1.386 0 2.643-.648 3.468-1.674L12 14.094l2.616 3.006a4.434 4.434 0 003.468 1.674A4.434 4.434 0 0022.5 14.34a4.434 4.434 0 00-4.416-4.434z" fill="#10B981" />
  ),

  // Plugin — puzzle piece in amber
  plugin: (
    <path d="M10.31 3.11a1 1 0 011.38 0l1.5 1.29a1 1 0 01.31.71v.97a6.002 6.002 0 014.42 4.42h.97a1 1 0 01.71.31l1.29 1.5a1 1 0 010 1.38l-1.29 1.5a1 1 0 01-.71.31h-.97a6.002 6.002 0 01-4.42 4.42v.97a1 1 0 01-.31.71l-1.5 1.29a1 1 0 01-1.38 0l-1.5-1.29a1 1 0 01-.31-.71v-.97a6.002 6.002 0 01-4.42-4.42h-.97a1 1 0 01-.71-.31l-1.29-1.5a1 1 0 010-1.38l1.29-1.5a1 1 0 01.71-.31h.97a6.002 6.002 0 014.42-4.42v-.97a1 1 0 01.31-.71l1.5-1.29zM12 14a2 2 0 100-4 2 2 0 000 4z" fill="#F59E0B" />
  ),

  // Vault — lock/shield in black
  vault: (
    <path d="M12 2L3 7v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V7l-9-5zm0 2.18l7 3.82v5c0 4.52-3.13 8.69-7 9.93-3.87-1.24-7-5.41-7-9.93V8l7-3.82zM11 10v2H9v2h2v2h2v-2h2v-2h-2v-2h-2z" fill="#000000" />
  ),

  // Discovery — radar/magnifier in indigo
  discovery: (
    <path d="M10 2a8 8 0 105.293 14.707l3 3a1 1 0 001.414-1.414l-3-3A8 8 0 0010 2zm-6 8a6 6 0 1112 0 6 6 0 01-12 0z" fill="#6366F1" />
  ),

  // Helm — ship wheel in blue
  helm: (
    <path d="M12 0a1.5 1.5 0 011.5 1.5V3a9 9 0 017.5 7.5h1.5a1.5 1.5 0 110 3H21a9 9 0 01-7.5 7.5v1.5a1.5 1.5 0 11-3 0V21A9 9 0 013 13.5H1.5a1.5 1.5 0 110-3H3A9 9 0 0110.5 3V1.5A1.5 1.5 0 0112 0zm0 5a7 7 0 100 14 7 7 0 000-14zm0 3a4 4 0 110 8 4 4 0 010-8z" fill="#0F1689" />
  ),

  // Bitbucket — bucket in blue
  bitbucket: (
    <path d="M1.5 1.062A1 1 0 00.5 2.062l3.1 18.5a1 1 0 00.97.838h13.36a.75.75 0 00.74-.625l2.08-12.5H6.72l-.48 2.88h9.52l-1.12 6.75H6.16L4.28 5.562h15.44l.48-2.88a1 1 0 00-.98-1.12H2.16a1 1 0 00-.66.5z" fill="#0052CC" />
  ),

  // Gitea — tea cup in green
  gitea: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 15H8v-1h3v1zm5.96-5.46c-.36.66-.96 1.1-1.64 1.28-.24.06-.5.1-.76.1-.28 0-.54-.04-.78-.12-.66-.2-1.2-.64-1.56-1.24-.16-.28-.28-.58-.34-.9h5.64c-.06.32-.18.62-.36.88zM11 9.5c0-.28.22-.5.5-.5h5c.28 0 .5.22.5.5s-.22.5-.5.5h-5c-.28 0-.5-.22-.5-.5z" fill="#609926" />
  ),

  // Marketplace — storefront in pink
  marketplace: (
    <path d="M3 3h18v2H3V3zm0 4h18v2H3V7zm0 4h18v2H3v-2zm0 4h12v2H3v-2zm16 0l4 4-4 4v-8z" fill="#EC4899" />
  ),

  // Default — generic circle in gray
  default: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.42 0-8-3.58-8-8s3.58-8 8-8 8 3.58 8 8-3.58 8-8 8z" fill="#9CA3AF" />
  ),

  // Dashboard — grid layout in slate
  dashboard: (
    <path d="M3 3h8v8H3V3zm0 10h8v8H3v-8zm10-10h8v8h-8V3zm0 10h8v8h-8v-8z" fill="#475569" />
  ),

  // Node.js — hexagon in green
  nodejs: (
    <path d="M12 2L3 7v10l9 5 9-5V7l-9-5zm0 2.18l6.5 3.64v7.36L12 18.82l-6.5-3.64V7.82L12 4.18z" fill="#339933" />
  ),

  // Go — gopher in blue
  go: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1.5 14.5c-1.38 0-2.5-1.12-2.5-2.5s1.12-2.5 2.5-2.5 2.5 1.12 2.5 2.5-1.12 2.5-2.5 2.5zm5-5c-1.38 0-2.5-1.12-2.5-2.5s1.12-2.5 2.5-2.5 2.5 1.12 2.5 2.5-1.12 2.5-2.5 2.5z" fill="#00ADD8" />
  ),

  // Python — two snakes in blue/yellow
  python: (
    <>
      <path d="M12 2C8.13 2 5 5.13 5 9v3h7v1H5c-3.87 0-7 3.13-7 7s3.13 7 7 7h3v-3c0-2.76 2.24-5 5-5h3c2.76 0 5-2.24 5-5V9c0-3.87-3.13-7-7-7h-2zm-1 16a1 1 0 110 2 1 1 0 010-2z" fill="#3776AB" />
      <path d="M19 10v-1h-7V8h7c3.87 0 7-3.13 7-7s-3.13-7-7-7h-3v3c0 2.76-2.24 5-5 5H8c-2.76 0-5 2.24-5 5v5c0 3.87 3.13 7 7 7h2v-3a1 1 0 011-1h6zm-1 6a1 1 0 110 2 1 1 0 010-2z" fill="#FFD43B" />
    </>
  ),

  // Java — coffee cup in red
  java: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15h-2v-2h2v2zm4 0h-2v-2h2v2zm2-4H8v-2h8v2zm0-4H8V7h8v2z" fill="#ED8B00" />
  ),

  // .NET — square in purple
  dotnet: (
    <path d="M4 4h16v16H4V4zm2 2v12h12V6H6zm5 2h2v2h-2V8zm0 4h2v2h-2v-2zm0 4h2v2h-2v-2z" fill="#512BD4" />
  ),

  // Ruby — diamond in red
  ruby: (
    <path d="M12 2L2 7l10 15L22 7l-10-5zm0 3.5L17.5 12 12 18.5 6.5 12 12 5.5z" fill="#CC342D" />
  ),

  // PHP — elephant in purple
  php: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 14h-2v-2h2v2zm4 0h-2v-2h2v2zm-6-6c0-1.66 1.34-3 3-3s3 1.34 3 3-1.34 3-3 3-3-1.34-3-3z" fill="#777BB4" />
  ),

  // Rust — gear in black
  rust: (
    <path d="M12 2L9 4 7 2 5 4l2 2-2 2 2 2-2 2 2 2 2-2 2 2 2-2-2-2 2-2-2-2 2-2-2-2 2-2-2 2-2-2-2 2-2-2 2 2 2 2-2 2 2 2-2 2 2 2 2-2 2 2 2-2 2 2 2-2 2 2 2-2z" fill="#000000" />
  ),

  // React — atom in cyan
  react: (
    <>
      <circle cx="12" cy="12" r="2" fill="#61DAFB" />
      <ellipse cx="12" cy="12" rx="10" ry="4" fill="none" stroke="#61DAFB" strokeWidth="1.5" />
      <ellipse cx="12" cy="12" rx="10" ry="4" fill="none" stroke="#61DAFB" strokeWidth="1.5" transform="rotate(60 12 12)" />
      <ellipse cx="12" cy="12" rx="10" ry="4" fill="none" stroke="#61DAFB" strokeWidth="1.5" transform="rotate(120 12 12)" />
    </>
  ),

  // Next.js — N in black
  nextjs: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 15V9l5 8h2V7h-2v8l-5-8H9v10h2z" fill="#000000" />
  ),

  // Vue — V in green
  vue: (
    <path d="M2 3l10 18L22 3h-4l-6 11L6 3H2z" fill="#4FC08D" />
  ),

  // Angular — shield in red
  angular: (
    <path d="M12 2L3 5l1.5 13L12 22l7.5-4L21 5l-9-3zm0 2.5l5.5 10.5H14l-1-2.5h-2l-1 2.5H7.5L12 4.5z" fill="#DD0031" />
  ),

  // PostgreSQL — elephant in blue
  postgresql: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-3 15c-1.66 0-3-1.34-3-3s1.34-3 3-3 3 1.34 3 3-1.34 3-3 3zm5-5c-1.66 0-3-1.34-3-3s1.34-3 3-3 3 1.34 3 3-1.34 3-3 3z" fill="#4169E1" />
  ),

  // Redis — diamond in red
  redis: (
    <path d="M12 2L2 7v10l10 5 10-5V7l-10-5zm0 2.18l6.5 3.64v7.36L12 18.82l-6.5-3.64V7.82L12 4.18z" fill="#DC382D" />
  ),

  // MongoDB — leaf in green
  mongodb: (
    <path d="M12 2C8.13 2 5 5.13 5 9c0 3.87 3.13 13 7 13s7-9.13 7-13c0-3.87-3.13-7-7-7zm0 2c2.76 0 5 2.24 5 5s-2.24 9-5 9-5-6.24-5-9 2.24-5 5-5z" fill="#47A248" />
  ),

  // MySQL — dolphin in blue
  mysql: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-2-2 2-2 2 2-2 2zm6-4l-2-2 2-2 2 2-2 2z" fill="#4479A1" />
  ),

  // Elasticsearch — magnifier in orange
  elasticsearch: (
    <path d="M10 2a8 8 0 105.293 14.707l3 3a1 1 0 001.414-1.414l-3-3A8 8 0 0010 2zm-6 8a6 6 0 1112 0 6 6 0 01-12 0z" fill="#F04E98" />
  ),

  // Nginx — N in green
  nginx: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-3 15V7l6 10V7h2v10h-2l-6-10v10H9z" fill="#009639" />
  ),

  // Traefik — traffic light in blue
  traefik: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 15h-2v-2h2v2zm4 0h-2v-2h2v2zm-6-6c0-1.66 1.34-3 3-3s3 1.34 3 3-1.34 3-3 3-3-1.34-3-3z" fill="#24A1C1" />
  ),

  // Prometheus — flame in orange
  prometheus: (
    <path d="M12 2C8.13 2 5 5.13 5 9c0 2.38 1.19 4.47 3 5.74V17c0 .55.45 1 1 1h6c.55 0 1-.45 1-1v-2.26c1.81-1.27 3-3.36 3-5.74 0-3.87-3.13-7-7-7zm-1 13h-2v-2h2v2zm4 0h-2v-2h2v2z" fill="#E6522C" />
  ),

  // Grafana — chart in orange
  grafana: (
    <path d="M3 3h18v2H3V3zm0 4h18v2H3V7zm0 4h18v2H3v-2zm0 4h12v2H3v-2zm16 0l4 4-4 4v-8z" fill="#F46800" />
  ),

  // RabbitMQ — rabbit in orange
  rabbitmq: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-3 15c-1.66 0-3-1.34-3-3s1.34-3 3-3 3 1.34 3 3-1.34 3-3 3zm5-5c-1.66 0-3-1.34-3-3s1.34-3 3-3 3 1.34 3 3-1.34 3-3 3z" fill="#FF6600" />
  ),

  // Kafka — infinity in blue
  kafka: (
    <path d="M18.084 9.906c-1.386 0-2.643.648-3.468 1.674L12 14.586l-2.616-3.006A4.434 4.434 0 005.916 9.906 4.434 4.434 0 001.5 14.34a4.434 4.434 0 004.416 4.434c1.386 0 2.643-.648 3.468-1.674L12 14.094l2.616 3.006a4.434 4.434 0 003.468 1.674A4.434 4.434 0 0022.5 14.34a4.434 4.434 0 00-4.416-4.434z" fill="#231F20" />
  ),

  // NATS — circle in blue
  nats: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.42 0-8-3.58-8-8s3.58-8 8-8 8 3.58 8 8-3.58 8-8 8z" fill="#27AAE1" />
  ),

  // Jupyter — notebook in orange
  jupyter: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15h-2v-2h2v2zm4 0h-2v-2h2v2zm-6-6c0-1.66 1.34-3 3-3s3 1.34 3 3-1.34 3-3 3-3-1.34-3-3z" fill="#F37626" />
  ),

  // MLflow — M in blue
  mlflow: (
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-4 15V7l4 6 4-6v10h-2V9l-2 3-2-3v8H8z" fill="#0194E2" />
  ),

  // SonarQube — magnifier in blue
  sonarqube: (
    <path d="M10 2a8 8 0 105.293 14.707l3 3a1 1 0 001.414-1.414l-3-3A8 8 0 0010 2zm-6 8a6 6 0 1112 0 6 6 0 01-12 0z" fill="#4E9BCD" />
  ),

  // Services — layered boxes in cyan
  services: (
    <path d="M21 7.5l-9-5.25L3 7.5m18 0l-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" fill="none" stroke="#06B6D4" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
  ),

  // Proxmox — server stack with VM windows in Proxmox orange
  proxmox: (
    <g>
      <rect x="3" y="4" width="18" height="7" rx="1.5" fill="none" stroke="#E57000" strokeWidth="1.8" />
      <rect x="3" y="13" width="18" height="7" rx="1.5" fill="none" stroke="#E57000" strokeWidth="1.8" />
      <circle cx="6.5" cy="7.5" r="1.2" fill="#E57000" />
      <circle cx="6.5" cy="16.5" r="1.2" fill="#E57000" />
      <path d="M10 7.5h8M10 16.5h8" stroke="#E57000" strokeWidth="1.8" strokeLinecap="round" />
    </g>
  ),

  // Telegram — paper plane in blue
  telegram: (
    <path d="M11.944 0A12 12 0 1 0 24 12.055C24 5.495 18.627.15 11.944 0zM16.93 8.12l-1.76 8.29c-.13.59-.46.73-.93.46l-2.74-2.02-1.32 1.27c-.15.15-.27.27-.56.27l.2-2.84 5.16-4.66c.22-.2-.05-.31-.35-.11l-6.37 4.01-2.74-.86c-.6-.18-.61-.6.12-.88l10.7-4.13c.5-.18.93.12.79.88z" fill="#0088CC" />
  ),

  // Teams — T in purple-blue
  teams: (
    <>
      <path d="M20.5 2h-17A1.5 1.5 0 002 3.5v17A1.5 1.5 0 003.5 22h17a1.5 1.5 0 001.5-1.5v-17A1.5 1.5 0 0020.5 2z" fill="#5059C9" />
      <path d="M17 8H7v2h4v8h2v-8h4V8z" fill="#FFFFFF" />
    </>
  ),
};

/**
 * Helper to get the icon key for a given service/type name.
 * Useful when the calling code wants to check if an icon exists.
 */
export function getBrandIconKey(name: string): string {
  const key = name.toLowerCase();
  if (key in BRAND_ICONS) return key;
  // Map common aliases
  const aliases: Record<string, string> = {
    'git_provider': 'git',
    'gitlab_ci': 'gitlab',
    'github_actions': 'github',
    'task_tracker': 'jira',
    'notification': 'slack',
    'cd_engine': 'cicd',
    'ci_engine': 'cicd',
    'secret_manager': 'vault',
    'cloud_provider': 'storage',
    'monitoring': 'discovery',
    'virtualization': 'proxmox',
    'custom': 'plugin',
    'kubernetes': 'kubernetes',
    'kube': 'kubernetes',
    'pepa': 'argocd',
  };
  return aliases[key] || 'default';
}
