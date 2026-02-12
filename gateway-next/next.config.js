/** @type {import('next').NextConfig} */
const nextConfig = {
  // Disable telemetry and unnecessary features for benchmarking
  typescript: { ignoreBuildErrors: false },
  eslint: { ignoreDuringBuilds: true },
  // Disable image optimization (not needed)
  images: { unoptimized: true },
  // Disable powered by header
  poweredByHeader: false,
};

module.exports = nextConfig;
