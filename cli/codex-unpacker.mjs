#!/usr/bin/env node
import { createHash } from 'node:crypto';
import { createReadStream, promises as fs } from 'node:fs';
import { basename, dirname, extname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { tmpdir } from 'node:os';
import { spawnSync } from 'node:child_process';
import { inflateRawSync } from 'node:zlib';

const updateManifestUrl = 'https://persistent.oaistatic.com/codex-app-prod/windows-store-update.json';
const mirrorLatestApi = 'https://api.github.com/repos/Wangnov/codex-app-mirror/releases/latest';
const packageIdentity = 'OpenAI.Codex';
const familySuffix = '2p2nqsd0c76g0';
const architecture = 'x64';
const statePath = 'data/latest.json';
const releaseTagPrefix = 'codex-msix';

const commands = new Set(['status', 'probe', 'download', 'local', 'help']);

main().catch((error) => {
  console.error(`\nerror: ${error.message || error}`);
  process.exit(1);
});

async function main() {
  const args = process.argv.slice(2);
  const command = commands.has(args[0]) ? args.shift() : 'help';
  const opts = parseFlags(args);

  if (command === 'help') {
    printHelp();
    return;
  }

  if (command === 'status') {
    console.log(JSON.stringify(await getStatus(), null, 2));
    return;
  }

  if (command === 'probe') {
    const probe = await probeLatest();
    printProbe(probe);
    if (opts.json) console.log(JSON.stringify(probe, null, 2));
    return;
  }

  if (command === 'download') {
    const result = await downloadLatest(opts.output);
    console.log(JSON.stringify(result, null, 2));
    return;
  }

  if (command === 'local') {
    const msixPath = opts._[0];
    if (!msixPath) throw new Error('local requires a path to a .msix file');
    const result = await dryRunLocal(msixPath);
    console.log(JSON.stringify(result, null, 2));
  }
}

function parseFlags(args) {
  const opts = { _: [] };
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === '--output') {
      opts.output = args[i + 1];
      i += 1;
    } else if (arg === '--force') opts.force = true;
    else if (arg === '--json') opts.json = true;
    else if (arg === '--dry-run') opts.dryRun = true;
    else opts._.push(arg);
  }
  return opts;
}

function printHelp() {
  console.log(`Codex Unpacker CLI

Usage:
  npx github:ChloeVPin/codex-unpacker status
  npx github:ChloeVPin/codex-unpacker probe
  npx github:ChloeVPin/codex-unpacker download [--output <path>]
  npx github:ChloeVPin/codex-unpacker local <path.msix> [--dry-run]
 
The download command saves the latest Codex MSIX to a local path and does not publish anything.
`);
}

async function getStatus() {
  const state = await readState().catch(() => null);
  return {
    stateVersion: state?.package?.version || '',
    stateHash: state?.package?.sha256 || '',
    workingFolder: process.cwd(),
  };
}

async function probeLatest() {
  const source = await resolveLatest();
  const state = await readState().catch(() => null);
  let wouldUpdate = state?.package?.version !== source.packageVersion;
  if (source.expectedSha256) {
    wouldUpdate = wouldUpdate || !sameHash(state?.package?.sha256, source.expectedSha256);
  }
  return {
    ...source,
    wouldUpdate,
    currentStateVersion: state?.package?.version || '',
    currentStateSha256: state?.package?.sha256 || '',
  };
}

async function resolveLatest() {
  const manifest = await getJson(updateManifestUrl);
  if (manifest.packageIdentity && manifest.packageIdentity !== packageIdentity) {
    throw new Error(`unexpected package identity ${manifest.packageIdentity}`);
  }

  const release = await getJson(mirrorLatestApi);
  const manifestAsset = findAsset(release.assets || [], 'release-manifest.json');
  const checksumAsset = findAsset(release.assets || [], 'SHA256SUMS-windows.txt');
  const msixAsset = findMsixAsset(release.assets || []);
  if (!manifestAsset || !msixAsset) {
    throw new Error('mirror release is missing release-manifest.json or the MSIX asset');
  }

  const mirrorManifest = await getJson(manifestAsset.browser_download_url);
  const checksumText = checksumAsset ? await getText(checksumAsset.browser_download_url).catch(() => '') : '';
  const expectedSha256 = checksumText ? parseChecksum(checksumText, msixAsset.name) : '';
  const windows = mirrorManifest.sources?.windows || {};
  const version = windows.version || versionFromMoniker(windows.packageMoniker || msixAsset.name);

  return {
    sourceKind: 'MirrorRelease',
    updateManifestVersion: windows.updateManifest?.buildVersion || manifest.buildVersion || '',
    packageVersion: version,
    packageMoniker: stripExtension(msixAsset.name),
    downloadUrl: msixAsset.browser_download_url,
    expectedSha256,
    mirrorReleaseTag: release.tag_name || '',
    mirrorReleaseUrl: release.html_url || '',
    directStoreStatus: 'Official manifest is checked; package bytes are resolved from the validated mirror release source.',
  };
}

async function publishLatest(force) {
  const source = await resolveLatest();
  const state = await readState().catch(() => null);
  if (!force && state?.package?.version === source.packageVersion && sameHash(state?.package?.sha256, source.expectedSha256)) {
    return { mode: 'No update', version: source.packageVersion, sha256: source.expectedSha256, message: 'Latest package already matches data/latest.json.' };
  }

  const temp = await fs.mkdtemp(join(tmpdir(), 'codex-unpacker-'));
  try {
    const target = join(temp, `${source.packageMoniker}.Msix`);
    console.log(`downloading ${source.packageMoniker}`);
    await downloadFile(source.downloadUrl, target);
    const info = await inspectMsix(target, source.packageVersion);
    if (source.expectedSha256 && !sameHash(info.sha256, source.expectedSha256)) {
      throw new Error(`downloaded package hash mismatch: expected ${source.expectedSha256}, got ${info.sha256}`);
    }
    return await publishPackage(info, source, force);
  } finally {
    await fs.rm(temp, { recursive: true, force: true });
  }
}

async function downloadLatest(outputPath) {
  const source = await resolveLatest();
  const defaultName = `${source.packageMoniker}.Msix`;
  let target = outputPath ? resolve(outputPath) : join(process.cwd(), defaultName);
  try {
    const stat = await fs.stat(target);
    if (stat.isDirectory()) {
      target = join(target, defaultName);
    }
  } catch {}

  await fs.mkdir(dirname(target), { recursive: true });
  console.log(`downloading ${source.packageMoniker}`);
  await downloadFile(source.downloadUrl, target);
  const info = await inspectMsix(target, source.packageVersion);
  if (source.expectedSha256 && !sameHash(info.sha256, source.expectedSha256)) {
    throw new Error(`downloaded package hash mismatch: expected ${source.expectedSha256}, got ${info.sha256}`);
  }
  await writeJson(statePath, {
    schemaVersion: 1,
    updatedAt: new Date().toISOString(),
    package: {
      name: info.name,
      version: info.version,
      packageMoniker: source.packageMoniker,
      packageFamilyName: info.packageFamilyName,
      publisher: info.publisher,
      sha256: info.sha256,
      size: info.size,
      fileName: info.fileName,
      sourceKind: source.sourceKind,
    },
    source,
  });
  return {
    mode: 'Downloaded',
    version: info.version,
    sha256: info.sha256,
    path: target,
    message: 'Saved to selected location.',
  };
}

async function dryRunLocal(msixPath) {
  const info = await inspectMsix(msixPath, '');
  const state = await readState().catch(() => null);
  const same = state?.package?.version === info.version && sameHash(state?.package?.sha256, info.sha256);
  return {
    mode: same ? 'No update' : 'Would download',
    version: info.version,
    sha256: info.sha256,
    message: same ? 'Package matches data/latest.json.' : 'Package is different from data/latest.json.',
  };
}

async function publishLocal(msixPath, force) {
  const info = await inspectMsix(msixPath, '');
  const state = await readState().catch(() => null);
  if (!force && state?.package?.version === info.version && sameHash(state?.package?.sha256, info.sha256)) {
    return { mode: 'No update', version: info.version, sha256: info.sha256, message: 'Local package already matches data/latest.json.' };
  }
  return publishPackage(info, {
    sourceKind: 'LocalMsix',
    updateManifestVersion: info.version,
    packageVersion: info.version,
    packageMoniker: info.packageMoniker,
    downloadUrl: `local:${info.fileName}`,
  }, force);
}

async function publishPackage(info, source, force) {
  if (!commandExists('gh')) throw new Error('GitHub CLI is not installed or not in PATH');
  if (!ghAuthed()) throw new Error('GitHub CLI is not authenticated; run gh auth login first');

  const repo = repoInfo().name;
  if (!repo) throw new Error('could not determine GitHub repository');
  const commit = run('git', ['rev-parse', 'HEAD']).stdout.trim();
  const temp = await fs.mkdtemp(join(tmpdir(), 'codex-release-'));
  try {
    const tag = releaseTag(info.version, info.sha256, force);
    const packageAsset = join(temp, info.fileName);
    const shaPath = join(temp, 'SHA256SUMS.txt');
    const manifestPath = join(temp, 'release.json');
    const notesPath = join(temp, 'release-notes.md');

    await fs.copyFile(info.path, packageAsset);
    await fs.writeFile(shaPath, `${info.sha256}  ${info.fileName}\r\n`);
    await writeJson(manifestPath, releaseManifest(info, source, tag, '', ''));
    await fs.writeFile(notesPath, releaseNotes(info, source));

    console.log(`creating GitHub release ${tag}`);
    run('gh', ['release', 'create', tag, '--repo', repo, '--target', commit, '--title', `Codex MSIX ${info.version}`, '--notes-file', notesPath, packageAsset, shaPath, manifestPath]);
    const view = JSON.parse(run('gh', ['release', 'view', tag, '--repo', repo, '--json', 'id,url']).stdout);

    const state = {
      schemaVersion: 1,
      updatedAt: new Date().toISOString(),
      package: {
        name: info.name,
        version: info.version,
        packageMoniker: info.packageMoniker,
        packageFamilyName: info.packageFamilyName,
        publisher: info.publisher,
        sha256: info.sha256,
        size: info.size,
        fileName: info.fileName,
        sourceKind: source.sourceKind,
      },
      source,
      release: {
        tag,
        id: String(view.id || ''),
        url: view.url || '',
      },
    };
    await writeJson(statePath, state);
    run('git', ['add', statePath], { soft: true });
    run('git', ['commit', '-m', 'Update mirrored Codex package state'], { soft: true });
    run('git', ['push'], { soft: true });

    return { mode: 'Published', version: info.version, sha256: info.sha256, releaseTag: tag, releaseUrl: view.url || '', message: 'Release created and local state updated.' };
  } finally {
    await fs.rm(temp, { recursive: true, force: true });
  }
}

async function inspectMsix(inputPath, expectedVersion) {
  const abs = resolve(inputPath);
  const zip = await readZipIndex(abs);
  const names = new Set(zip.entries.map((entry) => entry.name.replaceAll('\\', '/')));
  for (const name of ['AppxManifest.xml', 'AppxBlockMap.xml', 'AppxSignature.p7x', 'AppxMetadata/CodeIntegrity.cat']) {
    if (!names.has(name)) throw new Error(`MSIX is missing ${name}`);
  }
  const manifestBytes = await readZipFile(abs, zip, 'AppxManifest.xml');
  const identity = parseIdentity(manifestBytes.toString('utf8'));
  if (identity.Name !== packageIdentity) throw new Error(`unexpected package identity ${identity.Name}`);
  if (identity.ProcessorArchitecture && identity.ProcessorArchitecture.toLowerCase() !== architecture) {
    throw new Error(`unexpected architecture ${identity.ProcessorArchitecture}`);
  }
  if (expectedVersion && identity.Version !== expectedVersion) {
    throw new Error(`expected version ${expectedVersion}, got ${identity.Version}`);
  }
  const stat = await fs.stat(abs);
  const sha256 = await hashFile(abs);
  const fileName = basename(abs);
  const packageMoniker = stripExtension(fileName);
  const fileVersion = versionFromMoniker(packageMoniker);
  if (fileVersion && fileVersion !== identity.Version) {
    throw new Error(`filename version ${fileVersion} does not match manifest version ${identity.Version}`);
  }
  return {
    name: identity.Name,
    version: identity.Version,
    packageMoniker,
    packageFamilyName: `${identity.Name}_${familySuffix}`,
    publisher: identity.Publisher,
    architecture: identity.ProcessorArchitecture,
    sha256,
    size: stat.size,
    fileName,
    path: abs,
  };
}

async function readZipIndex(zipPath) {
  const data = await fs.readFile(zipPath);
  const eocdOffset = findEocd(data);
  if (eocdOffset < 0) throw new Error('invalid zip: end of central directory not found');
  let entryCount = data.readUInt16LE(eocdOffset + 10);
  let centralOffset = data.readUInt32LE(eocdOffset + 16);
  if (entryCount === 0xffff || centralOffset === 0xffffffff) {
    const zip64 = readZip64DirectoryInfo(data, eocdOffset);
    entryCount = zip64.entryCount;
    centralOffset = zip64.centralOffset;
  }
  const entries = [];
  let offset = centralOffset;
  for (let i = 0; i < entryCount; i += 1) {
    if (data.readUInt32LE(offset) !== 0x02014b50) throw new Error('invalid zip: bad central directory header');
    const method = data.readUInt16LE(offset + 10);
    let compressedSize = data.readUInt32LE(offset + 20);
    let uncompressedSize = data.readUInt32LE(offset + 24);
    const nameLength = data.readUInt16LE(offset + 28);
    const extraLength = data.readUInt16LE(offset + 30);
    const commentLength = data.readUInt16LE(offset + 32);
    let localHeaderOffset = data.readUInt32LE(offset + 42);
    const name = data.subarray(offset + 46, offset + 46 + nameLength).toString('utf8');
    const extra = data.subarray(offset + 46 + nameLength, offset + 46 + nameLength + extraLength);
    if (compressedSize === 0xffffffff || uncompressedSize === 0xffffffff || localHeaderOffset === 0xffffffff) {
      const zip64 = parseZip64Extra(extra, { compressedSize, uncompressedSize, localHeaderOffset });
      compressedSize = zip64.compressedSize;
      uncompressedSize = zip64.uncompressedSize;
      localHeaderOffset = zip64.localHeaderOffset;
    }
    entries.push({ name, method, compressedSize, uncompressedSize, localHeaderOffset });
    offset += 46 + nameLength + extraLength + commentLength;
  }
  return { entries };
}

function readZip64DirectoryInfo(data, eocdOffset) {
  const locatorOffset = eocdOffset - 20;
  if (locatorOffset < 0 || data.readUInt32LE(locatorOffset) !== 0x07064b50) {
    throw new Error('zip64 end of central directory locator not found');
  }
  const zip64EocdOffset = safeUInt64(data.readBigUInt64LE(locatorOffset + 8));
  if (data.readUInt32LE(zip64EocdOffset) !== 0x06064b50) {
    throw new Error('zip64 end of central directory record not found');
  }
  return {
    entryCount: safeUInt64(data.readBigUInt64LE(zip64EocdOffset + 32)),
    centralOffset: safeUInt64(data.readBigUInt64LE(zip64EocdOffset + 48)),
  };
}

function parseZip64Extra(extra, values) {
  let offset = 0;
  while (offset + 4 <= extra.length) {
    const headerId = extra.readUInt16LE(offset);
    const dataSize = extra.readUInt16LE(offset + 2);
    offset += 4;
    if (headerId === 0x0001) {
      let cursor = offset;
      const nextUInt64 = () => {
        if (cursor + 8 > offset + dataSize) throw new Error('invalid zip64 extra field');
        const value = safeUInt64(extra.readBigUInt64LE(cursor));
        cursor += 8;
        return value;
      };
      if (values.uncompressedSize === 0xffffffff) values.uncompressedSize = nextUInt64();
      if (values.compressedSize === 0xffffffff) values.compressedSize = nextUInt64();
      if (values.localHeaderOffset === 0xffffffff) values.localHeaderOffset = nextUInt64();
      return values;
    }
    offset += dataSize;
  }
  throw new Error('zip64 extra field required but not found');
}

function safeUInt64(value) {
  const asNumber = Number(value);
  if (!Number.isSafeInteger(asNumber)) throw new Error('zip64 value is too large for this runtime');
  return asNumber;
}

async function readZipFile(zipPath, zip, wantedName) {
  const data = await fs.readFile(zipPath);
  const entry = zip.entries.find((item) => item.name.replaceAll('\\', '/') === wantedName);
  if (!entry) throw new Error(`zip entry not found: ${wantedName}`);
  const offset = entry.localHeaderOffset;
  if (data.readUInt32LE(offset) !== 0x04034b50) throw new Error('invalid zip: bad local header');
  const nameLength = data.readUInt16LE(offset + 26);
  const extraLength = data.readUInt16LE(offset + 28);
  const payloadOffset = offset + 30 + nameLength + extraLength;
  const compressed = data.subarray(payloadOffset, payloadOffset + entry.compressedSize);
  if (entry.method === 0) return compressed;
  if (entry.method === 8) return inflateRawSync(compressed);
  throw new Error(`unsupported zip compression method ${entry.method}`);
}

function findEocd(data) {
  const min = Math.max(0, data.length - 0xffff - 22);
  for (let i = data.length - 22; i >= min; i -= 1) {
    if (data.readUInt32LE(i) === 0x06054b50) return i;
  }
  return -1;
}

function parseIdentity(xml) {
  const match = xml.match(/<Identity\b([^>]*)>/i);
  if (!match) throw new Error('AppxManifest.xml does not contain an Identity element');
  const attrs = {};
  for (const attr of match[1].matchAll(/([A-Za-z_:][\w:.-]*)\s*=\s*"([^"]*)"/g)) {
    attrs[attr[1]] = attr[2];
  }
  for (const required of ['Name', 'Version', 'Publisher']) {
    if (!attrs[required]) throw new Error(`Identity is missing ${required}`);
  }
  return attrs;
}

async function getJson(url) {
  const response = await fetch(url, { headers: { 'User-Agent': 'codex-unpacker/1.0', Accept: 'application/vnd.github+json, application/json, */*' } });
  if (!response.ok) throw new Error(`HTTP ${response.status} from ${url}: ${await response.text()}`);
  return response.json();
}

async function getText(url) {
  const response = await fetch(url, { headers: { 'User-Agent': 'codex-unpacker/1.0' } });
  if (!response.ok) throw new Error(`HTTP ${response.status} from ${url}: ${await response.text()}`);
  return response.text();
}

async function downloadFile(url, target) {
  const response = await fetch(url, { headers: { 'User-Agent': 'codex-unpacker/1.0' } });
  if (!response.ok) throw new Error(`HTTP ${response.status} from ${url}: ${await response.text()}`);
  await fs.mkdir(dirname(target), { recursive: true });
  const file = await fs.open(target, 'w');
  try {
    for await (const chunk of response.body) {
      await file.write(chunk);
    }
  } finally {
    await file.close();
  }
}

async function hashFile(path) {
  const hash = createHash('sha256');
  await new Promise((resolvePromise, reject) => {
    createReadStream(path)
      .on('data', (chunk) => hash.update(chunk))
      .on('error', reject)
      .on('end', resolvePromise);
  });
  return hash.digest('hex');
}

async function readState() {
  return JSON.parse(await fs.readFile(statePath, 'utf8'));
}

async function writeJson(path, value) {
  await fs.mkdir(dirname(path), { recursive: true });
  await fs.writeFile(path, `${JSON.stringify(value, null, 2)}\n`);
}

function releaseManifest(info, source, tag, id, url) {
  return {
    schemaVersion: 1,
    generatedAt: new Date().toISOString(),
    release: { tag, id, url },
    source,
    package: {
      name: info.name,
      version: info.version,
      packageMoniker: info.packageMoniker,
      packageFamilyName: info.packageFamilyName,
      publisher: info.publisher,
      architecture: info.architecture,
      sha256: info.sha256,
      size: info.size,
      fileName: info.fileName,
    },
  };
}

function releaseNotes(info, source) {
  return `Codex MSIX mirror update

Version: ${info.version}
Package: ${info.packageMoniker}
SHA256: ${info.sha256}
Source: ${source.downloadUrl}
Resolved via: ${source.sourceKind}
Advertised build: ${source.updateManifestVersion || ''}
Timestamp: ${new Date().toISOString()}
`;
}

function printProbe(probe) {
  console.log(`${probe.wouldUpdate ? 'update available' : 'no update'}: ${probe.packageVersion || '-'}`);
  console.log(`source: ${probe.sourceKind}`);
  console.log(`release: ${probe.mirrorReleaseTag || '-'}`);
  if (probe.expectedSha256) console.log(`sha256: ${probe.expectedSha256}`);
}

function findAsset(assets, name) {
  return assets.find((asset) => asset.name === name);
}

function findMsixAsset(assets) {
  return assets
    .filter((asset) => /^OpenAI\.Codex_.+_x64__2p2nqsd0c76g0\.Msix$/i.test(asset.name))
    .sort((a, b) => compareVersions(versionFromMoniker(stripExtension(b.name)), versionFromMoniker(stripExtension(a.name))))[0];
}

function parseChecksum(body, fileName) {
  for (const line of body.split(/\r?\n/)) {
    const fields = line.trim().split(/\s+/);
    if (fields.length >= 2 && fields[1].toLowerCase() === fileName.toLowerCase() && /^[a-f0-9]{64}$/i.test(fields[0])) {
      return fields[0].toLowerCase();
    }
  }
  return '';
}

function versionFromMoniker(moniker) {
  return moniker.split('_')[1] || '';
}

function compareVersions(a, b) {
  const left = parseVersion(a);
  const right = parseVersion(b);
  for (let i = 0; i < 4; i += 1) {
    if (left[i] !== right[i]) return left[i] - right[i];
  }
  return 0;
}

function parseVersion(version) {
  return String(version).split('.').slice(0, 4).map((part) => Number.parseInt(part, 10) || 0).concat([0, 0, 0, 0]).slice(0, 4);
}

function stripExtension(name) {
  return name.slice(0, name.length - extname(name).length);
}

function sameHash(a, b) {
  return Boolean(a && b && String(a).toLowerCase() === String(b).toLowerCase());
}

function releaseTag(version, hash, force) {
  let tag = `${releaseTagPrefix}-${version}-${hash.toLowerCase().slice(0, 12)}`;
  if (force) tag += `-force-${new Date().toISOString().replace(/\D/g, '').slice(0, 14)}`;
  return tag;
}

function commandExists(name) {
  return spawnSync(process.platform === 'win32' ? 'where.exe' : 'command', process.platform === 'win32' ? [name] : ['-v', name], { stdio: 'ignore', shell: process.platform !== 'win32' }).status === 0;
}

function ghAuthed() {
  return commandExists('gh') && spawnSync('gh', ['auth', 'status'], { stdio: 'ignore' }).status === 0;
}

function repoInfo() {
  if (commandExists('gh')) {
    const out = run('gh', ['repo', 'view', '--json', 'nameWithOwner,isPrivate'], { soft: true });
    if (out.status === 0) {
      try {
        const parsed = JSON.parse(out.stdout);
        if (parsed.nameWithOwner) return { name: parsed.nameWithOwner, isPrivate: Boolean(parsed.isPrivate) };
      } catch {}
    }
  }
  const remote = run('git', ['remote', 'get-url', 'origin'], { soft: true });
  const match = remote.stdout.match(/github\.com[:/](.+?)(?:\.git)?\s*$/);
  return { name: match?.[1]?.replace(/\.git$/, '') || '', isPrivate: false };
}

function run(name, args, options = {}) {
  const result = spawnSync(name, args, { encoding: 'utf8' });
  if (!options.soft && result.status !== 0) {
    throw new Error(`${name} ${args.join(' ')} failed\n${result.stdout || ''}${result.stderr || ''}`);
  }
  return { status: result.status ?? 1, stdout: result.stdout || '', stderr: result.stderr || '' };
}
