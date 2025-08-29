#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const https = require('https');
const os = require('os');
const { execSync } = require('child_process');

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

// Create binaries directory
const binariesDir = path.join(__dirname, '..', 'binaries');
if (!fs.existsSync(binariesDir)) {
  fs.mkdirSync(binariesDir, { recursive: true });
}

const binaryPath = path.join(binariesDir, binaryName);

// Check if binary already exists
if (fs.existsSync(binaryPath)) {
  console.log(`✓ Binary already exists: ${binaryName}`);
  
  // Make executable on Unix systems
  if (platform !== 'win32') {
    fs.chmodSync(binaryPath, 0o755);
  }
  return;
}

console.error(`❌ Binary not found for ${platform}-${goArch}`);
console.error(`Expected: ${binaryPath}`);
console.error('');
console.error(`Platform ${platform}-${goArch} may not be supported.`);
console.error('Supported platforms:');
console.error('  - darwin-amd64, darwin-arm64');
console.error('  - linux-amd64, linux-arm64');
console.error('  - win32-amd64, win32-arm64');
process.exit(1);

// TODO: For production releases, download from GitHub:
/*
const downloadUrl = `https://github.com/berkantay/colog/releases/latest/download/${binaryName}`;

console.log(`📥 Downloading ${downloadUrl}...`);

const file = fs.createWriteStream(binaryPath);
https.get(downloadUrl, (response) => {
  if (response.statusCode !== 200) {
    console.error(`❌ Download failed: HTTP ${response.statusCode}`);
    console.error(`Platform ${platform}-${goArch} may not be supported`);
    process.exit(1);
  }
  
  response.pipe(file);
  
  file.on('finish', () => {
    file.close();
    console.log(`✅ Downloaded: ${binaryName}`);
    
    // Make executable on Unix systems
    if (platform !== 'win32') {
      fs.chmodSync(binaryPath, 0o755);
    }
  });
}).on('error', (err) => {
  fs.unlink(binaryPath, () => {}); // Delete partial file
  console.error(`❌ Download failed: ${err.message}`);
  process.exit(1);
});
*/