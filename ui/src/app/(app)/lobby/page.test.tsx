import {render, screen, within} from '@testing-library/react';
import {expectNoAxeViolations} from '@/test/axe';
import {describe, expect, test, vi} from 'vitest';
import Lobby from './page';
vi.mock('@/lib/mockConfig', () => ({USE_MOCK: true}));
vi.mock('@/lib/api/dailyReward', () => ({
  getCooldown: vi.fn().mockResolvedValue({remaining_time_seconds: 0}),
}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: { children: React.ReactNode }) => children}));
vi.mock('@/components/lobby/StakesGrid', () => ({StakesGrid: () => <div>stakes-grid</div>}));
vi.mock('@/components/lobby/ActiveTableBanner', () => ({ActiveTableBanner: () => <div>active-table</div>}));
vi.mock('@/components/lobby/CreateRoomDialogTrigger', () => ({CreateRoomDialogTrigger: () => <div>create-room</div>}));
vi.mock('@/components/social/PeopleDrawer', () => ({PeopleDrawer: () => <div>people-drawer</div>}));
vi.mock('@/components/lobby/OnboardingIntro', () => ({OnboardingIntro: () => <div>onboarding</div>}));
vi.mock('@/components/lobby/ProfileMenu', () => ({ProfileMenu: () => <div>profile-menu</div>}));
vi.mock('@/lib/hooks/useSocialUnread', () => ({useSocialUnread: () => 0}));
vi.mock('@/components/social/PeopleNavBadge', () => ({PeopleNavBadge: () => <span>people-badge</span>}));
vi.mock('@/components/table/MockControls', () => ({MockControls: () => <div>mock-controls</div>}));

describe('lobby page', () => {
  test('keeps the lobby focused on tables and links to the dedicated credits hub', async () => {
    render(<Lobby/>);
    const heading = screen.getByRole('heading', {name: 'Escolha os blinds e o tamanho da mesa.'});
    expect(heading).toBeInTheDocument();
    expect(screen.getByText('Buscamos uma mesa pública com vaga para sua escolha; se não houver, criamos uma nova. Tudo com fichas sandbox.')).toBeInTheDocument();
    const activeTable = screen.getByText('active-table');
    const onboarding = screen.getByText('onboarding');
    expect(heading.compareDocumentPosition(activeTable) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(activeTable.compareDocumentPosition(onboarding) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getByText('stakes-grid')).toBeInTheDocument();
    expect(screen.getByText('people-drawer')).toBeInTheDocument();
    expect(screen.getByText('active-table')).toBeInTheDocument();
    const nav = screen.getByRole('navigation', {name: 'Navegação principal'});
    expect(within(nav).getByRole('link', {name: 'Lobby'})).toHaveAttribute('aria-current', 'page');
    expect(await within(nav).findByRole('link', {name: /Loja.*recompensa diária disponível/})).toHaveAttribute('href', '/store');
    expect(screen.queryByRole('button', {name: /Recompensa Diária/})).not.toBeInTheDocument();
    expect(await screen.findByText('mock-controls')).toBeInTheDocument();
  });

  // Issue #60: an automated floor under the a11y intent in ui/CLAUDE.md — a new
  // serious or critical axe violation on this route fails CI.
  test('is axe-clean', async () => {
    const {container} = render(<Lobby/>);
    await expectNoAxeViolations(container);
  });

});
