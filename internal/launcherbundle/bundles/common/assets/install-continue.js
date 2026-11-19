// Installs only in the Linux sandbox/container's remote VS Code extension home.
const fs = require('fs');
const os = require('os');
const path = require('path');
const crypto = require('crypto');
const {spawnSync} = require('child_process');
const version = '2.0.0';
const target = `linux-${os.arch()}`;
const name = `continue.continue-${version}-${target}`;
const extensions = path.join(os.homedir(), '.vscode-server', 'extensions');
const destination = path.join(extensions, name);
const markerName = '.sandbox-continue-policy.json';
function valid(root) {
  try {
    const marker = JSON.parse(fs.readFileSync(path.join(root, markerName), 'utf8'));
    if (marker.revision !== 1 || marker.version !== version || marker.platform !== target) return false;
    const files = Object.keys(marker.files).sort();
    if (files.join(',') !== ['gui/assets/index.js','out/extension.js','package.json'].join(',')) return false;
    return files.every(file => crypto.createHash('sha256').update(fs.readFileSync(path.join(root, file))).digest('hex') === marker.files[file]);
  } catch (_) { return false; }
}
try {
  if (os.platform() !== 'linux' || !['linux-arm64','linux-x64'].includes(target)) throw Error('Continue defaults must be installed inside the Linux sandbox.');
  fs.mkdirSync(extensions, {recursive:true});
  const conflicts = fs.readdirSync(extensions).filter(entry => entry.startsWith('continue.continue-') && entry !== name);
  if (conflicts.length) throw Error('A different Continue version is already installed. Remove it or use a new sandbox to use the pinned sandbox distribution.');
  if (process.argv[2] === 'check') process.exit(valid(destination) ? 0 : 1);
  if (process.argv[2] !== 'install') throw Error('Expected check or install.');
  const staging = fs.mkdtempSync(path.join(extensions, '.sandbox-continue-'));
  try {
    const unpack = spawnSync('tar', ['-xf','-','-C',staging], {stdio:'inherit'});
    if (unpack.error || unpack.status !== 0) throw Error('Could not receive Continue package.');
    const prepared = path.join(staging, name);
    if (!valid(prepared)) throw Error('Continue package does not contain verified automatic defaults.');
    const previous = path.join(staging, 'previous');
    if (fs.existsSync(destination)) fs.renameSync(destination, previous);
    try { fs.renameSync(prepared, destination); }
    catch (error) { if (fs.existsSync(previous)) fs.renameSync(previous, destination); throw error; }
    console.log(`Continue ${version}: enabled tools default to Automatic, including terminal commands.`);
  } finally { fs.rmSync(staging, {recursive:true, force:true}); }
} catch (error) { console.error(error.message); process.exit(1); }
