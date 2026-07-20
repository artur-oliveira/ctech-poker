import type {Metadata, Viewport} from 'next'
import {QueryProvider} from '@/lib/providers/QueryProvider'
import './globals.css'

export const metadata: Metadata = {
  metadataBase: new URL(process.env.NEXT_PUBLIC_APP_URL || 'https://poker.aoctech.app'),
  title: {default: 'CTech Poker — a mesa está pronta', template: '%s · CTech Poker'},
  description: 'Texas Hold’em prático, social e responsivo, onde você estiver. Jogue com fichas sandbox ou dinheiro real e reúna seus amigos.',
  applicationName: 'CTech Poker',
  keywords: ['poker online', 'Texas Hold’em', 'poker com amigos', 'CTech Poker', 'poker sandbox'],
  alternates: {canonical: '/'},
  icons: {
    icon: [{url: '/favicon.ico'}, {
      url: '/favicon-16x16.png',
      sizes: '16x16',
      type: 'image/png'
    }, {url: '/favicon-32x32.png', sizes: '32x32', type: 'image/png'}],
    apple: [{url: '/apple-touch-icon.png', sizes: '180x180', type: 'image/png'}],
    other: [{rel: 'manifest', url: '/site.webmanifest'}]
  },
  manifest: '/site.webmanifest',
  openGraph: {
    type: 'website',
    locale: 'pt_BR',
    url: '/',
    siteName: 'CTech Poker',
    title: 'CTech Poker — a mesa está pronta',
    description: 'Texas Hold’em prático, social e responsivo, onde você estiver.',
    images: [{
      url: '/og-image.png',
      width: 1200,
      height: 630,
      alt: 'CTech Poker — o jeito mais simples de jogar poker com amigos.'
    }]
  },
  twitter: {
    card: 'summary_large_image',
    title: 'CTech Poker — a mesa está pronta',
    description: 'Texas Hold’em prático, social e responsivo, onde você estiver.',
    images: ['/og-image.png']
  },
  robots: {
    index: true,
    follow: true,
    googleBot: {index: true, follow: true, 'max-image-preview': 'large', 'max-snippet': -1, 'max-video-preview': -1}
  },
  category: 'games',
}
export const viewport: Viewport = {
  width: 'device-width',
  initialScale: 1,
  viewportFit: 'cover'
}
export default function Layout({children}: { children: React.ReactNode }) {
  return <html lang="pt-BR" suppressHydrationWarning>
  <body suppressHydrationWarning><QueryProvider>{children}</QueryProvider></body>
  </html>
}
