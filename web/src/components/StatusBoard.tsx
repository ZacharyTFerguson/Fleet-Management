"use client";

import { useEffect, useState } from "react";

type Endpoint = {
  ok: boolean;
  role?: string;
  detail?: string;
  project?: string;
  error?: string;
  write?: string;
};

type Report = {
  sqlite: Endpoint;
  neon: Endpoint;
  supabase: Endpoint;
  onestep: Endpoint;
  owners?: {
    working_store: string;
    desk_publish: string;
    backup: string;
    note: string;
  };
  user?: string;
  at?: string;
  error?: string;
  login?: string;
};

function Card({ title, ep }: { title: string; ep?: Endpoint }) {
  if (!ep) return null;
  return (
    <article className="car-card">
      <div className="car-card-head">
        <h2>{title}</h2>
        <span className={"status-badge " + (ep.ok ? "ok" : "hold")}>
          <span className="status-label">health</span>
          <span className="status-text">{ep.ok ? "live" : "hold"}</span>
        </span>
      </div>
      <p className="matchup-why">{ep.role}</p>
      {ep.project ? <p className="field-value">{ep.project}</p> : null}
      {ep.detail ? <p className="mono">{ep.detail}</p> : null}
      {ep.write ? <p className="matchup-why">Write: {ep.write}</p> : null}
      {ep.error ? <p className="matchup-why">{ep.error}</p> : null}
    </article>
  );
}

export function StatusBoard() {
  const [rep, setRep] = useState<Report | null>(null);
  const [err, setErr] = useState("");

  const load = () => {
    fetch("/api/status")
      .then(async (r) => {
        const j = await r.json();
        if (r.status === 401) {
          setErr("Sign in on Login to see live health.");
          setRep(j);
          return;
        }
        setErr("");
        setRep(j);
      })
      .catch(() => setErr("status unavailable"));
  };

  useEffect(() => {
    load();
  }, []);

  return (
    <section className="roster" aria-label="Connection health">
      <div className="roster-meta">
        <span>
          <span className="meta-label">Checked</span>
          {rep?.at || "—"}
        </span>
        {rep?.user ? (
          <span>
            <span className="meta-label">User</span>
            {rep.user}
          </span>
        ) : null}
        <div className="roster-actions">
          <button className="cta" type="button" onClick={load}>
            Recheck
          </button>
        </div>
      </div>
      {err ? <p className="search-empty">{err}</p> : null}
      {rep?.owners ? (
        <p className="matchup-why">
          {rep.owners.working_store} · {rep.owners.backup} · {rep.owners.desk_publish}
        </p>
      ) : null}
      <div className="status-grid">
        <Card title="SQLite" ep={rep?.sqlite} />
        <Card title="Neon" ep={rep?.neon} />
        <Card title="Supabase" ep={rep?.supabase} />
        <Card title="OneStep" ep={rep?.onestep} />
      </div>
    </section>
  );
}
