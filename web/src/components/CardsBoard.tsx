"use client";

import { useCallback, useEffect, useState, useTransition } from "react";
import type { CardsSnapshot, GPSCardMatch, UnknownMatchup } from "@/lib/types";

const REFRESH_MS = Number(process.env.NEXT_PUBLIC_REFRESH_MS || 120000);
const DRIVER_MODE_KEY = "fleet-driver-mode";

function nick(snap: CardsSnapshot, id: string): string {
  if (!id) return "—";
  const n = snap.nicknames?.[id];
  return n ? `${n} · ${id}` : id;
}

function kindLabel(kind: string): string {
  if (kind === "suspect") return "Mismatch";
  if (kind === "ambiguous") return "Split";
  if (kind === "singleton") return "One swipe";
  return kind;
}

export function CardsBoard() {
  const [snap, setSnap] = useState<CardsSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [driverMode, setDriverMode] = useState(true);
  const [query, setQuery] = useState("");
  const [tab, setTab] = useState<"gps" | "unknown" | "stations">("gps");

  useEffect(() => {
    try {
      const saved = window.localStorage.getItem(DRIVER_MODE_KEY);
      if (saved === "0") setDriverMode(false);
      else setDriverMode(true);
    } catch {
      /* ignore */
    }
  }, []);

  useEffect(() => {
    document.documentElement.classList.toggle("driver-mode", driverMode);
    try {
      window.localStorage.setItem(DRIVER_MODE_KEY, driverMode ? "1" : "0");
    } catch {
      /* ignore */
    }
  }, [driverMode]);

  const load = useCallback(() => {
    startTransition(async () => {
      try {
        const res = await fetch("/api/cards", { cache: "no-store" });
        const body = await res.json();
        if (!res.ok) throw new Error(body.error || res.statusText);
        setSnap(body as CardsSnapshot);
        setError(null);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Could not load cards");
      }
    });
  }, []);

  useEffect(() => {
    load();
    const id = window.setInterval(load, Number.isFinite(REFRESH_MS) ? REFRESH_MS : 120000);
    return () => window.clearInterval(id);
  }, [load]);

  if (error && !snap) {
    return (
      <section className="panel error-panel" aria-live="polite">
        <h2>Signal lost</h2>
        <p>{error}</p>
        <button type="button" className="cta" onClick={load}>
          Retry
        </button>
      </section>
    );
  }

  if (!snap) {
    return (
      <section className="panel loading-panel" aria-busy="true">
        <div className="pulse-bar" />
        <p>Mapping stations and unknown matchups…</p>
      </section>
    );
  }

  const q = query.trim().toLowerCase();
  const unknown = snap.unknown.filter((u) => {
    if (!q) return true;
    const hay = [
      u.card_id,
      u.enterprise_car,
      u.best_car,
      u.runner_up_car,
      u.latest_station,
      u.why,
      snap.nicknames?.[u.enterprise_car],
      snap.nicknames?.[u.best_car],
    ]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    return hay.includes(q);
  });
  const stations = snap.stations.filter((s) => {
    if (!q) return true;
    return `${s.name} ${s.address}`.toLowerCase().includes(q);
  });

  return (
    <section className="roster" aria-label="Card matchups">
      <div className="roster-meta">
        <p>
          <span className="meta-label">Swipes</span> {snap.stats.swipes}
        </p>
        <p>
          <span className="meta-label">Cards</span> {snap.stats.cards}
        </p>
        <p>
          <span className="meta-label">Stations</span> {snap.stats.stations}
        </p>
        <p>
          <span className="meta-label">GPS best</span> {snap.stats.gps_best ?? 0}
        </p>
        <p>
          <span className="meta-label">Unknown</span> {snap.stats.unknown}
        </p>
        <div className="roster-actions">
          <button
            type="button"
            className={`cta driver-toggle ${driverMode ? "is-on" : ""}`}
            onClick={() => setDriverMode((v) => !v)}
            aria-pressed={driverMode}
          >
            {driverMode ? "Driver mode on" : "Driver mode"}
          </button>
          <button type="button" className={`cta ghost ${pending ? "is-pending" : ""}`} onClick={load} disabled={pending}>
            {pending ? "Refreshing…" : "Refresh now"}
          </button>
        </div>
      </div>

      <div className="desk-tabs" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={tab === "gps"}
          className={`cta ${tab === "gps" ? "is-on" : "ghost"}`}
          onClick={() => setTab("gps")}
        >
          GPS best {snap.stats.gps_best ?? 0}
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={tab === "unknown"}
          className={`cta ${tab === "unknown" ? "is-on" : "ghost"}`}
          onClick={() => setTab("unknown")}
        >
          Unknown {snap.stats.unknown}
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={tab === "stations"}
          className={`cta ${tab === "stations" ? "is-on" : "ghost"}`}
          onClick={() => setTab("stations")}
        >
          Stations {snap.stats.stations}
        </button>
      </div>

      <label className="car-search">
        <span className="field-label">
          {tab === "stations" ? "Find a station" : "Find a card or car"}
        </span>
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={tab === "stations" ? "Station name or address" : "Card, unit, or eFleets id"}
          autoComplete="off"
        />
      </label>

      {tab === "gps" ? (
        <GPSList
          snap={snap}
          rows={(snap.gps_best ?? []).filter((g) => {
            if (!g.best) return false;
            if (!q) return true;
            const hay = [g.card_id, g.efleets_id, snap.nicknames?.[g.efleets_id], ...(g.stations ?? [])]
              .filter(Boolean)
              .join(" ")
              .toLowerCase();
            return hay.includes(q);
          })}
          query={query}
        />
      ) : tab === "unknown" ? (
        <UnknownList snap={snap} rows={unknown} query={query} />
      ) : (
        <StationList rows={stations} query={query} />
      )}

      {snap.cars_without_card.length > 0 && tab === "unknown" ? (
        <p className="search-empty">
          {snap.cars_without_card.length} cars show up on swipes but no card votes them BEST:{" "}
          {snap.cars_without_card
            .slice(0, 12)
            .map((id) => nick(snap, id))
            .join(", ")}
          {snap.cars_without_card.length > 12 ? "…" : ""}
        </p>
      ) : null}
    </section>
  );
}

function GPSList({
  snap,
  rows,
  query,
}: {
  snap: CardsSnapshot;
  rows: GPSCardMatch[];
  query: string;
}) {
  if (rows.length === 0) {
    return (
      <p className="search-empty" role="status">
        {query
          ? `No GPS matchups for “${query}”.`
          : "No GPS stop matches yet. oilchange cards rebuild uses OneStep stop windows, then the swipe at that time."}
      </p>
    );
  }
  return (
    <ul className="car-cards">
      {rows.map((g, i) => {
        const enterprise = (g.enterprise_cars ?? []).join(", ");
        const clash = enterprise && !g.enterprise_cars?.includes(g.efleets_id);
        return (
          <li key={`${g.efleets_id}-${g.card_id}`} className={`car-card ${clash ? "is-hold" : "is-live"}`} style={{ animationDelay: `${Math.min(i, 12) * 40}ms` }}>
            <div className="car-card-top">
              <div className="car-card-unit">
                <span className="field-label">Car (GPS stop)</span>
                <span className="unit-name">{nick(snap, g.efleets_id)}</span>
              </div>
              <span className={`status-badge ${clash ? "hold" : "ok"}`}>
                <span className="field-label status-label">Hits</span>
                <span className="status-text">{g.evidence_n}</span>
              </span>
            </div>
            <div className="car-card-grid">
              <div className="car-field">
                <span className="field-label">Best card</span>
                <span className="field-value card-id">{g.card_id}</span>
              </div>
              <div className="car-field">
                <span className="field-label">Enterprise wrote</span>
                <span className="field-value">{enterprise ? nick(snap, enterprise.split(", ")[0] || "") : "—"}</span>
              </div>
            </div>
            <p className="matchup-why">{(g.stations ?? []).slice(0, 6).join(" · ") || "Station from the swipe at stop time"}</p>
          </li>
        );
      })}
    </ul>
  );
}

function UnknownList({
  snap,
  rows,
  query,
}: {
  snap: CardsSnapshot;
  rows: UnknownMatchup[];
  query: string;
}) {
  if (rows.length === 0) {
    return (
      <p className="search-empty" role="status">
        {query ? `No unknown matchups for “${query}”.` : "No unknown matchups. Run oilchange cards rebuild."}
      </p>
    );
  }
  return (
    <ul className="car-cards">
      {rows.map((u, i) => (
        <li key={u.card_id} className={`car-card ${u.kind === "suspect" ? "is-hold" : "is-live"}`} style={{ animationDelay: `${Math.min(i, 12) * 40}ms` }}>
          <div className="car-card-top">
            <div className="car-card-unit">
              <span className="field-label">Card</span>
              <span className="unit-name card-id">{u.card_id}</span>
            </div>
            <span className={`status-badge ${u.kind === "suspect" ? "hold" : "ok"}`}>
              <span className="field-label status-label">Kind</span>
              <span className="status-text">{kindLabel(u.kind)}</span>
            </span>
          </div>
          <div className="car-card-grid">
            <div className="car-field">
              <span className="field-label">Enterprise wrote</span>
              <span className="field-value">{nick(snap, u.enterprise_car)}</span>
            </div>
            <div className="car-field">
              <span className="field-label">Swipe majority</span>
              <span className="field-value">
                {nick(snap, u.best_car)}
                {u.best_n ? <span className="field-unit"> n={u.best_n}</span> : null}
              </span>
            </div>
            <div className="car-field">
              <span className="field-label">Station</span>
              <span className="field-value">{u.latest_station || "—"}</span>
            </div>
            <div className="car-field">
              <span className="field-label">Last swipe</span>
              <span className="field-value mono">
                {u.latest_at ? new Date(u.latest_at).toLocaleString() : "—"}
              </span>
            </div>
          </div>
          <p className="matchup-why">{u.why}</p>
          {u.neighbors && u.neighbors.length > 0 ? (
            <div className="car-card-ids">
              {u.neighbors.map((n) => (
                <span key={`${n.efleets_id}-${n.card_id}-${n.at}`}>
                  <span className="field-label">Also at pump</span> {nick(snap, n.efleets_id)} · {n.card_id} · {n.days_apart}d
                </span>
              ))}
            </div>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

function StationList({
  rows,
  query,
}: {
  rows: CardsSnapshot["stations"];
  query: string;
}) {
  if (rows.length === 0) {
    return (
      <p className="search-empty" role="status">
        {query ? `No stations for “${query}”.` : "No stations yet. Ingest DETAILS then cards rebuild."}
      </p>
    );
  }
  return (
    <ul className="car-cards">
      {rows.map((s, i) => (
        <li key={s.key} className="car-card is-live" style={{ animationDelay: `${Math.min(i, 12) * 40}ms` }}>
          <div className="car-card-top">
            <div className="car-card-unit">
              <span className="field-label">Station</span>
              <span className="unit-name station-name">{s.name || "—"}</span>
            </div>
          </div>
          <p className="matchup-why">{s.address || "No address on the swipe"}</p>
          <div className="car-card-grid">
            <div className="car-field">
              <span className="field-label">Swipes</span>
              <span className="field-value mono">{s.swipes}</span>
            </div>
            <div className="car-field">
              <span className="field-label">Cars</span>
              <span className="field-value mono">{s.cars}</span>
            </div>
            <div className="car-field">
              <span className="field-label">Cards</span>
              <span className="field-value mono">{s.cards}</span>
            </div>
          </div>
        </li>
      ))}
    </ul>
  );
}
