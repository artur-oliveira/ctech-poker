import {ILocalBundling} from "aws-cdk-lib";
import {execFileSync} from "child_process";

export const localGoBundling = (cwd: string): ILocalBundling => {
  return {
    tryBundle(outputDir: string): boolean {
      try {
        execFileSync('go', ['build', '-o', `${outputDir}/bootstrap`, '.'], {
          cwd: '../api/cmd/archiver',
          env: {...process.env, GOOS: 'linux', GOARCH: 'arm64', CGO_ENABLED: '0'},
          stdio: 'inherit',
        });
        return true;
      } catch {
        return false; // no local Go toolchain — fall back to Docker bundling
      }
    },
  }
}