package desk

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"oilchange/internal/model"
)

// RecordsAPI is sqlite-backed fuel / shop RO / OneStep surfaces for /records/.
type RecordsAPI struct {
	Fuel         func(ctx context.Context) ([]model.CardTx, error)
	Maintenance  func(ctx context.Context) ([]model.ShopRO, error)
	OilChanges   func(ctx context.Context) ([]model.OilChange, error)
	Devices      func(ctx context.Context) ([]model.OneStepDevice, error)
	Miles        func(ctx context.Context) ([]model.DriveStopMiles, error)
	FuelSource   string
	MaintSource  string
	OneStepAuth  string
}

func serveJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method == http.MethodHead {
		w.WriteHeader(status)
		return
	}
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func serveFuel(w http.ResponseWriter, r *http.Request, api *RecordsAPI) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api == nil || api.Fuel == nil {
		serveJSON(w, r, http.StatusServiceUnavailable, map[string]string{"error": "no sqlite store — set OILCHANGE_DB"})
		return
	}
	txs, err := api.Fuel(r.Context())
	if err != nil {
		serveJSON(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	type row struct {
		CardID      string   `json:"card_id"`
		At          string   `json:"at"`
		Station     string   `json:"merchant"`
		Address     string   `json:"address"`
		Gallons     *float64 `json:"gallons"`
		Amount      *float64 `json:"amount"`
		Odometer    *int     `json:"odometer"`
		EFleetsID   string   `json:"enterprise_efleets_id"`
		Driver      string   `json:"driver"`
	}
	out := make([]row, 0, len(txs))
	sorted := append([]model.CardTx(nil), txs...)
	named := func(t model.CardTx) bool {
		n := strings.ToUpper(strings.TrimSpace(t.StationName))
		return n != "" && n != "TRACKER" && n != "UNKNOWN"
	}
	sort.Slice(sorted, func(i, j int) bool {
		ni, nj := named(sorted[i]), named(sorted[j])
		if ni != nj {
			return ni
		}
		return sorted[i].At.After(sorted[j].At)
	})
	for _, t := range sorted {
		driver := t.DriverFirst
		if t.DriverLast != "" {
			if driver != "" {
				driver += " "
			}
			driver += t.DriverLast
		}
		at := ""
		if !t.At.IsZero() {
			at = t.At.UTC().Format(time.RFC3339)
		}
		out = append(out, row{
			CardID:    t.CardID,
			At:        at,
			Station:   t.StationName,
			Address:   t.StationAddress,
			Gallons:   t.Gallons,
			Amount:    t.Amount,
			Odometer:  t.Odometer,
			EFleetsID: t.RecordedEFleetsID,
			Driver:    driver,
		})
	}
	src := api.FuelSource
	if src == "" {
		src = "sqlite card_transactions (eFleets DETAILS)"
	}
	serveJSON(w, r, http.StatusOK, map[string]any{
		"source": src,
		"count":  len(out),
		"fills":  out,
	})
}

func serveMaintenance(w http.ResponseWriter, r *http.Request, api *RecordsAPI) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api == nil || api.Maintenance == nil {
		serveJSON(w, r, http.StatusServiceUnavailable, map[string]string{"error": "no sqlite store — set OILCHANGE_DB"})
		return
	}
	ros, err := api.Maintenance(r.Context())
	if err != nil {
		serveJSON(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	type row struct {
		ROID      string `json:"ro_id"`
		EFleetsID string `json:"efleets_id"`
		At        string `json:"at"`
		Odometer  int    `json:"odometer"`
		Shop      string `json:"shop"`
		Service   string `json:"service"`
	}
	out := make([]row, 0, len(ros))
	for _, ro := range ros {
		at := ""
		if !ro.At.IsZero() {
			at = ro.At.UTC().Format(time.RFC3339)
		}
		out = append(out, row{
			ROID:      ro.ROID,
			EFleetsID: ro.EFleetsID,
			At:        at,
			Odometer:  ro.Odometer,
			Shop:      ro.LocationName,
			Service:   ro.ServiceDesc,
		})
	}
	src := api.MaintSource
	if src == "" {
		src = "sqlite shop_ros"
	}
	type oilRow struct {
		EFleetsID string `json:"efleets_id"`
		Miles     int    `json:"miles"`
		Date      string `json:"date"`
		Location  string `json:"location"`
		Source    string `json:"source"`
	}
	oils := []oilRow{}
	if api.OilChanges != nil {
		ocs, err := api.OilChanges(r.Context())
		if err != nil {
			serveJSON(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		for _, o := range ocs {
			d := ""
			if !o.Date.IsZero() {
				d = o.Date.UTC().Format("2006-01-02")
			}
			oils = append(oils, oilRow{
				EFleetsID: o.EFleetsID,
				Miles:     o.Miles,
				Date:      d,
				Location:  o.Location,
				Source:    o.Source,
			})
		}
	}
	serveJSON(w, r, http.StatusOK, map[string]any{
		"source":       src,
		"count":        len(out),
		"ros":          out,
		"oil_changes":  oils,
	})
}

func serveOneStepStatus(w http.ResponseWriter, r *http.Request, api *RecordsAPI) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api == nil || api.Devices == nil {
		serveJSON(w, r, http.StatusServiceUnavailable, map[string]string{"error": "no sqlite store — set OILCHANGE_DB"})
		return
	}
	devs, err := api.Devices(r.Context())
	if err != nil {
		serveJSON(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var miles []model.DriveStopMiles
	if api.Miles != nil {
		miles, err = api.Miles(r.Context())
		if err != nil {
			serveJSON(w, r, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	milesByFactory := map[string][]map[string]any{}
	for _, m := range miles {
		milesByFactory[m.FactoryID] = append(milesByFactory[m.FactoryID], map[string]any{
			"since": m.Since.UTC().Format(time.RFC3339),
			"miles": m.Miles,
		})
	}
	type row struct {
		FactoryID   string           `json:"factory_id"`
		DeviceID    string           `json:"device_id"`
		DisplayName string           `json:"display_name"`
		LinkedCar   string           `json:"linked_car_efleets_id"`
		Active      bool             `json:"active"`
		Dead        bool             `json:"dead"`
		DriveStop   []map[string]any `json:"drive_stop_miles"`
	}
	out := make([]row, 0, len(devs))
	linked := 0
	withMiles := 0
	for _, d := range devs {
		car := ""
		if d.LinkedCarEFleetsID != nil {
			car = *d.LinkedCarEFleetsID
		}
		if car != "" {
			linked++
		}
		ds := milesByFactory[d.FactoryID]
		if len(ds) > 0 {
			withMiles++
		}
		out = append(out, row{
			FactoryID:   d.FactoryID,
			DeviceID:    d.DeviceID,
			DisplayName: d.DisplayName,
			LinkedCar:   car,
			Active:      d.Active,
			Dead:        d.Dead,
			DriveStop:   ds,
		})
	}
	auth := api.OneStepAuth
	if auth == "" {
		auth = "unknown"
	}
	serveJSON(w, r, http.StatusOK, map[string]any{
		"auth_mode":          auth,
		"auth_note":          "RS256 JWT Bearer when PEM is present; never raw token. drive-stop uses device_id + dt_tracker_from + dt_tracker_to. Miles from distance / drive_stop_list only.",
		"devices":            len(out),
		"linked_factory_id":  linked,
		"with_drive_stop":    withMiles,
		"boxes":              out,
	})
}

func serveRecordsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write([]byte(recordsHTML))
}

const recordsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>Fleet records — fuel, shop RO, OneStep</title>
<style>
:root { color-scheme: light; --ink:#1a1a1a; --muted:#5c5c5c; --line:#d8d8d8; --hold:#b42318; --ok:#067647; }
body { font-family: ui-sans-serif, system-ui, sans-serif; margin: 0; color: var(--ink); background:#f6f4ef; }
header { padding: 1.25rem 1.5rem 0.5rem; }
nav a { margin-right: 0.9rem; color: inherit; }
h1 { font-size: 1.8rem; margin: 0.2rem 0; }
.lede, .src { color: var(--muted); max-width: 52rem; }
.banner { background:#fff3cd; border:1px solid #e6c200; padding:0.7rem 1.5rem; margin:0.8rem 0 0; }
section { padding: 1rem 1.5rem 2rem; }
table { border-collapse: collapse; width: 100%; background:#fff; }
th, td { border-bottom:1px solid var(--line); text-align:left; padding:0.4rem 0.5rem; font-size:0.9rem; vertical-align:top; }
th { font-size:0.75rem; letter-spacing:0.04em; text-transform:uppercase; color:var(--muted); }
.ok { color: var(--ok); font-weight: 600; }
.hold { color: var(--hold); }
.mono { font-family: ui-monospace, monospace; font-size: 0.82rem; }
</style>
</head>
<body>
<header>
<nav>
<a href="/">Oil Desk</a>
<a href="/records/">Records</a>
<a href="/history/">History (fills→cars)</a>
<a href="/cards/">Card pairing</a>
<a href="/devices/">Devices</a>
</nav>
<p class="brand">FLEET</p>
<h1>Records</h1>
<p class="lede">Shop/RO maintenance, eFleets Fuel &amp; Charging punches, and OneStep boxes.
Last Reading is Enterprise fill/shop odo + drive-stop <em>distance</em> only. OneStep odometer is never Last Reading. Miles are never invented.</p>
</header>
<div class="banner" id="banner">Loading sqlite + live labels…</div>
<section>
<h2>OneStep</h2>
<p class="src" id="os-src"></p>
<div id="os"></div>
</section>
<section>
<h2>Maintenance / shop RO</h2>
<p class="src" id="ro-src"></p>
<div id="ros"></div>
</section>
<section>
<h2>Fuel &amp; Charging (DETAILS)</h2>
<p class="src" id="fuel-src"></p>
<div id="fuel"></div>
</section>
<script>
function esc(s){return String(s??'').replace(/[&<>"]/g,c=>({ '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;' }[c]));}
function when(iso){ if(!iso) return '—'; const d=new Date(iso); return isNaN(d) ? iso : d.toLocaleString(); }
async function load(){
  const [fuel, maint, os] = await Promise.all([
    fetch('/api/fuel',{cache:'no-store'}).then(r=>r.json()),
    fetch('/api/maintenance',{cache:'no-store'}).then(r=>r.json()),
    fetch('/api/onestep',{cache:'no-store'}).then(r=>r.json()),
  ]);
  document.getElementById('banner').textContent =
    'Fuel '+ (fuel.count||0) +' punches · Shop RO '+ (maint.count||0) +
    ' · OneStep '+ (os.devices||0) +' boxes, '+ (os.linked_factory_id||0) +' paired by factory_id/VIN, '+
    (os.with_drive_stop||0) +' with measured drive-stop distance. Auth: '+ (os.auth_mode||'?');
  document.getElementById('os-src').textContent = (os.auth_note||'') + ' Mode: ' + (os.auth_mode||'');
  const boxes = (os.boxes||[]).filter(b => (b.drive_stop_miles&&b.drive_stop_miles.length) || b.linked_car_efleets_id).slice(0,80);
  document.getElementById('os').innerHTML = '<table><thead><tr><th>factory_id</th><th>device_id (History)</th><th>car</th><th>drive-stop miles</th><th>label</th></tr></thead><tbody>'+
    boxes.map(b=>{
      const miles=(b.drive_stop_miles||[]).map(m=>Number(m.miles).toFixed(2)+' since '+when(m.since)).join('<br>') || '—';
      return '<tr><td class="mono">'+esc(b.factory_id)+'</td><td class="mono">'+esc(b.device_id)+'</td><td class="mono">'+esc(b.linked_car_efleets_id)+'</td><td>'+miles+'</td><td>'+esc(b.display_name)+'</td></tr>';
    }).join('') + '</tbody></table>';
  document.getElementById('ro-src').textContent = 'Source: ' + (maint.source||'') + ' — ' + (maint.count||0) + ' shop ROs, ' + ((maint.oil_changes||[]).length) + ' oil-done rows. Not invented.';
  const oilRows=(maint.oil_changes||[]).map(o=>'<tr><td>'+esc(o.date)+'</td><td class="mono">'+esc(o.efleets_id)+'</td><td class="mono">oil-done</td><td>'+(o.miles??'—')+'</td><td>'+esc(o.location)+'</td><td>'+esc(o.source)+'</td></tr>').join('');
  document.getElementById('ros').innerHTML = '<table><thead><tr><th>when</th><th>car</th><th>RO</th><th>odo</th><th>shop</th><th>service</th></tr></thead><tbody>'+
    oilRows +
    (maint.ros||[]).map(r=>'<tr><td>'+esc(when(r.at))+'</td><td class="mono">'+esc(r.efleets_id)+'</td><td class="mono">'+esc(r.ro_id)+'</td><td>'+(r.odometer??'—')+'</td><td>'+esc(r.shop)+'</td><td>'+esc(r.service)+'</td></tr>').join('')
    + '</tbody></table>';
  document.getElementById('fuel-src').textContent = 'Source: ' + (fuel.source||'') + ' — ' + (fuel.count||0) + ' punches. Merchant / time / odo from the export.';
  const fills=(fuel.fills||[]).slice(0,400);
  document.getElementById('fuel').innerHTML = '<table><thead><tr><th>when</th><th>card</th><th>merchant</th><th>odo</th><th>gal</th><th>$</th><th>Enterprise vehicle</th></tr></thead><tbody>'+
    fills.map(f=>'<tr><td>'+esc(when(f.at))+'</td><td class="mono">'+esc(f.card_id)+'</td><td>'+esc(f.merchant)+'<br><span class="src">'+esc(f.address)+'</span></td><td>'+(f.odometer??'—')+'</td><td>'+(f.gallons??'—')+'</td><td>'+(f.amount??'—')+'</td><td class="mono">'+esc(f.enterprise_efleets_id)+'</td></tr>').join('')
    + '</tbody></table>';
}
load().catch(e=>{ document.getElementById('banner').textContent = 'Load failed: '+e; });
</script>
</body>
</html>
`
