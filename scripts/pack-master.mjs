#!/usr/bin/env node
/**
 * Source code packager for a standalone open-source release.
 *
 * Excludes local secrets, runtime state, caches, dependencies, and build
 * artifacts while preserving the source tree and documentation needed to
 * build the project from a clean checkout.
 */

import { execSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, '..');
const DIST_DIR = path.join(ROOT, 'dist');
const OUTPUT_ZIP = path.join(DIST_DIR, 'chatloop-source.zip');
const STAGING_DIR = path.join(os.tmpdir(), `chatloop-pack-${Date.now()}`);
const IS_WIN = process.platform === 'win32';

const BLACKLIST_PATTERNS = [
  /^\.env$/,
  /^\.env\.local$/,
  /^\.env\.(?!example).+$/,
  /\.db(-wal|-shm)?$/i,
  /\.sqlite3?$/i,
  /node_modules/i,
  /\.git($|\/|\\)/,
  /\.tmp($|\/|\\)/,
  /(^|[/\\])tmp($|[/\\])/,
  /(^|[/\\])dist($|[/\\])/,
  /\.DS_Store$/i,
  /Thumbs\.db$/i,
  /\.log$/i,
  /AGENTS\.md$/i,
  /SESSION-HANDOFF\.md$/i,
  /backend[/\\]data[/\\]media/i,
];

function shouldExclude(relPath) {
  const normalized = relPath.replace(/\\/g, '/');
  return BLACKLIST_PATTERNS.some((pattern) => pattern.test(normalized) || pattern.test(path.basename(normalized)));
}

function copyRecursively(srcDir, destDir, currentRel = '') {
  const entries = fs.readdirSync(srcDir, { withFileTypes: true });
  for (const entry of entries) {
    const relPath = path.join(currentRel, entry.name);
    const srcPath = path.join(srcDir, entry.name);
    const destPath = path.join(destDir, entry.name);
    if (shouldExclude(relPath)) continue;
    if (entry.isDirectory()) {
      fs.mkdirSync(destPath, { recursive: true });
      copyRecursively(srcPath, destPath, relPath);
    } else if (entry.isFile()) {
      fs.copyFileSync(srcPath, destPath);
    }
  }
}

function ensureEmptyDir(dir) {
  if (fs.existsSync(dir)) fs.rmSync(dir, { recursive: true, force: true });
  fs.mkdirSync(dir, { recursive: true });
}

function createZipFromStaging() {
  if (IS_WIN) {
    const ps = `Compress-Archive -Path '${STAGING_DIR}\\*' -DestinationPath '${OUTPUT_ZIP}' -Force`;
    execSync(`powershell -NoProfile -Command "${ps}"`, { stdio: 'inherit' });
  } else {
    execSync(`cd "${STAGING_DIR}" && zip -r "${OUTPUT_ZIP}" .`, { stdio: 'ignore' });
  }
}

function verifyZip() {
  let files = [];
  if (IS_WIN) {
    const ps = `Add-Type -AssemblyName System.IO.Compression.FileSystem; [System.IO.Compression.ZipFile]::OpenRead('${OUTPUT_ZIP}').Entries.FullName`;
    files = execSync(`powershell -NoProfile -Command "${ps}"`, { encoding: 'utf-8' }).split(/\r?\n/).map((s) => s.trim()).filter(Boolean);
  } else {
    files = execSync(`unzip -Z1 "${OUTPUT_ZIP}"`, { encoding: 'utf-8' }).split('\n').map((s) => s.trim()).filter(Boolean);
  }

  const leaks = files.filter(shouldExclude);
  if (leaks.length) {
    fs.unlinkSync(OUTPUT_ZIP);
    throw new Error(`Verifikasi gagal: ${leaks.join(', ')}`);
  }
  console.log(`✓ Source archive verified (${files.length} files).`);
}

function pack() {
  fs.mkdirSync(DIST_DIR, { recursive: true });
  if (fs.existsSync(OUTPUT_ZIP)) fs.unlinkSync(OUTPUT_ZIP);
  ensureEmptyDir(STAGING_DIR);
  copyRecursively(ROOT, STAGING_DIR);
  createZipFromStaging();
  fs.rmSync(STAGING_DIR, { recursive: true, force: true });
  verifyZip();
  const stat = fs.statSync(OUTPUT_ZIP);
  console.log(`✓ Created ${OUTPUT_ZIP} (${(stat.size / 1048576).toFixed(2)} MB).`);
}

try {
  pack();
} catch (error) {
  console.error(`✗ Packaging failed: ${error.message}`);
  process.exit(1);
}
