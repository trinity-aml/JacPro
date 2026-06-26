package proxy

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(settingsHTML))
}

func (s *Server) handleSettingsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.Get())
	case http.MethodPost, http.MethodPut:
		defer r.Body.Close()
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		var patch SettingsPatch
		if err := decoder.Decode(&patch); err != nil {
			badRequest(w, err)
			return
		}
		settings, err := s.store.Update(patch)
		if err != nil {
			badRequest(w, err)
			return
		}
		if err := s.logger.Reconfigure(settings); err != nil {
			internalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleBackendStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	settings := s.store.Get()
	start := time.Now()
	data, ok := s.forwardBackendJSON(r.Context(), "/version", 5*time.Second)
	status := map[string]any{
		"ok":       ok,
		"base_url": settings.BaseURL,
		"latency":  time.Since(start).String(),
	}
	if ok {
		status["version"] = data
	} else {
		status["error"] = errors.New("backend did not return JSON from /version").Error()
	}
	writeJSON(w, http.StatusOK, status)
}

const settingsHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>JacPro Settings</title>
  <style>
    :root {
      color-scheme: light dark;
      --bg: #f6f7f9;
      --panel: #ffffff;
      --text: #1b1d22;
      --muted: #667085;
      --line: #d9dee7;
      --accent: #1769aa;
      --accent-strong: #0f578f;
      --ok: #18794e;
      --bad: #b42318;
      --warn: #a15c07;
      --shadow: 0 10px 24px rgba(15, 23, 42, 0.08);
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #111418;
        --panel: #191d23;
        --text: #f1f5f9;
        --muted: #a6b0c0;
        --line: #303741;
        --accent: #58a6d6;
        --accent-strong: #84c6ee;
        --ok: #4ab480;
        --bad: #ff8a80;
        --warn: #f3b45d;
        --shadow: none;
      }
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--text);
      font: 15px/1.45 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    header {
      border-bottom: 1px solid var(--line);
      background: var(--panel);
    }
    .topbar, main {
      width: min(1080px, calc(100% - 32px));
      margin: 0 auto;
    }
    .topbar {
      min-height: 64px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 12px;
      min-width: 0;
    }
    .mark {
      width: 34px;
      height: 34px;
      border-radius: 8px;
      background: linear-gradient(135deg, #1769aa, #21a179);
      box-shadow: inset 0 0 0 1px rgba(255,255,255,.28);
      flex: 0 0 auto;
    }
    h1 {
      margin: 0;
      font-size: 20px;
      line-height: 1.2;
      letter-spacing: 0;
    }
    .sub {
      color: var(--muted);
      font-size: 13px;
      white-space: nowrap;
    }
    main {
      padding: 28px 0 42px;
      display: grid;
      grid-template-columns: minmax(0, 1fr) 300px;
      gap: 20px;
      align-items: start;
    }
    form, aside {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: var(--shadow);
    }
    form {
      padding: 22px;
    }
    aside {
      padding: 18px;
    }
    .section {
      padding: 0 0 22px;
      margin: 0 0 22px;
      border-bottom: 1px solid var(--line);
    }
    .section:last-child {
      border-bottom: 0;
      margin-bottom: 0;
      padding-bottom: 0;
    }
    h2 {
      font-size: 14px;
      text-transform: uppercase;
      color: var(--muted);
      margin: 0 0 14px;
      letter-spacing: .06em;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 16px;
    }
    label {
      display: grid;
      gap: 7px;
      font-weight: 650;
      min-width: 0;
    }
    .hint {
      color: var(--muted);
      font-weight: 500;
      font-size: 12px;
    }
    input, select {
      width: 100%;
      min-height: 40px;
      border: 1px solid var(--line);
      border-radius: 6px;
      padding: 8px 10px;
      color: var(--text);
      background: transparent;
      font: inherit;
    }
    input:focus, select:focus {
      outline: 2px solid color-mix(in srgb, var(--accent) 40%, transparent);
      border-color: var(--accent);
    }
    .toggles {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 12px;
    }
    .toggle {
      min-height: 48px;
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 10px 12px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 14px;
      font-weight: 650;
    }
    .toggle input {
      width: 18px;
      height: 18px;
      min-height: 18px;
      accent-color: var(--accent);
      flex: 0 0 auto;
    }
    .actions {
      display: flex;
      justify-content: flex-end;
      gap: 10px;
      padding-top: 20px;
      border-top: 1px solid var(--line);
    }
    button {
      border: 1px solid var(--line);
      background: transparent;
      color: var(--text);
      min-height: 40px;
      padding: 8px 14px;
      border-radius: 6px;
      font: inherit;
      font-weight: 700;
      cursor: pointer;
    }
    button.primary {
      color: white;
      border-color: var(--accent);
      background: var(--accent);
    }
    button.primary:hover { background: var(--accent-strong); }
    button:disabled {
      cursor: wait;
      opacity: .7;
    }
    .status {
      margin-top: 14px;
      min-height: 22px;
      font-weight: 650;
    }
    .ok { color: var(--ok); }
    .bad { color: var(--bad); }
    .warn { color: var(--warn); }
    .metric {
      display: grid;
      gap: 5px;
      padding: 12px 0;
      border-bottom: 1px solid var(--line);
    }
    .metric:last-child { border-bottom: 0; }
    .metric span:first-child {
      color: var(--muted);
      font-size: 12px;
      text-transform: uppercase;
      letter-spacing: .06em;
    }
    .metric span:last-child {
      overflow-wrap: anywhere;
      font-weight: 700;
    }
    .side-actions {
      display: grid;
      gap: 10px;
      margin-top: 14px;
    }
    .full { grid-column: 1 / -1; }
    @media (max-width: 860px) {
      main { grid-template-columns: 1fr; }
      aside { order: -1; }
    }
    @media (max-width: 640px) {
      .topbar {
        align-items: flex-start;
        flex-direction: column;
        padding: 14px 0;
      }
      .sub { white-space: normal; }
      .grid, .toggles { grid-template-columns: 1fr; }
      form { padding: 16px; }
      .actions { justify-content: stretch; }
      .actions button { flex: 1 1 auto; }
    }
  </style>
</head>
<body>
  <header>
    <div class="topbar">
      <div class="brand">
        <div class="mark" aria-hidden="true"></div>
        <h1>JacPro Settings</h1>
      </div>
      <div class="sub">Torznab and Jackett proxy for JacRed</div>
    </div>
  </header>

  <main>
    <form id="settings-form" autocomplete="off">
      <section class="section">
        <h2>Backend</h2>
        <div class="grid">
          <label class="full">
            JacRed base URL
            <input name="base_url" type="url" required placeholder="http://127.0.0.1:9117">
          </label>
          <label class="full">
            API key
            <input name="apikey" type="password" autocomplete="new-password">
          </label>
          <label>
            Timeout
            <input name="request_timeout" type="number" min="1" max="300" step="1" required>
          </label>
          <label>
            Log level
            <select name="log_level">
              <option>DEBUG</option>
              <option>INFO</option>
              <option>WARNING</option>
              <option>ERROR</option>
              <option>CRITICAL</option>
            </select>
          </label>
        </div>
      </section>

      <section class="section">
        <h2>Search</h2>
        <div class="toggles">
          <label class="toggle">Merge v1 results <input name="merge_v1" type="checkbox"></label>
          <label class="toggle">Enrich Torznab titles <input name="enrich_titles" type="checkbox"></label>
          <label class="toggle">Strip trailing year <input name="strip_trailing_year" type="checkbox"></label>
          <label class="toggle">Skip category trim <input name="skip_cat_filter" type="checkbox"></label>
        </div>
      </section>

      <section class="section">
        <h2>Server</h2>
        <div class="grid">
          <label>
            Host
            <input name="host" required>
            <span class="hint">Applies after restart</span>
          </label>
          <label>
            Port
            <input name="port" type="number" min="1" max="65535" step="1" required>
            <span class="hint">Applies after restart</span>
          </label>
          <label class="full">
            Log file
            <input name="log_file">
          </label>
        </div>
      </section>

      <div class="actions">
        <button type="button" id="reload">Reload</button>
        <button class="primary" type="submit">Save</button>
      </div>
      <div id="status" class="status" role="status"></div>
    </form>

    <aside>
      <h2>Status</h2>
      <div class="metric"><span>Version</span><span id="version">-</span></div>
      <div class="metric"><span>Backend</span><span id="backend">-</span></div>
      <div class="metric"><span>Latency</span><span id="latency">-</span></div>
      <div class="metric"><span>Base URL</span><span id="baseurl">-</span></div>
      <div class="side-actions">
        <button type="button" id="check-backend">Check Backend</button>
        <button type="button" onclick="location.href='/api?t=caps'">Torznab Caps</button>
      </div>
    </aside>
  </main>

  <script>
    const form = document.querySelector('#settings-form');
    const statusEl = document.querySelector('#status');
    const versionEl = document.querySelector('#version');
    const backendEl = document.querySelector('#backend');
    const latencyEl = document.querySelector('#latency');
    const baseurlEl = document.querySelector('#baseurl');
    const fields = [
      'base_url', 'apikey', 'request_timeout', 'log_level', 'merge_v1',
      'enrich_titles', 'strip_trailing_year', 'skip_cat_filter', 'host',
      'port', 'log_file'
    ];

    function setStatus(text, kind) {
      statusEl.textContent = text || '';
      statusEl.className = 'status ' + (kind || '');
    }

    function readForm() {
      const data = {};
      for (const name of fields) {
        const input = form.elements[name];
        if (!input) continue;
        if (input.type === 'checkbox') data[name] = input.checked;
        else if (input.type === 'number') data[name] = Number(input.value);
        else data[name] = input.value;
      }
      return data;
    }

    function writeForm(data) {
      for (const name of fields) {
        const input = form.elements[name];
        if (!input) continue;
        if (input.type === 'checkbox') input.checked = Boolean(data[name]);
        else input.value = data[name] ?? '';
      }
      versionEl.textContent = data.version || '-';
      baseurlEl.textContent = data.base_url || '-';
    }

    async function loadSettings() {
      setStatus('Loading...', 'warn');
      const res = await fetch('/api/settings');
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      writeForm(data);
      setStatus('', '');
      return data;
    }

    async function saveSettings(event) {
      event.preventDefault();
      setStatus('Saving...', 'warn');
      form.querySelector('button.primary').disabled = true;
      try {
        const res = await fetch('/api/settings', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(readForm())
        });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'Save failed');
        writeForm(data);
        setStatus('Saved', 'ok');
        await checkBackend();
      } catch (err) {
        setStatus(err.message || String(err), 'bad');
      } finally {
        form.querySelector('button.primary').disabled = false;
      }
    }

    async function checkBackend() {
      backendEl.textContent = 'Checking...';
      latencyEl.textContent = '-';
      const res = await fetch('/api/backend/status');
      const data = await res.json();
      baseurlEl.textContent = data.base_url || '-';
      latencyEl.textContent = data.latency || '-';
      if (data.ok) {
        backendEl.textContent = 'OK';
        backendEl.className = 'ok';
      } else {
        backendEl.textContent = data.error || 'Unavailable';
        backendEl.className = 'bad';
      }
    }

    form.addEventListener('submit', saveSettings);
    document.querySelector('#reload').addEventListener('click', () => loadSettings().catch(err => setStatus(err.message, 'bad')));
    document.querySelector('#check-backend').addEventListener('click', () => checkBackend().catch(err => {
      backendEl.textContent = err.message || String(err);
      backendEl.className = 'bad';
    }));

    loadSettings()
      .then(checkBackend)
      .catch(err => setStatus(err.message || String(err), 'bad'));
  </script>
</body>
</html>`
