import { spawn } from 'node:child_process';

// Spec §5.6 step 4: on a timeout or an interrupt the group gets SIGTERM, then SIGKILL after 30 s.
export const KILL_GRACE_MS = 30_000;

// One interrupt per command run: signal handlers trigger it, running children subscribe to it.
export function createInterrupt() {
  const listeners = new Set();
  const interrupt = {
    interrupted: false,
    onInterrupt(fn) {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    trigger() {
      interrupt.interrupted = true;
      for (const fn of [...listeners]) fn();
    },
  };
  return interrupt;
}

// Spec §5.6: SIGINT, SIGTERM and SIGHUP to the harness stop the running child and the run.
export function installSignalHandlers(interrupt) {
  const signals = ['SIGINT', 'SIGTERM', 'SIGHUP'];
  const handler = () => interrupt.trigger();
  for (const s of signals) process.on(s, handler);
  return () => {
    for (const s of signals) process.off(s, handler);
  };
}

// Signals a whole process group; false when it no longer exists.
export function signalGroup(pid, signal) {
  try {
    process.kill(-pid, signal);
    return true;
  } catch {
    return false;
  }
}

const NOT_RUN = { code: null, signal: null, timedOut: false, interrupted: true, error: null };

// Runs argv without a shell in its own process group (detached), forwarding its output. A spawn
// failure resolves with `error` set (Node may also emit 'close'; the promise settles once).
export function spawnGroup(argv, { cwd, env, timeoutMs, graceMs, interrupt, onOutput }) {
  if (interrupt.interrupted) return Promise.resolve(NOT_RUN);
  return new Promise((settle) => {
    const child = spawn(argv[0], argv.slice(1), { cwd, env, detached: true, stdio: ['ignore', 'pipe', 'pipe'] });
    let timedOut = false;
    let interrupted = false;
    let killTimer = null;
    const stop = () => {
      signalGroup(child.pid, 'SIGTERM');
      killTimer = setTimeout(() => signalGroup(child.pid, 'SIGKILL'), graceMs);
    };
    const timer = timeoutMs === null ? null : setTimeout(() => {
      timedOut = true;
      stop();
    }, timeoutMs);
    const unsubscribe = interrupt.onInterrupt(() => {
      interrupted = true;
      stop();
    });
    child.stdout.on('data', onOutput);
    child.stderr.on('data', onOutput);
    const finish = (code, signal, error) => {
      clearTimeout(timer);
      clearTimeout(killTimer);
      unsubscribe();
      settle({ code, signal, timedOut, interrupted, error });
    };
    child.on('error', (error) => finish(null, null, error));
    child.on('close', (code, signal) => finish(code, signal, null));
  });
}
