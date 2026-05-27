#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');
const childProcess = require('child_process');

const root = path.resolve(__dirname, '..');
const packagedBinary = path.join(root, 'bin', 'metareview');
const goEntrypoint = path.join(root, 'cmd', 'metareview');
const args = process.argv.slice(2);

let command;
let commandArgs;
let options = { cwd: process.cwd(), stdio: 'inherit' };

if (fs.existsSync(packagedBinary)) {
  command = packagedBinary;
  commandArgs = args;
} else if (fs.existsSync(goEntrypoint)) {
  command = 'go';
  commandArgs = ['run', './cmd/metareview', ...args];
  options = { cwd: root, stdio: 'inherit' };
} else {
  console.error('No packaged metareview binary or Go source checkout found.');
  console.error(`Expected packaged binary: ${packagedBinary}`);
  console.error(`Expected Go source entrypoint: ${goEntrypoint}`);
  console.error('Install a packaged metareview release or run from a source checkout with Go 1.22+ available.');
  process.exit(1);
}

const result = childProcess.spawnSync(command, commandArgs, options);

if (result.error) {
  if (command === 'go' && result.error.code === 'ENOENT') {
    console.error('Go runtime not found. Install Go 1.22+ or use a packaged metareview release with bin/metareview.');
  } else {
    console.error(result.error.message);
  }
  process.exit(1);
}

process.exit(result.status === null ? 1 : result.status);
