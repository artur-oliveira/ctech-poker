import {render} from '@testing-library/react';
import {describe, expect, test} from 'vitest';

import {PerimeterTimer} from './PerimeterTimer';

describe('PerimeterTimer', () => {
  test('renders the configured duration and elapsed animation offset', () => {
    const {container} = render(
      <PerimeterTimer className="player-clock" durationMs={10_000} elapsedMs={2500}
                      restartKey="turn-1" radius={16}/>,
    );
    const timer = container.querySelector('svg')!;
    const border = container.querySelector('rect')!;
    
    expect(timer).toHaveClass('perimeter-timer', 'player-clock');
    expect(timer).toHaveStyle({animationDuration: '10000ms', animationDelay: '-2500ms'});
    expect(timer).toHaveAttribute('aria-hidden', 'true');
    expect(timer).toHaveAttribute('focusable', 'false');
    expect(border).toHaveAttribute('rx', '16');
    expect(border).toHaveAttribute('ry', '16');
  });
  
  test('clamps elapsed time within the animation duration', () => {
    const {container, rerender} = render(
      <PerimeterTimer className="clock" durationMs={5000} elapsedMs={-100}
                      restartKey="negative" radius={8}/>,
    );
    expect(container.querySelector('svg')).toHaveStyle({animationDelay: '0ms'});
    
    rerender(<PerimeterTimer className="clock" durationMs={5000} elapsedMs={9000}
                             restartKey="overflow" radius={8}/>);
    expect(container.querySelector('svg')).toHaveStyle({animationDelay: '-5000ms'});
  });
});
