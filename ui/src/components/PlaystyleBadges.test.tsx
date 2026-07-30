import {fireEvent, render, screen} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {PlaystyleBadges} from './PlaystyleBadges';

describe('PlaystyleBadges', () => {
  test('reveals each explanation through a keyboard and touch compatible disclosure', () => {
    render(<PlaystyleBadges badges={[{key: 'selective'}]}/>);
    
    const summary = screen.getByText('Seletivo');
    expect(screen.getByText('VPIP de até 22%')).not.toBeVisible();
    
    fireEvent.click(summary);
    expect(screen.getByText('VPIP de até 22%')).toBeVisible();
    expect(summary.closest('details')).toHaveAttribute('open');
  });
  
  test('ignores unknown server badge keys', () => {
    const {container} = render(<PlaystyleBadges badges={[{key: 'future-style'}]}/>);
    expect(container.querySelector('.poker-style-badges')).toBeEmptyDOMElement();
  });
});
