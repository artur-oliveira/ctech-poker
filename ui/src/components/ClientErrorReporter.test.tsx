import {render} from '@testing-library/react';
import {describe, expect, test, vi} from 'vitest';
import {ClientErrorReporter} from './ClientErrorReporter';

const mocks = vi.hoisted(() => ({install: vi.fn()}));
vi.mock('@/lib/telemetry', () => ({installGlobalErrorReporter: mocks.install}));

describe('ClientErrorReporter', () => {
  test('installs the global reporter and renders nothing', () => {
    const {container} = render(<ClientErrorReporter/>);
    expect(mocks.install).toHaveBeenCalledOnce();
    expect(container).toBeEmptyDOMElement();
  });
});
