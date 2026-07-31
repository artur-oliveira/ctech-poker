import Link from 'next/link';
import Image from 'next/image';
import {OG_PREVIEWS} from '@/lib/ogPreviews';

export default function OgPreviewStudio() {
  return <main className="og-studio">
    <header>
      <p>Ferramenta local de produção · não faz parte do app</p>
      <h1>Imagens sociais por rota</h1>
      <span>Capture cada quadro em 1200 × 630 e salve no caminho indicado.</span>
    </header>
    <section>
      {OG_PREVIEWS.map(preview => <figure key={preview.slug}>
        <Link href={`/${preview.slug}`} aria-label={`Abrir ${preview.title} para captura`}>
          <Image src={`/og/${preview.slug}.webp`} width={1200} height={630} alt={`Captura real: ${preview.title}`}/>
        </Link>
        <figcaption><code>public/og/{preview.slug}.webp</code><span>{preview.route}</span></figcaption>
      </figure>)}
    </section>
  </main>;
}
