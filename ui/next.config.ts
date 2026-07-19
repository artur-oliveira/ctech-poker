import type {NextConfig} from 'next'
import path from 'path'

const isProduction = process.env.NODE_ENV === 'production'
const nextConfig: NextConfig = {
  turbopack: {root: path.join(__dirname)},
  images: {unoptimized: true}, ...(isProduction ? {output: 'export' as const} : {
    async rewrites() {
      return [{
        source: '/v1.0/:path*',
        destination: `${process.env.DEV_API_ORIGIN || 'http://localhost:8003'}/v1.0/:path*`
      }]
    }
  })
}
export default nextConfig
