#!/usr/bin/env node

const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');
const os = require('os');

// Determine platform and architecture
const platform = os.platform();
const arch = os.arch();

// Map Node.js arch to Go arch
const archMap = {
  'x64': 'amd64',
  'arm64': 'arm64'
};

const goArch = archMap[arch] || arch;

// Determine binary name
let binaryName = `colog-mcp-${platform}-${goArch}`;
if (platform === 'win32') {
  binaryName += '.exe';
}

// Binary path
const binaryPath = path.join(__dirname, '..', 'binaries', binaryName);

// Check if binary exists
if (!fs.existsSync(binaryPath)) {
  console.error(`Binary not found for ${platform}-${goArch}: ${binaryPath}`);
  console.error('Please run: npm run postinstall');
  process.exit(1);
}

// Make binary executable (Unix systems)
if (platform !== 'win32') {
  try {
    fs.chmodSync(binaryPath, 0o755);
  } catch (err) {
    console.warn('Warning: Could not make binary executable:', err.message);
  }
}

// Spawn the binary with all arguments
const child = spawn(binaryPath, process.argv.slice(2), {
  stdio: 'inherit',
  env: process.env
});

child.on('error', (err) => {
  console.error('Failed to start colog-mcp:', err.message);
  process.exit(1);
});

child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
  } else {
    process.exit(code || 0);
  }
});