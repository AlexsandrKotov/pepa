/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  compress: true,
  productionBrowserSourceMaps: false,
  images: {
    unoptimized: true,
  },
  experimental: {
    optimizePackageImports: ['@heroicons/react', 'lucide-react'],
  },
}

module.exports = nextConfig;
