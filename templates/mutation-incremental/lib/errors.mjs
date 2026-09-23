// Exit codes are part of the CLI contract (spec §5.1): 2 usage/config, 3 lock held, 4 engine failure.
export class UsageError extends Error {
  constructor(message) {
    super(message);
    this.exitCode = 2;
  }
}

export class LockHeldError extends Error {
  constructor(message) {
    super(message);
    this.exitCode = 3;
  }
}

export class EngineError extends Error {
  constructor(message) {
    super(message);
    this.exitCode = 4;
  }
}

export class InterruptedError extends Error {
  constructor(message) {
    super(message);
    this.exitCode = 130;
  }
}
