import {fireEvent, render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';

const mocks = vi.hoisted(() => ({
  hasSeen: vi.fn(),
  markSeen: vi.fn(),
}));

vi.mock('@/lib/onboarding', () => ({
  hasSeenOnboarding: mocks.hasSeen,
  markOnboardingSeen: mocks.markSeen,
}));

import {OnboardingIntro} from './OnboardingIntro';

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

  test.each([
    ['Fechar introdução'],
    ['Regras do poker'],
    ['Como funciona a mesa'],
  ])('persists dismissal through %s', name => {
    const {container} = render(<OnboardingIntro/>);
    fireEvent.click(screen.getByRole('button', {name}));
    expect(mocks.markSeen).toHaveBeenCalledOnce();
    expect(container).toBeEmptyDOMElement();
  });
});
