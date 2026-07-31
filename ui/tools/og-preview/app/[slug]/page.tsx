import {notFound} from 'next/navigation';
import Image from 'next/image';
import {OG_PREVIEWS} from '@/lib/ogPreviews';

export default async function OgPreviewPage({params}: {params: Promise<{slug: string}>}) {
  const {slug} = await params;
  const preview = OG_PREVIEWS.find(item => item.slug === slug);
  if (!preview) notFound();
  return <main className="og-capture">
    <Image src={`/og/${preview.slug}.webp`} alt={`Captura real: ${preview.title}`} width={1200} height={630}/>
  </main>;
}
