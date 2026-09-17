import {render, screen} from '@testing-library/react';
import {describe, expect, test, vi} from 'vitest';
import AchievementsGuide from './achievements/page';
import BasicsGuide from './basics/page';
import CommunityGuide from './community/page';
import HandsGuide from './hands/page';
import ProfileGuide from './profile/page';
import StoreGuide from './store/page';
import TableGuide from './table/page';
import {GUIDE_TOPICS} from '@/components/guide/GuidePage';

vi.mock('@/lib/auth/session', () => ({useOptionalSession: () => ({authed: true, checking: false})}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
vi.mock('@/lib/hooks/useSocialUnread', () => ({useSocialUnread: () => 0}));
vi.mock('@/components/social/PeopleNavBadge', () => ({PeopleNavBadge: () => <span>people-badge</span>}));
vi.mock('next/image', () => ({
  default: ({alt}: {alt: string}) => alt
    ? <div role="img" aria-label={alt}/>
    : <div aria-hidden="true"/>,
}));

const topics = [
  {href: '/guide/basics', Page: BasicsGuide, title: 'Do lobby à primeira mão'},
  {href: '/guide/table', Page: TableGuide},
  {href: '/guide/hands', Page: HandsGuide},
  {href: '/guide/achievements', Page: AchievementsGuide},
  {href: '/guide/store', Page: StoreGuide},
  {href: '/guide/profile', Page: ProfileGuide},
  {href: '/guide/community', Page: CommunityGuide},
];

describe('guide topic pages', () => {
  test.each(topics)('$href renders its chrome, topic nav and section index', ({href, Page}) => {
    render(<Page/>);
    const topicNav = screen.getByRole('navigation', {name: 'Tópicos do guia'});
    expect(topicNav.querySelectorAll('a')).toHaveLength(GUIDE_TOPICS.length);
    expect(topicNav.querySelector(`a[href="${href}"]`)).toHaveAttribute('aria-current', 'page');
    const mobileTopics = screen.getByRole('navigation', {name: 'Tópicos do guia em telas pequenas'});
    expect(mobileTopics.querySelectorAll('a')).toHaveLength(GUIDE_TOPICS.length);
    expect(mobileTopics.querySelector(`a[href="${href}"]`)).toHaveAttribute('aria-current', 'page');
    expect(document.querySelector('.guide-page-index-mobile')).toBeInTheDocument();
    expect(screen.getByRole('navigation', {name: 'Nesta página'})).toBeInTheDocument();
    // Every "Nesta página" entry must anchor to a section actually rendered below it.
    const anchors = Array.from(document.querySelectorAll<HTMLAnchorElement>('.guide-on-this-page a'));
    expect(anchors.length).toBeGreaterThan(0);
    for (const anchor of anchors) {
      expect(document.querySelector(`#${anchor.getAttribute('href')!.slice(1)}`)).toBeInTheDocument();
    }
    expect(screen.getAllByRole('link', {name: /Todos os guias/}).length).toBeGreaterThan(0);
  });

  test('every topic in the shared nav has a page and the chain links forward', () => {
    expect(GUIDE_TOPICS.map(topic => topic.href).sort()).toEqual(topics.map(topic => topic.href).sort());
    render(<BasicsGuide/>);
    expect(screen.getByRole('heading', {level: 1, name: 'Do lobby à primeira mão'})).toBeInTheDocument();
    expect(screen.getByRole('link', {name: 'Conhecer a mesa'})).toHaveAttribute('href', '/guide/table');
  });

  test('the last topic ends the chain without a next link', () => {
    render(<CommunityGuide/>);
    expect(document.querySelector('.guide-topic-footer')!.querySelectorAll('a')).toHaveLength(1);
  });

  test('the profile topic documents the profile route, not the popover editor it replaced', () => {
    render(<ProfileGuide/>);
    expect(screen.getByRole('link', {name: /Abrir meu perfil/})).toHaveAttribute('href', '/player-profile');
    for (const label of ['Editar perfil', 'Salvar nome', 'Salvar vitrine', 'Ver como visitante', 'Copiar link']) {
      expect(document.body.textContent).toContain(label);
    }
    // The dialog and its entry point are gone from the product; the guide may
    // not keep sending players to them.
    for (const gone of ['Vitrine do perfil', 'Editar minha vitrine', 'Personalizar ordem e seções']) {
      expect(document.body.textContent).not.toContain(gone);
    }
  });

  test.each(topics)('$href keeps the shipped copy rules', ({Page}) => {
    render(<Page/>);
    const text = document.body.textContent!;
    expect(text).not.toMatch(/sandbox/i);
    expect(text).not.toMatch(/[—–]/);
  });

  test('renders illustrations with descriptive alt text', () => {
    render(<TableGuide/>);
    for (const image of screen.getAllByRole('img')) {
      expect(image.getAttribute('aria-label')!.length).toBeGreaterThan(10);
    }
  });
});
