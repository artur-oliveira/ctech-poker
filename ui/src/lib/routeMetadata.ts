import type {Metadata} from 'next';

const OG_SIZE = {width: 1200, height: 630};

type RouteMetadataOptions = {
  title: string;
  description: string;
  path: string;
  image: string;
  index?: boolean;
};

export function routeMetadata({
                                title,
                                description,
                                path,
                                image,
                                index = false
                              }: RouteMetadataOptions): Metadata {
  const imagePath = `/og/${image}.png`;
  return {
    title,
    description,
    alternates: {canonical: path},
    openGraph: {
      type: 'website',
      locale: 'pt_BR',
      siteName: 'CTech Poker',
      url: path,
      title: `${title} · CTech Poker`,
      description,
      images: [{url: imagePath, ...OG_SIZE, alt: `${title} no CTech Poker`}, {
        url: '/og-image.png',
        ...OG_SIZE,
        alt: 'CTech Poker · sua mesa de poker com amigos'
      }]
    },
    twitter: {
      card: 'summary_large_image',
      title: `${title} · CTech Poker`,
      description,
      images: [imagePath]
    },
    robots: {index, follow: index}
  };
}
