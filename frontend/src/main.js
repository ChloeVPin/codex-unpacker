import './style.css';

import {
  ChooseLocalMSIX,
  DryRunLocal,
  GetStatus,
  ProbeLatest,
  DownloadLatest,
} from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

const state = {
  busy: false,
  status: null,
  probe: null,
  local: null,
  result: null,
  logs: [],
};

const app = document.querySelector('#app');

app.innerHTML = `
  <main class="shell">
    <section class="panel">
      <header class="topbar">
        <div>
          <p class="eyebrow">Codex Unpacker</p>
          <h1>Download the latest Codex MSIX</h1>
          <p class="lede">Save the current package locally, or inspect a Codex MSIX already on disk.</p>
          <p class="version">Version 1.0.1</p>
        </div>
        <span id="statePill" class="pill">Checking</span>
      </header>

      <div class="grid">
        <div class="metric">
          <span>Saved</span>
          <strong id="savedValue">-</strong>
        </div>
        <div class="metric">
          <span>Latest</span>
          <strong id="latestValue">-</strong>
        </div>
        <div class="metric">
          <span>Folder</span>
          <strong id="folderValue">-</strong>
        </div>
      </div>

      <div class="section">
        <div>
          <h2>Latest package</h2>
          <p id="probeText" class="muted">Check the current Windows package source, then save it locally.</p>
        </div>
        <div class="actions">
          <button id="probeBtn" class="button secondary">Probe</button>
          <button id="downloadBtn" class="button primary">Download</button>
        </div>
      </div>

      <div class="section">
        <div>
          <h2>Inspect local MSIX</h2>
          <p id="localText" class="muted">Choose a package that is already on your machine and check it against the recorded state.</p>
        </div>
        <div class="actions">
          <button id="pickBtn" class="button secondary">Choose</button>
          <button id="dryRunBtn" class="button secondary" disabled>Dry run</button>
        </div>
      </div>

      <div id="resultBox" class="resultBox hidden"></div>

      <div class="logWrap">
        <div class="logHeader">
          <span>Activity</span>
          <button id="clearBtn" class="ghost">Clear</button>
        </div>
        <pre id="logBox"></pre>
      </div>
    </section>
  </main>
`;

const refs = {
  statePill: document.querySelector('#statePill'),
  savedValue: document.querySelector('#savedValue'),
  latestValue: document.querySelector('#latestValue'),
  folderValue: document.querySelector('#folderValue'),
  probeText: document.querySelector('#probeText'),
  localText: document.querySelector('#localText'),
  resultBox: document.querySelector('#resultBox'),
  logBox: document.querySelector('#logBox'),
  probeBtn: document.querySelector('#probeBtn'),
  downloadBtn: document.querySelector('#downloadBtn'),
  pickBtn: document.querySelector('#pickBtn'),
  dryRunBtn: document.querySelector('#dryRunBtn'),
  clearBtn: document.querySelector('#clearBtn'),
};

EventsOn('log', (message) => addLog(message));

refs.probeBtn.addEventListener('click', () => runTask('Probe latest', async () => {
  state.probe = await ProbeLatest();
  state.result = {
    title: state.probe.wouldUpdate ? 'Download available' : 'Already saved',
    detail: `${state.probe.packageVersion || '-'} via ${state.probe.sourceKind}`,
  };
}));

refs.downloadBtn.addEventListener('click', () => runTask('Download latest', async () => {
  const result = await DownloadLatest();
  state.result = {
    ...result,
    title: result.mode || 'Downloaded',
    detail: result.path ? `Saved to ${result.path}` : result.message,
  };
  await refreshStatus();
}));

refs.pickBtn.addEventListener('click', () => runTask('Choose local package', async () => {
  const selected = await ChooseLocalMSIX();
  if (selected && selected.path) {
    state.local = selected;
    state.result = {
      title: 'Local package loaded',
      detail: `${selected.version} (${selected.sha256.slice(0, 12)})`,
    };
  }
}));

refs.dryRunBtn.addEventListener('click', () => runTask('Dry run local package', async () => {
  state.result = await DryRunLocal(state.local.path);
}));

refs.clearBtn.addEventListener('click', () => {
  state.logs = [];
  render();
});

async function runTask(label, fn) {
  if (state.busy) return;
  state.busy = true;
  state.result = null;
  addLog(label);
  render();
  try {
    await fn();
  } catch (error) {
    state.result = {
      title: 'Failed',
      detail: error?.message || String(error),
      error: true,
    };
    addLog(error?.message || String(error));
  } finally {
    state.busy = false;
    render();
  }
}

async function refreshStatus() {
  state.status = await GetStatus();
}

function addLog(message) {
  const time = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  state.logs = [`${time}  ${message}`, ...state.logs].slice(0, 80);
  render();
}

function render() {
  const status = state.status;
  refs.statePill.textContent = state.busy ? 'Working' : status ? 'Ready' : 'Checking';
  refs.statePill.className = `pill ${state.busy ? 'working' : status ? 'ready' : 'warn'}`;

  refs.savedValue.textContent = status?.stateVersion ? `${status.stateVersion} (${status.stateHash.slice(0, 8)})` : 'None';
  refs.latestValue.textContent = state.probe?.packageVersion ? `${state.probe.packageVersion}${state.probe.wouldUpdate ? ' new' : ''}` : 'Not checked';
  refs.folderValue.textContent = status?.workingFolder || 'Unknown';

  if (state.probe) {
    const marker = state.probe.wouldUpdate ? 'New download' : 'Current package';
    refs.probeText.textContent = `${marker}: ${state.probe.packageVersion} from ${state.probe.sourceKind}`;
  }

  if (state.local?.path) {
    refs.localText.textContent = `${state.local.version} at ${state.local.fileName}`;
  }

  refs.probeBtn.disabled = state.busy;
  refs.downloadBtn.disabled = state.busy;
  refs.pickBtn.disabled = state.busy;
  refs.dryRunBtn.disabled = state.busy || !state.local?.path;

  if (state.result) {
    refs.resultBox.className = `resultBox ${state.result.error ? 'error' : ''}`;
    refs.resultBox.innerHTML = `
      <strong>${escapeHtml(state.result.title || state.result.mode || 'Done')}</strong>
      <span>${escapeHtml(state.result.detail || state.result.message || state.result.releaseUrl || '')}</span>
    `;
  } else {
    refs.resultBox.className = 'resultBox hidden';
    refs.resultBox.innerHTML = '';
  }

  refs.logBox.textContent = state.logs.join('\n');
}

function escapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');
}

refreshStatus()
  .then(() => addLog('Ready'))
  .catch((error) => addLog(error?.message || String(error)))
  .finally(render);
