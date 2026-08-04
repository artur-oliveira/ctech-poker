import {fireEvent, render, screen} from '@testing-library/react';
import {describe, expect, test, vi} from 'vitest';
import {FilterGroup} from './FilterGroup';

describe('FilterGroup', () => {
  test('exposes filter buttons as one named group with a pressed state', () => {
    const onChange = vi.fn();
    render(<FilterGroup
      label="Filtro de exemplo"
      value="all"
      options={[
        {value: 'all', label: 'Todas'},
        {value: 'wins', label: 'Vitórias'}
      ]}
      onChangeAction={onChange}
    />);
    
    expect(screen.getByRole('group', {name: 'Filtro de exemplo'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Todas'})).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', {name: 'Vitórias'})).toHaveAttribute('aria-pressed', 'false');
    
    fireEvent.click(screen.getByRole('button', {name: 'Vitórias'}));
    expect(onChange).toHaveBeenCalledWith('wins');
  });
});
