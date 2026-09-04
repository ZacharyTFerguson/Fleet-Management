# Chrome-session Enterprise adapter (blind)

For Grok Build and `oilchange`: drive a **already logged-in** eFleets Chrome tab. No password typing. No screenshot vision. Guide by **URL + DOM selectors + DevTools Network**.

Today `internal/enterprise` has:
- `FileAdapter` — CSV/xlsx drop (works offline)
- `HTTPAdapter` — username/password cookie jar (blind but needs secrets; MFA/JS wall fails closed)

Add a third: **`ChromeSessionAdapter`** — attach to local Chrome CDP, use the open session, fetch the same exports the parsers already understand.

## Operator one-time: start Chrome with CDP

On the machine that already has eFleets signed in (`FergFam`):

```powershell
# Close other Chromes first, or use a dedicated profile dir.
& "$env:ProgramFiles\Google\Chrome\Application\chrome.exe" `
  --remote-debugging-port=9222 `
  --user-data-dir="$env:LOCALAPPDATA\oilchange-chrome-profile" `
  "https://login.efleets.com/fleetweb"
```

Sign in **once** by hand in that window (saved Log in is fine; never put the password in env for this path). Leave the tab open.

Env for oilchange (gitignored `oilchange.env`):

```bash
EFLEETS_CDP_URL=http://127.0.0.1:9222
EFLEETS_CUST_NUM=583424
# Do NOT set EFLEETS_USERNAME / EFLEETS_PASSWORD for this path.
```

If CDP is down or no eFleets tab is logged in → fail with a clear error; fall back to `--vehicles` / `--fuel-details` file flags. Never invent CSVs.

## Prefer Network over clicking when possible

Blind agents should **capture export URLs once** from DevTools, then reuse them with the session cookies from CDP (or paste into `EFLEETS_*_URL`).

### DevTools recipe (human or agent)

1. Open logged-in eFleets.
2. F12 → **Network** → clear → check **Preserve log**.
3. Navigate and trigger each export (steps below).
4. Filter `xlsx` / `csv` / `download` / `export`.
5. Right-click the successful download request → **Copy → Copy link address** (CDP can also pull the body with the page session).

Paste into env when stable:

```bash
EFLEETS_DETAILS_URL=...      # Fuel & Charging DETAILS Excel/CSV
EFLEETS_MAINT_URL=...        # Maintenance Detail
EFLEETS_FLEETSUMMARY_URL=... # Fleet Summary / All Cars
```

`HTTPAdapter.urlFor` already refuses to guess these paths. Chrome adapter should either set those URLs after discovering them, or download via the click path and return bytes to the existing parsers.

## Blind click map (JS pointers)

Company: PDI Services LLC `583424`. One tab per destination.

### A. Fuel & Charging DETAILS (fill seconds)

Landing URL (navigate; do not hunt the menu if possible):

```
https://login.efleets.com/fleetweb/fuel?fuelTab=fuel
```

Paste `docs/efleets-blind-pointers.js` into DevTools Console on that page.

Agent algorithm (no vision):

1. `Page.navigate` → fuel DETAILS URL above.
2. Wait until `document.readyState === "complete"` and URL contains `fuel`.
3. Run the pointer script; prefer controls whose text matches download, then **Excel** (not CSV/PDF).
4. Leave date range on **30 Days** unless instructed otherwise.
5. On download: CDP download behavior into `data/runtime/efleets/` or Network response body.
6. File name pattern often `DETAILS_583424_*.xlsx`. Row 1 title; **headers row 2** (parsers stay header-by-name).

Do **not** search by nickname for the fleet sync. Fleet-wide DETAILS export is preferred for `sync-enterprise`; per-vehicle search is for audits.

### B. Maintenance / shop RO

Try:

```
https://login.efleets.com/fleetweb/maintenanceSummary?maintenanceTab=detail
```

Same pointer script (`maintenance|detail|download|excel`). Capture `EFLEETS_MAINT_URL` from Network.

### C. Fleet Summary / vehicles

From All Cars / fleet summary grid, trigger CSV/Excel download. Capture `EFLEETS_FLEETSUMMARY_URL`. Match cars later by eFleets id header — never nickname.

## CDP adapter shape (for Grok Build)

New type next to `HTTPAdapter` / `FileAdapter`:

```go
// ChromeSessionAdapter attaches to an existing Chrome (CDP). No password.
// Blind: navigate by URL, click by text/selector pointers, capture download bytes.
type ChromeSessionAdapter struct {
    CDPURL      string // http://127.0.0.1:9222
    CustNum     string
    DetailsURL  string // optional pre-captured
    MaintURL    string
    FleetURL    string
    DownloadDir string
}

func (a *ChromeSessionAdapter) Fetch(ctx context.Context, kind ReportKind) ([]byte, string, error)
```

Selection order in `sync-enterprise`:

1. Explicit `--vehicles` / `--fuel-details` / `--shop-ro` flags → `FileAdapter`
2. Else if `EFLEETS_CDP_URL` set → `ChromeSessionAdapter`
3. Else if username+password set → `HTTPAdapter` (legacy; MFA may fail)
4. Else error: need files or a live Chrome session

Never call `ChromeSessionAdapter` and `HTTPAdapter` password login in the same run.

### CDP checklist

1. `GET {CDPURL}/json/list` → find tab where `url` contains `login.efleets.com`.
2. If none: error `no logged-in eFleets tab; open Chrome with CDP and sign in`.
3. Connect WebSocket to that tab's `webSocketDebuggerUrl`.
4. `Runtime.evaluate` pointer script; click by matching text.
5. `Page.setDownloadBehavior` or Network get body.
6. Hand bytes to existing `parse.go` header parsers — **do not invent columns**.

## Auth / safety

- Password fields blank → hold. CDP path must not fill them.
- Do not scrape cookies into chat, Slack, or git.
- Do not commit `oilchange.env`, PII dumps, or HAR files.
- Login page HTML (`Client Login` + `password`) → session dead; stop; ask human to re-auth in the CDP Chrome.

## What Grok Build should land first

1. This doc + `efleets-blind-pointers.js` (Cheif dropped both under `docs/`).
2. `EFLEETS_CDP_URL` in `config` + `oilchange env` presence line.
3. `ChromeSessionAdapter` stub: list tabs, clear error if missing, optional navigate to DETAILS URL.
4. Wire selection order in `cmdSyncEnterprise` / `SyncEnterprise`.
5. Optional: `oilchange sync-enterprise --discover-efleets` prints pointer table JSON.

Leave Oil Desk / local chat room / Neon backup work alone while landing this.

## Locked product rules (do not regress)

- Last Reading = Enterprise odo at trusted fill second + OneStep drive-stop miles-since.
- HOLD skips Last Reading write.
- Never OneStep odometer as Last Reading.
- Header-resolved columns only.
- Neon = `Fleet_Management_Neon` / `Fleet_Manage_Oil` backup; SQLite remains daily driver.
