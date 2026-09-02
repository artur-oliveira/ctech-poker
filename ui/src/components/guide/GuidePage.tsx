'use client';

import type {LucideIcon} from 'lucide-react';
import {ArrowLeft, ArrowRight, BookOpen, ChevronDown, CircleAlert, Info, Lightbulb, ShieldCheck} from 'lucide-react';
import Image from 'next/image';
import Link from 'next/link';
import type {ReactNode} from 'react';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';
import {useOptionalSession} from '@/lib/auth/session';

export const GUIDE_TOPICS = [
  {href: '/guide/basics', label: 'Primeiros passos'},
  {href: '/guide/table', label: 'Na mesa'},
  {href: '/guide/hands', label: 'Mãos e provas'},
  {href: '/guide/achievements', label: 'Conquistas'},
  {href: '/guide/store', label: 'Fichas'},
  {href: '/guide/profile', label: 'Perfil e estatísticas'},
  {href: '/guide/community', label: 'Comunidade e segurança'},
] as const;

export type GuideSection = {
  id: string;
  title: string;
  summary?: string;
  body: ReactNode;
  image?: {src: string; alt: string; width?: number; height?: number};
};

export function GuidePage({icon, eyebrow, title, description, sections, currentHref, next}: {
  icon: LucideIcon;
  eyebrow: string;
  title: string;
  description: string;
  sections: GuideSection[];
  currentHref: string;
  next?: {href: string; label: string};
}) {
  const {authed} = useOptionalSession();
  return <AppPage authed={authed} current="guide">
    <AppPageBody className="guide guide-topic">
      <AppPageHeader icon={icon} eyebrow={eyebrow} title={title} description={description}/>
      <details className="guide-topic-picker">
        <summary><span>Tópico</span><strong>{GUIDE_TOPICS.find(topic => topic.href === currentHref)?.label}</strong>
          <ChevronDown aria-hidden="true"/></summary>
        <nav aria-label="Tópicos do guia em telas pequenas">
          {GUIDE_TOPICS.map(topic => <Link key={topic.href} href={topic.href}
            aria-current={currentHref === topic.href ? 'page' : undefined}>{topic.label}</Link>)}
        </nav>
      </details>
      <nav className="guide-topic-nav" aria-label="Tópicos do guia">
        {GUIDE_TOPICS.map(topic => <Link key={topic.href} href={topic.href}
          aria-current={currentHref === topic.href ? 'page' : undefined}>{topic.label}</Link>)}
      </nav>
      <aside className="guide-on-this-page" aria-labelledby="guide-page-index-title">
        <b id="guide-page-index-title">Nesta página</b>
        <ol>{sections.map(section => <li key={section.id}><a href={`#${section.id}`}>{section.title}</a></li>)}</ol>
      </aside>
      <details className="guide-page-index-mobile">
        <summary>Nesta página <ChevronDown aria-hidden="true"/></summary>
        <nav aria-label="Nesta página">
          {sections.map(section => <a key={section.id} href={`#${section.id}`}>{section.title}</a>)}
        </nav>
      </details>
      <div className="guide-topic-content">
        {sections.map(section => <article key={section.id} id={section.id} className="guide-topic-section">
          <header>
            <div><h2>{section.title}</h2>{section.summary && <p>{section.summary}</p>}</div>
          </header>
          {section.image && <figure className="guide-shot guide-topic-shot">
            <Image src={section.image.src} alt={section.image.alt} width={section.image.width ?? 1440}
                   height={section.image.height ?? 900} sizes="(max-width: 720px) 100vw, 960px"/>
          </figure>}
          <div className="guide-topic-body">{section.body}</div>
        </article>)}
      </div>
      <footer className="guide-topic-footer">
        <Link href="/guide"><ArrowLeft aria-hidden="true"/> Todos os guias</Link>
        {next && <Link href={next.href}>{next.label}<ArrowRight aria-hidden="true"/></Link>}
      </footer>
    </AppPageBody>
  </AppPage>;
}

export function GuideSteps({children}: {children: ReactNode}) {
  return <ol className="guide-steps">{children}</ol>;
}

export function GuideBullets({children}: {children: ReactNode}) {
  return <ul className="guide-bullets">{children}</ul>;
}

export function GuideCallout({kind = 'info', title, children}: {
  kind?: 'info' | 'tip' | 'safe' | 'warning'; title: string; children: ReactNode;
}) {
  const Icon = kind === 'tip' ? Lightbulb : kind === 'safe' ? ShieldCheck : kind === 'warning' ? CircleAlert : Info;
  return <aside className={`guide-callout ${kind}`}>
    <Icon aria-hidden="true"/><div><b>{title}</b><p>{children}</p></div>
  </aside>;
}

export function GuideLink({href, children}: {href: string; children: ReactNode}) {
  return <Link className="guide-inline-link" href={href}>{children}<ArrowRight aria-hidden="true"/></Link>;
}

export function GuideTerm({term, children}: {term: string; children: ReactNode}) {
  return <div className="guide-term"><dt>{term}</dt><dd>{children}</dd></div>;
}

/** `variant="keys"` sets the term column in the mono face used by the table's
 * own <kbd> chips, so a shortcut list reads as keys and not as vocabulary. */
export function GuideTerms({children, variant}: {children: ReactNode; variant?: 'keys'}) {
  return <dl className={variant ? `guide-terms guide-${variant}` : 'guide-terms'}>{children}</dl>;
}

export function GuideEmptyIcon() {
  return <BookOpen aria-hidden="true"/>;
}
