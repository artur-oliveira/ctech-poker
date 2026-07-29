/**
 * Production replacement for the local simulator. Reaching this module means
 * a compile-time guard was bypassed, so fail closed instead of simulating API
 * or realtime behavior.
 */
export async function mockAdapter(): Promise<never> {
  throw new Error('Development adapter is unavailable in production.');
}

export class MockTableService {
  connect(): never {
    throw new Error('Development realtime is unavailable in production.');
  }
  
  close() {
    // No resources can be opened by this production replacement.
  }
  
  send() {
    return false;
  }
  
  reconnect() {
    // No connection exists in production.
  }
}
