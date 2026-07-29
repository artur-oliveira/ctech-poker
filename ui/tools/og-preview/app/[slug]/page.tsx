import {notFound} from 'next/navigation';
import {OG_IMAGE_DATA, OgRouteImage, type OgSlug} from '@/components/og/OgRouteImage';

export default async function OgPreviewPage({params}: {params: Promise<{slug: string}>}) {
  const {slug} = await params;
  const preview = OG_IMAGE_DATA.find(item => item.slug === slug);
  if (!preview) notFound();
  return <main className="og-capture"><OgRouteImage slug={slug as OgSlug}/></main>;
}
