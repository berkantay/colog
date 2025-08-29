#!/usr/bin/env node

const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

// Supported platforms and architectures
const targets = [
  { os: 'darwin', arch: 'amd64' },
  { os: 'darwin', arch: 'arm64' },
  { os: 'linux', arch: 'amd64' },
  { os: 'linux', arch: 'arm64' },
  { os: 'windows', arch: 'amd64' },
  { os: 'windows', arch: 'arm64' }
];

// Create binaries directory
const binariesDir = path.join(__dirname, '..', 'binaries');
if (!fs.existsSync(binariesDir)) {
  fs.mkdirSync(binariesDir, { recursive: true });
}

console.log('🔨 Building binaries for all platforms...\n');

for (const target of targets) {
  const { os: goos, arch: goarch } = target;
  let binaryName = `colog-mcp-${goos}-${goarch}`;
  
  if (goos === 'windows') {
    binaryName += '.exe';
  }
  
  const binaryPath = path.join(binariesDir, binaryName);
  
  console.log(`📦 Building ${binaryName}...`);
  
  try {
    execSync(`go build -o "${binaryPath}" ./cmd/colog-mcp`, {
      cwd: path.join(__dirname, '..'),
      env: {
        ...process.env,
        GOOS: goos,
        GOARCH: goarch,
        CGO_ENABLED: '0' // Disable CGO for static binaries
      },
      stdio: 'inherit'
    });
    
    console.log(`✅ Built ${binaryName}\n`);
  } catch (error) {
    console.error(`❌ Failed to build ${binaryName}:`, error.message);
    process.exit(1);
  }
}

console.log('🎉 All binaries built successfully!');

// List built files
const files = fs.readdirSync(binariesDir);
console.log('\nBuilt binaries:');
files.forEach(file => {
  const stats = fs.statSync(path.join(binariesDir, file));
  const sizeMB = (stats.size / 1024 / 1024).toFixed(1);
  console.log(`  ${file} (${sizeMB}MB)`);
});