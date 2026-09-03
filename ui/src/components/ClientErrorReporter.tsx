'use client';
import {useEffect} from 'react';
import {installGlobalErrorReporter} from '@/lib/telemetry';

/**
 * Mounted once by the root layout so uncaught errors and unhandled rejections
 * reach `/v1.0/client-errors` (Issue #53). It renders nothing: the listeners
 * must be installed from the browser, and this is a static export, so the root
 * layout itself only ever runs at build time.
 */
export function ClientErrorReporter() {
  useEffect(installGlobalErrorReporter, []);
  return null;
}
