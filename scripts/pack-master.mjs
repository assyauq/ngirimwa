#!/usr/bin/env node
/**
 * Master Source Code Packager for Members / LMS
 *
 * Menghasilkan file ZIP distribusi yang 100% bersih & aman untuk member:
 * - Tanpa .env (hanya menyertakan .env.example)
 * - Tanpa file sesi database (*.db, *.db-wal, *.db-shm, sqlite)
 * - Tanpa session media WhatsApp & file machine ID
 * - Tanpa node_modules
 * - Tanpa folder git / cache / debug log (.tmp, .DS_Store, Thumbs.db)
 * - Tanpa compiled binary (backend/tmp/main, dist)
 * - Tanpa file internal dev (AGENTS.md, docs internal)
 */

import { execSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, '..');
const DIST_DIR = path.join(ROOT, 'dist');
const OUTPUT_ZIP = path.join(DIST_DIR, 'chatloop-member.zip');
const STAGING_DIR = path.join(os.tmpdir(), `chatloop-pack-${Date.now()}`);
const IS_WIN = process.platform === 'win32';

function log(msg) {
  console.log(msg);
}

// Pola file/folder yang DILARANG masuk ke zip member
const BLACKLIST_PATTERNS = [
  /^\.env$/,
  /^\.env\.local$/,
  /^\.env\.(?!example).+$/, // Menolak .env.production, .env.development, dll, tapi mengizinkan .env.example
  /\.db(-wal|-shm)?$/i,
  /\.sqlite3?$/i,
  /^\.license-machine-id$/,
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
  /generate_marketing_strategy_pdf\.py$/i,
  /backend[/\\]data[/\\]media/i,
  /backend[/\\]data[/\\]\.license-machine-id/i,
];

function shouldExclude(relPath) {
  const normalized = relPath.replace(/\\/g, '/');
  for (const pattern of BLACKLIST_PATTERNS) {
    if (pattern.test(normalized) || pattern.test(path.basename(normalized))) {
      return true;
    }
  }
  return false;
}

function copyRecursively(srcDir, destDir, currentRel = '') {
  const entries = fs.readdirSync(srcDir, { withFileTypes: true });
  for (const entry of entries) {
    const relPath = path.join(currentRel, entry.name);
    const srcPath = path.join(srcDir, entry.name);
    const destPath = path.join(destDir, entry.name);

    if (shouldExclude(relPath)) {
      continue;
    }

    if (entry.isDirectory()) {
      fs.mkdirSync(destPath, { recursive: true });
      copyRecursively(srcPath, destPath, relPath);
    } else if (entry.isFile()) {
      fs.copyFileSync(srcPath, destPath);
    }
  }
}

function ensureEmptyDir(dir) {
  if (fs.existsSync(dir)) {
    fs.rmSync(dir, { recursive: true, force: true });
  }
  fs.mkdirSync(dir, { recursive: true });
}

function createZipFromStaging() {
  if (IS_WIN) {
    const psScript = `Compress-Archive -Path '${STAGING_DIR}\\*' -DestinationPath '${OUTPUT_ZIP}' -Force`;
    execSync(`powershell -NoProfile -Command "${psScript}"`, { stdio: 'inherit' });
  } else {
    execSync(`cd "${STAGING_DIR}" && zip -r "${OUTPUT_ZIP}" .`, { stdio: 'ignore' });
  }
}

function verifyZip() {
  log('\n🔍 Memverifikasi isi file ZIP...');
  let files = [];
  if (IS_WIN) {
    const psList = `Add-Type -AssemblyName System.IO.Compression.FileSystem; [System.IO.Compression.ZipFile]::OpenRead('${OUTPUT_ZIP}').Entries.FullName`;
    const out = execSync(`powershell -NoProfile -Command "${psList}"`, { encoding: 'utf-8' });
    files = out.split(/\r?\n/).map((s) => s.trim()).filter(Boolean);
  } else {
    const out = execSync(`unzip -Z1 "${OUTPUT_ZIP}"`, { encoding: 'utf-8' });
    files = out.split('\n').map((s) => s.trim()).filter(Boolean);
  }

  let leaksFound = 0;
  for (const file of files) {
    if (shouldExclude(file)) {
      console.error(`  ❌ PERINGATAN: File terlarang terdeteksi: ${file}`);
      leaksFound++;
    }
  }

  if (leaksFound > 0) {
    fs.unlinkSync(OUTPUT_ZIP);
    throw new Error(`Verifikasi gagal! Ditemukan ${leaksFound} file yang tidak seharusnya masuk.`);
  }

  log(`  ✓ Semua file aman (${files.length} file). Tidak ada file sensitif / .env / .db / sesi yang bocor.`);
}

function pack() {
  log('==============================================');
  log('  Packaging Source Code untuk Member / LMS');
  log('==============================================\n');

  if (!fs.existsSync(DIST_DIR)) {
    fs.mkdirSync(DIST_DIR, { recursive: true });
  }
  if (fs.existsSync(OUTPUT_ZIP)) {
    fs.unlinkSync(OUTPUT_ZIP);
  }

  log('1. Menyiapkan staging directory bersih...');
  ensureEmptyDir(STAGING_DIR);

  log('2. Menyaring file source code (mengabaikan .env, .db, node_modules, cache, media, dll)...');
  copyRecursively(ROOT, STAGING_DIR);

  // Pastikan folder data/ ada untuk tempat database member nanti saat dijalankan
  const stagingDataDir = path.join(STAGING_DIR, 'data');
  if (!fs.existsSync(stagingDataDir)) {
    fs.mkdirSync(stagingDataDir, { recursive: true });
  }
  fs.writeFileSync(path.join(stagingDataDir, '.gitkeep'), '');

  log('3. Mengompresi menjadi file ZIP...');
  createZipFromStaging();

  log('4. Membersihkan staging directory...');
  fs.rmSync(STAGING_DIR, { recursive: true, force: true });

  // Verifikasi keamanan isi zip
  verifyZip();

  const stat = fs.statSync(OUTPUT_ZIP);
  const sizeMB = (stat.size / (1024 * 1024)).toFixed(2);
  log(`\n==============================================`);
  log(`  ✓ SUKSES: File Siap Dibagikan ke Member!`);
  log(`  Lokasi : ${OUTPUT_ZIP}`);
  log(`  Ukuran : ${sizeMB} MB (${stat.size} bytes)`);
  log(`==============================================\n`);
}

try {
  pack();
} catch (err) {
  console.error('\n✗ Gagal mem-package source code:', err.message);
  process.exit(1);
}
