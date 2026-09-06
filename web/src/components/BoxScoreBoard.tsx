"use client";

import { useEffect, useMemo, useState } from "react";

type Row = {
  efleets_id: string;
  card_id?: string;
  punch_at: string;
  merchant?: string;
  recorded: number;
  maint_odo?: number;
  maint_at?: string;
  miles_since?: number;
  expected?: number;
  overage: number;
  shortage: number;
  abs_diff?: number;
  trend: string;
  status: string;
  hold_reason?: string;
  hold_detail?: string;
  in_trend: boolean;
};

type Vehicle = {
  efleets_id: string;
  maint_odo?: number;
  maint_at?: string;
  has_maint?: boolean;
  rows: Row[];
  sum_overage: number;
  sum_shortage: number;
  latest_abs_diff?: number;
  latest_trend?: string;
};

type Fleet = {
  at?: string;
  note?: string;
  vehicles: Vehicle[];
  sum_overage: number;
  sum_shortage: number;
  trend_up: number;
  trend_down: number;
  trend_flat: number;
  suspect_n: number;
  hold_n: number;
  trusted_n: number;
  error?: string;
};

type Filter = "all" | "trusted" | "suspect" | "hold";

export function BoxScoreBoard() {
  const [fleet, setFleet] = useState<Fleet | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");
  const [filter, setFilter] = useState<Filter>("all");

  const load = (path = "/api/boxscore") => {
    fetch(path)
      .then(async (r) => {
        const j = await r.json();
        if (r.status === 401) {
          setErr("Sign in on Login to see the mileage box score.");
          return;
        }
        if (!r.ok) {
          setErr(j.error || "unavailable");
          return;
        }
        setErr("");
        setFleet(j);
      })
      .catch(() => setErr("unavailable"));
  };

  useEffect(() => {
    load();
  }, []);

  const act = async (path: string, body?: object) => {
    setBusy(path);
    setErr("");
    try {
      const res = await fetch(`/api/boxscore/${path}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body || {}),
      });
      const j = await res.json();
      if (!res.ok) {
        setErr(j.error || path + " failed");
        return;
      }
      if (path === "rebuild") {
        setFleet(j);
      } else {
        load();
      }
    } finally {
      setBusy("");
    }
  };

  const rows = useMemo(() => {
    const all: Row[] = [];
    for (const v of fleet?.vehicles || []) {
      for (const r of v.rows || []) {
        all.push(r);
      }
    }
    if (filter === "all") return all;
    if (filter === "hold") return all.filter((r) => r.status === "hold");
    return all.filter((r) => r.status === filter);
  }, [fleet, filter]);

  return (
    <section className="roster" aria-label="Mileage box score">
      <div className="roster-meta">
        <span>
          <span className="meta-label">Trusted</span>
          {fleet?.trusted_n ?? 0}
        </span>
        <span>
          <span className="meta-label">Suspect</span>
          {fleet?.suspect_n ?? 0}
        </span>
        <span>
          <span className="meta-label">HOLD</span>
          {fleet?.hold_n ?? 0}
        </span>
        <span>
          <span className="meta-label">Overage</span>
          {fleet?.sum_overage ?? 0}
        </span>
        <span>
          <span className="meta-label">Shortage</span>
          {fleet?.sum_shortage ?? 0}
        </span>
        <span>
          <span className="meta-label">Gap ↑ / ↓ / flat</span>
          {fleet?.trend_up ?? 0} / {fleet?.trend_down ?? 0} / {fleet?.trend_flat ?? 0}
        </span>
        <div className="roster-actions">
          <button className="cta" type="button" disabled={!!busy} onClick={() => act("rebuild")}>
            {busy === "rebuild" ? "Scoring…" : "Rebuild from stored windows"}
          </button>
        </div>
      </div>
      {fleet?.note ? <p className="matchup-why">{fleet.note}</p> : null}
      {err ? <p className="search-empty">{err}</p> : null}

      <div className="roster-actions filter-row">
        {(["all", "trusted", "suspect", "hold"] as Filter[]).map((f) => (
          <button
            key={f}
            className={"cta ghost" + (filter === f ? " is-current" : "")}
            type="button"
            onClick={() => setFilter(f)}
          >
            {f}
          </button>
        ))}
      </div>

      <div className="marker-table-wrap">
        <table className="marker-table">
          <thead>
            <tr>
              <th>Car</th>
              <th>Punch</th>
              <th>Recorded</th>
              <th>Expected</th>
              <th>Over / short</th>
              <th>|gap|</th>
              <th>Trend</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={`${r.efleets_id}|${r.punch_at}|${r.recorded}`}>
                <td className="mono">{r.efleets_id}</td>
                <td>
                  <div className="mono">{r.punch_at}</div>
                  {r.merchant ? <div className="matchup-why">{r.merchant}</div> : null}
                </td>
                <td className="mono">{r.recorded}</td>
                <td className="mono">{r.expected ?? "—"}</td>
                <td className="mono">
                  {r.overage}/{r.shortage}
                </td>
                <td className="mono">{r.abs_diff ?? "—"}</td>
                <td>
                  <span className={"trend-badge " + r.trend}>
                    {r.in_trend ? r.trend : "out"}
                  </span>
                </td>
                <td>
                  <span className={"status-badge " + (r.status === "trusted" ? "ok" : "hold")}>
                    <span className="status-text">{r.status}</span>
                  </span>
                  {r.hold_detail ? <div className="matchup-why">{r.hold_detail}</div> : null}
                </td>
                <td>
                  <div className="roster-actions">
                    {r.status === "hold" && r.hold_reason === "NO_DRIVESTOP" ? (
                      <button
                        className="cta ghost"
                        type="button"
                        disabled={!!busy}
                        onClick={() =>
                          act("measure", { efleets_id: r.efleets_id, punch_at: r.punch_at })
                        }
                      >
                        Measure
                      </button>
                    ) : null}
                    {r.status === "suspect" || r.status === "hold" ? (
                      <>
                        <button
                          className="cta ghost"
                          type="button"
                          disabled={!!busy}
                          onClick={() =>
                            act("dismiss", {
                              efleets_id: r.efleets_id,
                              punch_at: r.punch_at,
                              recorded: r.recorded,
                            })
                          }
                        >
                          Dismiss
                        </button>
                        <button
                          className="cta ghost"
                          type="button"
                          disabled={!!busy}
                          onClick={() =>
                            act("correct", {
                              efleets_id: r.efleets_id,
                              punch_at: r.punch_at,
                              recorded: r.recorded,
                            })
                          }
                        >
                          Corrected
                        </button>
                      </>
                    ) : null}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
