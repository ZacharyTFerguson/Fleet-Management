"use client";

import { useEffect, useState } from "react";

type Job = {
  id: string;
  general_code: string;
  stage: string;
  draft: {
    name: string;
    address: string;
    group: string;
    lat?: number;
    lng?: number;
    zone?: { radius_m: number; prefer: string; zone_type?: string; vertices?: number[]; shape?: string };
  };
  review_notes?: string;
  third_party_lat?: number;
  third_party_lng?: number;
  third_party_provider?: string;
  onestep_lat?: number;
  onestep_lng?: number;
  has_confirm?: boolean;
  last_error?: string;
  map_url?: string;
};

type List = {
  jobs: Job[];
  places: number;
  note?: string;
  error?: string;
  write_proven?: boolean;
  portal_url?: string;
};

export function StationsBoard() {
  const [list, setList] = useState<List | null>(null);
  const [err, setErr] = useState("");
  const [sel, setSel] = useState<Job | null>(null);
  const [token, setToken] = useState("");
  const [confirm, setConfirm] = useState("");
  const [dry, setDry] = useState<string>("");
  const [busy, setBusy] = useState("");

  const load = () => {
    fetch("/api/markers")
      .then(async (r) => {
        const j = await r.json();
        if (r.status === 401) {
          setErr("Sign in on Login to run the Gas Stations pipeline.");
          return;
        }
        if (!r.ok) {
          setErr(j.error || "unavailable");
          return;
        }
        setErr("");
        setList(j);
      })
      .catch(() => setErr("unavailable"));
  };

  useEffect(() => {
    load();
  }, []);

  const act = async (id: string, path: string, body?: object) => {
    setBusy(path);
    setErr("");
    try {
      const res = await fetch(`/api/markers/${id}/${path}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body || {}),
      });
      const j = await res.json();
      if (!res.ok) {
        setErr(j.error || path + " failed");
        return;
      }
      if (path === "review") {
        setToken(j.confirm_token || "");
        setSel(j.job);
      } else if (path === "dry-run") {
        setDry(JSON.stringify(j, null, 2));
      } else {
        setSel(j);
      }
      load();
    } finally {
      setBusy("");
    }
  };

  return (
    <section className="roster" aria-label="Gas station markers">
      <div className="roster-meta">
        <span>
          <span className="meta-label">Places</span>
          {list?.places ?? 0}
        </span>
        <span>
          <span className="meta-label">Jobs</span>
          {list?.jobs?.length ?? 0}
        </span>
        <div className="roster-actions">
          <button
            className="cta"
            type="button"
            disabled={!!busy}
            onClick={async () => {
              setBusy("pull");
              const res = await fetch("/api/markers", { method: "POST" });
              const j = await res.json();
              if (!res.ok) setErr(j.error || "pull failed");
              else {
                setList(j);
                setErr("");
              }
              setBusy("");
            }}
          >
            {busy === "pull" ? "Pulling…" : "Pull gas candidates"}
          </button>
          <button
            className="cta"
            type="button"
            disabled={!!busy}
            onClick={async () => {
              setBusy("onestep");
              const res = await fetch("/api/markers/onestep", { method: "POST" });
              const j = await res.json();
              if (!res.ok) setErr(j.error || "OneStep download failed");
              else {
                setList(j);
                setErr("");
              }
              setBusy("");
            }}
          >
            {busy === "onestep" ? "Downloading…" : "Download Gas_Stations from OneStep"}
          </button>
        </div>
      </div>
      {list?.note ? <p className="matchup-why">{list.note}</p> : null}
      {list?.portal_url ? (
        <p className="matchup-why">
          {list.write_proven
            ? "API write is opted in (ONESTEP_WRITE_PROVEN). Still confirm-gated, one marker at a time."
            : "API create is not proven on this key. Draw the zone in the portal after review."}{" "}
          <a className="portal-link" href={list.portal_url} target="_blank" rel="noreferrer">
            Open OneStep map
          </a>
        </p>
      ) : null}
      {err ? <p className="search-empty">{err}</p> : null}

      <div className="marker-table-wrap">
        <table className="marker-table">
          <thead>
            <tr>
              <th>Canon label</th>
              <th>Address</th>
              <th>Stage</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {(list?.jobs || []).map((j) => (
              <tr key={j.id} className={sel?.id === j.id ? "is-sel" : ""}>
                <td className="mono">{j.draft?.name}</td>
                <td>{j.draft?.address}</td>
                <td>
                  <span className={"status-badge " + (j.stage === "hold" ? "hold" : "ok")}>
                    <span className="status-text">{j.stage}</span>
                  </span>
                </td>
                <td>
                  <button className="cta ghost" type="button" onClick={() => setSel(j)}>
                    Open
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {sel ? (
        <article className="car-card marker-detail">
          <div className="car-card-head">
            <h2>{sel.draft?.name}</h2>
            <span className="status-badge ok">
              <span className="status-text">{sel.stage}</span>
            </span>
          </div>
          <p className="matchup-why">
            Group {sel.draft?.group}
            {sel.draft?.zone?.zone_type ? ` · ${sel.draft.zone.zone_type}` : ""}
            {sel.draft?.zone?.vertices?.length
              ? ` · ${sel.draft.zone.vertices.length / 2} vertices`
              : ` · zone ${sel.draft?.zone?.radius_m}m ${sel.draft?.zone?.prefer}`}
          </p>
          <p>{sel.draft?.address}</p>
          {sel.last_error ? <p className="search-empty">{sel.last_error}</p> : null}
          {sel.map_url ? (
            <iframe title="Third-party map check" className="map-preview" src={sel.map_url} />
          ) : (
            <p className="matchup-why">Geocode to preview the third-party map (OpenStreetMap).</p>
          )}
          {sel.third_party_lat != null ? (
            <p className="mono">
              Third-party ({sel.third_party_provider}): {sel.third_party_lat}, {sel.third_party_lng}
            </p>
          ) : null}
          {sel.onestep_lat != null ? (
            <p className="mono">
              OneStep GPS: {sel.onestep_lat}, {sel.onestep_lng}
            </p>
          ) : null}
          <div className="roster-actions">
            <button className="cta" type="button" disabled={!!busy} onClick={() => act(sel.id, "geocode")}>
              Third-party check
            </button>
            <button
              className="cta"
              type="button"
              disabled={!!busy}
              onClick={() => act(sel.id, "review", { action: "approve" })}
            >
              Approve
            </button>
            <button
              className="cta ghost"
              type="button"
              disabled={!!busy}
              onClick={() => act(sel.id, "review", { action: "hold", notes: "operator hold" })}
            >
              HOLD
            </button>
            <button className="cta" type="button" disabled={!!busy} onClick={() => act(sel.id, "dry-run")}>
              Dry-run
            </button>
          </div>
          {token ? <p className="mono">Confirm token (not a credential): {token}</p> : null}
          <label className="field-label">
            Type SEND_TO_ONESTEP to unlock one create
            <input className="secret-input" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
          </label>
          <div className="roster-actions">
            {list?.portal_url ? (
              <a className="cta" href={list.portal_url} target="_blank" rel="noreferrer">
                Open in portal
              </a>
            ) : null}
            <button
              className="cta"
              type="button"
              disabled={!!busy || confirm !== "SEND_TO_ONESTEP" || !list?.write_proven}
              onClick={() => act(sel.id, "send", { confirm, confirm_token: token })}
            >
              Send marker to OneStep
            </button>
            <button
              className="cta"
              type="button"
              disabled={!!busy || confirm !== "SEND_TO_ONESTEP" || !list?.write_proven}
              onClick={() => act(sel.id, "zone", { confirm, confirm_token: token })}
            >
              Place zone near marker
            </button>
          </div>
          {!list?.write_proven ? (
            <p className="matchup-why">
              Send/zone stay locked until a live POST/PUT is proven and{" "}
              <code>ONESTEP_WRITE_PROVEN=1</code> is set. Dry-run still lists the
              proven GET paths.
            </p>
          ) : null}
          {dry ? <pre className="dry-run">{dry}</pre> : null}
        </article>
      ) : null}
    </section>
  );
}
