import {fireEvent, render, screen} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';

const {replace, searchParams} = vi.hoisted(() => ({
  replace: vi.fn(),
  searchParams: {toString: vi.fn(() => 'keep=value')},
}));

vi.mock('next/navigation', () => ({
  useRouter: () => ({replace}),
  useSearchParams: () => searchParams,
}));

import {MockControls} from './MockControls';

describe('MockControls', () => {
  beforeEach(() => vi.clearAllMocks());

  test('updates scenario and latency while preserving existing query parameters', () => {
    render(<MockControls scenario="waiting" delay={350}/>);

    fireEvent.change(screen.getByLabelText('Cena'), {target: {value: 'river'}});
    expect(replace).toHaveBeenLastCalledWith('?keep=value&scenario=river', {scroll: false});

    fireEvent.change(screen.getByLabelText('Latência'), {target: {value: '1200'}});
    expect(replace).toHaveBeenLastCalledWith('?keep=value&delay=1200', {scroll: false});
  });

  test('hides optional controls when their values are absent', () => {
    render(<MockControls/>);
    expect(screen.queryByLabelText('Cena')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Latência')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Simular erro (toda requisição)')).toBeInTheDocument();
  });

  test('loads, changes and clears the global REST failure simulation', () => {
    window.localStorage.setItem('ctech_poker_mock_errors', JSON.stringify({'* *': {status: 429}}));
    const {unmount} = render(<MockControls/>);
    const error = screen.getByLabelText('Simular erro (toda requisição)') as HTMLSelectElement;
    expect(error.value).toBe('429');

    fireEvent.change(error, {target: {value: 'network'}});
    expect(JSON.parse(window.localStorage.getItem('ctech_poker_mock_errors')!))
      .toEqual({'* *': {status: 0}});

    fireEvent.change(error, {target: {value: '500'}});
    expect(JSON.parse(window.localStorage.getItem('ctech_poker_mock_errors')!))
      .toEqual({'* *': {status: 500}});

    fireEvent.change(error, {target: {value: ''}});
    expect(window.localStorage.getItem('ctech_poker_mock_errors')).toBeNull();
    unmount();
  });

  test('recovers from malformed persisted error configuration', () => {
    window.localStorage.setItem('ctech_poker_mock_errors', '{invalid');
    render(<MockControls/>);
    expect(screen.getByLabelText<HTMLSelectElement>('Simular erro (toda requisição)').value).toBe('');
  });
});
