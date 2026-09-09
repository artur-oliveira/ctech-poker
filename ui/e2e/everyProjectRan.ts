import type {FullConfig, FullResult, Reporter, Suite} from '@playwright/test/reporter';

/** Fails the run when a configured project contributed no tests at all.
 *
 * A project that executes nothing does not look like a failure from the
 * outside: the summary still ends in a passing count for the projects that did
 * run. A `--project` typo, a `testDir` that stops matching and a project left
 * out of a shard all land here, and each one silently narrows the engines a
 * change is gated in — which is how a WebKit-only regression reaches main. */

/** CLI flags that legitimately run a subset of the configured projects. */
const NARROWING_FLAGS = ['--project', '-p', '--shard'];

export default class EveryProjectRan implements Reporter {
  private missing: string[] = [];

  onBegin(config: FullConfig, suite: Suite) {
    // A reporter is not told which CLI filters were applied, and `config.projects`
    // is the full configured set either way — so deliberate narrowing has to be
    // read off argv. Without it, every `--project=chromium` run would "fail".
    if (NARROWING_FLAGS.some(flag => process.argv.some(arg => arg === flag || arg.startsWith(`${flag}=`)))) return;
    const ran = new Set(suite.allTests().map(test => test.parent.project()?.name));
    this.missing = config.projects.map(project => project.name).filter(name => name && !ran.has(name));
  }

  async onEnd(result: FullResult): Promise<{status: FullResult['status']}> {
    if (!this.missing.length) return {status: result.status};
    console.error(`\nNo tests ran for: ${this.missing.join(', ')}.` +
      '\nA project that executes nothing is a silent skip, not a pass.');
    return {status: result.status === 'passed' ? 'failed' : result.status};
  }
}
