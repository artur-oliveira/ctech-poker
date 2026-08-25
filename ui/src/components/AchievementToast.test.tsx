import {act, render, screen} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';

import {AchievementToast} from './AchievementToast';

describe('AchievementToast', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());
  
  test('renders a localized unlock and pluralized star count', () => {
    render(<AchievementToast unlock={{key: 'wins', stars: 3}}/>);
    
    expect(screen.getByText('CONQUISTA DESBLOQUEADA')).toBeInTheDocument();
    expect(screen.getByText('Vitórias')).toBeInTheDocument();
    expect(screen.getByText('3 estrelas')).toBeInTheDocument();
    expect(screen.getAllByText('★')).toHaveLength(3);
  });
  
  test('falls back to a readable key and uses singular for one star', () => {
    render(<AchievementToast unlock={{key: 'custom_unlock', stars: 1}}/>);
    expect(screen.getByText('custom unlock')).toBeInTheDocument();
    expect(screen.getByText('1 estrela')).toBeInTheDocument();
  });
  
  test('animates out and clears after the hold period', () => {
    const {container} = render(<AchievementToast unlock={{key: 'wins', stars: 1}}/>);
    
    act(() => vi.advanceTimersByTime(4200));
    expect(container.querySelector('.achievement-toast')).toHaveClass('leaving');
    
    act(() => vi.advanceTimersByTime(350));
    expect(container).toBeEmptyDOMElement();
  });
  
  test('restarts the lifecycle when another unlock arrives', () => {
    const {container, rerender} = render(<AchievementToast unlock={{key: 'wins', stars: 1}}/>);
    act(() => vi.advanceTimersByTime(4200));
    expect(container.querySelector('.achievement-toast')).toHaveClass('leaving');
    
    rerender(<AchievementToast unlock={{key: 'bluff', stars: 2}}/>);
    expect(screen.getByText('Mestre do Blefe')).toBeInTheDocument();
    expect(container.querySelector('.achievement-toast')).not.toHaveClass('leaving');
    
    act(() => vi.advanceTimersByTime(4550));
    expect(container).toBeEmptyDOMElement();
  });

  test('queues an unlock while an outcome decision owns the announcement layer', () => {
    const {rerender} = render(<AchievementToast unlock={{key: 'wins', stars: 1}} blocked/>);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();

    rerender(<AchievementToast unlock={{key: 'wins', stars: 1}} blocked={false}/>);
    expect(screen.getByRole('status')).toHaveTextContent('Vitórias');
  });

  test('removes a visible toast when blocked and restores the queued unlock afterwards', () => {
    const {rerender} = render(<AchievementToast unlock={{key: 'bluff', stars: 2}}/>);
    expect(screen.getByRole('status')).toHaveTextContent('Mestre do Blefe');

    rerender(<AchievementToast unlock={{key: 'bluff', stars: 2}} blocked/>);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    rerender(<AchievementToast unlock={null} blocked={false}/>);
    expect(screen.getByRole('status')).toHaveTextContent('Mestre do Blefe');
  });

  test('renders nothing when there is no current or queued unlock', () => {
    const {container, rerender} = render(<AchievementToast unlock={null}/>);
    expect(container).toBeEmptyDOMElement();
    rerender(<AchievementToast unlock={null} blocked/>);
    expect(container).toBeEmptyDOMElement();
    rerender(<AchievementToast unlock={null} blocked={false}/>);
    expect(container).toBeEmptyDOMElement();
  });

  test('queues the visible toast itself when blocking begins without a replacement unlock', () => {
    const {rerender} = render(<AchievementToast unlock={{key: 'wins', stars: 1}}/>);
    expect(screen.getByRole('status')).toHaveTextContent('Vitórias');

    rerender(<AchievementToast unlock={null} blocked/>);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    rerender(<AchievementToast unlock={null} blocked={false}/>);
    expect(screen.getByRole('status')).toHaveTextContent('Vitórias');
  });

  test('retains the same visible unlock without restarting its lifecycle', () => {
    const unlock = {key: 'wins', stars: 2};
    const {rerender} = render(<AchievementToast unlock={unlock}/>);
    const toast = screen.getByRole('status');
    rerender(<AchievementToast unlock={{...unlock}}/>);
    expect(screen.getByRole('status')).toBe(toast);
  });
});
