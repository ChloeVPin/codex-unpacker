import './style.css';

import {
  ChooseLocalMSIX,
  DryRunLocal,
  GetStatus,
  ProbeLatest,
  PublishLatest,
  PublishLocal,
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
          <p class="eyebrow">Codex Unpacked</p>
          <h1>Release mirror</h1>
        </div>
        <span id="statePill" class="pill">Checking</span>
      </header>

      <div class="grid">
        <div class="metric">
          <span>Repo</span>
          <strong id="repoValue">-</strong>
        </div>
        <div class="metric">
          <span>State</span>
          <strong id="stateValue">-</strong>
        </div>
        <div class="metric">
          <span>Auth</span>
          <strong id="authValue">-</strong>
        </div>
      </div>

      <div class="section">
        <div>
          <h2>Latest upstream</h2>
          <p id="probeText" class="muted">Probe the current Windows package source.</p>
        </div>
        <div class="actions">
          <button id="probeBtn" class="button secondary">Probe</button>
          <button id="publishLatestBtn" class="button primary">Publish</button>
        </div>
      </div>

      <div class="section">
        <div>
          <h2>Local MSIX</h2>
          <p id="localText" class="muted">Choose a package for dry-run validation or manual publishing.</p>
        </div>
        <div class="actions">
          <button id="pickBtn" class="button secondary">Choose</button>
          <button id="dryRunBtn" class="button secondary" disabled>Dry run</button>
          <button id="publishLocalBtn" class="button primary" disabled>Publish</button>
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
  repoValue: document.querySelector('#repoValue'),
  stateValue: document.querySelector('#stateValue'),
  authValue: document.querySelector('#authValue'),
  probeText: document.querySelector('#probeText'),
  localText: document.querySelector('#localText'),
  resultBox: document.querySelector('#resultBox'),
  logBox: document.querySelector('#logBox'),
  probeBtn: document.querySelector('#probeBtn'),
  publishLatestBtn: document.querySelector('#publishLatestBtn'),
  pickBtn: document.querySelector('#pickBtn'),
  dryRunBtn: document.querySelector('#dryRunBtn'),
  publishLocalBtn: document.querySelector('#publishLocalBtn'),
  clearBtn: document.querySelector('#clearBtn'),
};

EventsOn('log', (message) => addLog(message));

refs.probeBtn.addEventListener('click', () => runTask('Probe latest', async () => {
  state.probe = await ProbeLatest();
  state.result = {
    title: state.probe.wouldUpdate ? 'Update available' : 'No update',
    detail: `${state.probe.packageVersion || '-'} via ${state.probe.sourceKind}`,
  };
}));

refs.publishLatestBtn.addEventListener('click', () => runTask('Publish latest', async () => {
  state.result = await PublishLatest(false);
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

refs.publishLocalBtn.addEventListener('click', () => runTask('Publish local package', async () => {
  state.result = await PublishLocal(state.local.path, false);
  await refreshStatus();
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
  refs.statePill.textContent = state.busy ? 'Working' : status?.ghAuthed ? 'Ready' : 'Needs auth';
  refs.statePill.className = `pill ${state.busy ? 'working' : status?.ghAuthed ? 'ready' : 'warn'}`;

  refs.repoValue.textContent = status?.repo || 'Not linked';
  refs.stateValue.textContent = status?.stateVersion ? `${status.stateVersion} (${status.stateHash.slice(0, 8)})` : 'Empty';
  refs.authValue.textContent = status?.ghAuthed ? 'GitHub CLI' : status?.ghAvailable ? 'Sign in' : 'Missing gh';

  if (state.probe) {
    const marker = state.probe.wouldUpdate ? 'New' : 'Current';
    refs.probeText.textContent = `${marker}: ${state.probe.packageVersion} from ${state.probe.sourceKind}`;
  }

  if (state.local?.path) {
    refs.localText.textContent = `${state.local.version} at ${state.local.fileName}`;
  }

  refs.probeBtn.disabled = state.busy;
  refs.publishLatestBtn.disabled = state.busy || !status?.ghAuthed;
  refs.pickBtn.disabled = state.busy;
  refs.dryRunBtn.disabled = state.busy || !state.local?.path;
  refs.publishLocalBtn.disabled = state.busy || !state.local?.path || !status?.ghAuthed;

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
