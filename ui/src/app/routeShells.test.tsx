import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import type {Metadata} from 'next';
import AchievementsLayout, {metadata as achievementsMetadata} from './achievements/layout';
import CallbackLayout, {metadata as callbackMetadata} from './callback/layout';
import GuideLayout, {metadata as guideMetadata} from './guide/layout';
import HandsLayout, {metadata as handsMetadata} from './hands/layout';
import HandsHistoryLayout, {metadata as handsHistoryMetadata} from './hands/history/layout';
import HandsReplayLayout, {metadata as handsReplayMetadata} from './hands/replay/layout';
import LeaderboardLayout, {metadata as leaderboardMetadata} from './leaderboard/layout';
import LobbyLayout, {metadata as lobbyMetadata} from './lobby/layout';
import PokerRulesLayout, {metadata as pokerRulesMetadata} from './poker-rules/layout';
import ProfileLayout, {metadata as profileMetadata} from './profile/layout';
import ShareLayout, {metadata as shareMetadata} from './share/layout';
import TableLayout, {metadata as tableMetadata} from './table/layout';
import ErrorPage from './error';
import GlobalError from './global-error';
import NotFoundPage from './not-found';
import UnavailablePage, {metadata as unavailableMetadata} from './unavailable/page';
import {OG_PREVIEWS} from '@/lib/ogPreviews';

const layouts = [
  {path: '/achievements', Layout: AchievementsLayout, metadata: achievementsMetadata, indexable: false},
  {path: '/guide', Layout: GuideLayout, metadata: guideMetadata, indexable: true},
  {path: '/hands', Layout: HandsLayout, metadata: handsMetadata, indexable: false},
  {path: '/hands/history', Layout: HandsHistoryLayout, metadata: handsHistoryMetadata, indexable: false},
  {path: '/hands/replay', Layout: HandsReplayLayout, metadata: handsReplayMetadata, indexable: false},
  {path: '/leaderboard', Layout: LeaderboardLayout, metadata: leaderboardMetadata, indexable: false},
  {path: '/lobby', Layout: LobbyLayout, metadata: lobbyMetadata, indexable: false},
  {path: '/poker-rules', Layout: PokerRulesLayout, metadata: pokerRulesMetadata, indexable: true},
  {path: '/profile', Layout: ProfileLayout, metadata: profileMetadata, indexable: true},
  {path: '/share', Layout: ShareLayout, metadata: shareMetadata, indexable: false},
  {path: '/table', Layout: TableLayout, metadata: tableMetadata, indexable: false},
];

const robotsOf = (metadata: Metadata) => metadata.robots as {index: boolean; follow: boolean};

describe('route layouts', () => {
  test.each(layouts)('$path passes children through untouched', ({Layout}) => {
    render(<Layout><p>route content</p></Layout>);
    expect(screen.getByText('route content')).toBeInTheDocument();
  });

  test.each(layouts)('$path declares canonical, OG and Twitter metadata', ({path, metadata}) => {
    expect(metadata.alternates?.canonical).toBe(path);
    expect(metadata.openGraph?.title).toBe(`${metadata.title} · CTech Poker`);
    expect(metadata.openGraph?.description).toBe(metadata.description);
    expect(metadata.twitter?.title).toBe(`${metadata.title} · CTech Poker`);
    const images = metadata.openGraph?.images as {url: string; width: number; height: number}[];
    expect(images).toHaveLength(2);
    expect(images[0].url).toMatch(/^\/og\/[a-z-]+\.webp$/);
    expect(images.every(image => image.width === 1200 && image.height === 630)).toBe(true);
  });

  test.each(layouts)('$path only opts into indexing when the content is public', ({metadata, indexable}) => {
    expect(robotsOf(metadata)).toEqual({index: indexable, follow: indexable});
  });

  test('every route OG image has a matching capture entry', () => {
    const slugs = new Set(OG_PREVIEWS.map(preview => preview.slug));
    for (const {metadata} of layouts) {
      const images = metadata.openGraph?.images as {url: string}[];
      expect(slugs).toContain(images[0].url.replace('/og/', '').replace('.webp', ''));
    }
  });

  test('the callback shell stays private and renders the exchange screen', () => {
    render(<CallbackLayout><p>trocando código</p></CallbackLayout>);
    expect(screen.getByText('trocando código')).toBeInTheDocument();
    expect(callbackMetadata.robots).toEqual({index: false, follow: false});
  });
});

describe('system state routes', () => {
  test('the error boundary reports the failure and offers a retry', async () => {
    const reset = vi.fn();
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    const error = Object.assign(new Error('boom'), {digest: 'abc123'});
    render(<ErrorPage error={error} reset={reset}/>);

    expect(consoleError).toHaveBeenCalledWith(error);
    expect(screen.getByText('Referência do erro: abc123')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Tentar novamente/}));
    expect(reset).toHaveBeenCalledOnce();
  });

  test('the error boundary falls back to generic guidance without a digest', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    render(<ErrorPage error={new Error('boom')} reset={vi.fn()}/>);
    expect(screen.getByText('Tente carregar esta tela novamente.')).toBeInTheDocument();
  });

  test('the root boundary still offers recovery when the provider tree crashes', async () => {
    const reset = vi.fn();
    vi.spyOn(console, 'error').mockImplementation(() => {});
    render(<GlobalError error={Object.assign(new Error('root boom'), {digest: 'root123'})} reset={reset}/>);
    expect(screen.getByText('Referência do erro: root123')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Tentar novamente/}));
    expect(reset).toHaveBeenCalledOnce();
  });

  test('the maintenance page offers a health-checked retry and stays out of the index', () => {
    render(<UnavailablePage/>);
    expect(screen.getByText('503')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Tentar novamente/})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Ir para o início/})).toHaveAttribute('href', '/');
    expect(unavailableMetadata.robots).toEqual({index: false, follow: false});
  });

  test('the not-found page routes players back to the lobby', () => {
    render(<NotFoundPage/>);
    expect(screen.getByText('404')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Voltar ao lobby/})).toHaveAttribute('href', '/lobby');
  });
});
