#!/usr/bin/env node
// Entry point (spec §5.1). All logic lives in lib/main.mjs so it can be tested in-process.
import { main } from './lib/main.mjs';

process.exitCode = await main(process.argv.slice(2));
