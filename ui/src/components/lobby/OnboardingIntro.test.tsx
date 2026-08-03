import {fireEvent, render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {OnboardingIntro} from './OnboardingIntro';

const mocks = vi.hoisted(() => ({
  hasSeen: vi.fn(),
  markSeen: vi.fn(),
}));

vi.mock('@/lib/onboarding', () => ({
  hasSeenOnboarding: mocks.hasSeen,
  markOnboardingSeen: mocks.markSeen,
}));

describe('OnboardingIntro', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.hasSeen.mockReturnValue(false);
  });
  
  test('does not render for a returning player', () => {
    mocks.hasSeen.mockReturnValue(true);
    const {container} = render(<OnboardingIntro/>);
    expect(container).toBeEmptyDOMElement();
  });
  
  test('keeps table entry primary and persists dismissal', () => {
    const {container} = render(<OnboardingIntro/>);
    expect(screen.getByText('Escolha os blinds e o formato abaixo. Você joga com fichas sandbox e pode entrar sem tutorial.')).toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Fechar introdução'}));
    expect(mocks.markSeen).toHaveBeenCalledOnce();
    expect(container).toBeEmptyDOMElement();
  });
});
