"use client";

import { useCallback, useEffect, useState, useTransition } from "react";
import type { FleetSnapshot } from "@/lib/types";
import { formatMiles, remainingMiles } from "@/lib/types";

const REFRESH_MS = Number(process.env.NEXT_PUBLIC_REFRESH_MS || 120000);
const DRIVER_MODE_KEY = "fleet-driver-mode";

export function CarsBoard() {
  const [snap, setSnap] = useState<FleetSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [tick, setTick] = useState(0);
  const [driverMode, setDriverMode] = useState(true);
  const [query, setQuery] = useState("");

  useEffect(() => {
    try {
      const saved = window.localStorage.getItem(DRIVER_MODE_KEY);
      // Default ON for drivers; only turn off if they explicitly saved "0".
      // eslint-disable-next-line react-hooks/set-state-in-effect -- storage is client-only; defer it until after hydration.
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
        const res = await fetch("/api/cars", { cache: "no-store" });
        const body = await res.json();
        if (!res.ok) throw new Error(body.error || res.statusText);
        setSnap(body as FleetSnapshot);
        setError(null);
        setTick((n) => n + 1);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Could not load cars");
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
        <p>Pulling the fleet roster…</p>
      </section>
    );
  }

  if (snap.cars.length === 0) {
    return (
      <section className="panel empty-panel">
        <h2>No cars yet</h2>
        <p>
          Run <code>oilchange sync-enterprise</code> then{" "}
          <code>oilchange sync</code> to refresh the mirror, or set Supabase
          env for ZacharyTFerguson&apos;s Project (<code>fleet_cars</code>).
        </p>
        <button type="button" className="cta" onClick={load}>
          Refresh
        </button>
      </section>
    );
  }

  const q = query.trim().toLowerCase();
  const visible = [...snap.cars]
    .filter((car) => {
      if (!q) return true;
      const hay = [car.nickname, car.plate, car.region, car.efleets_id, car.pdi_id]
        .join(" ")
        .toLowerCase();
      return hay.includes(q);
    })
    .sort((a, b) => (a.nickname || "").localeCompare(b.nickname || "", undefined, { sensitivity: "base" }));

  return (
    <section className="roster" aria-label="Fleet cars">
      <div className="roster-meta">
        <p>
          <span className="meta-label">Source</span> {snap.source}
        </p>
        <p>
          <span className="meta-label">Synced</span>{" "}
          {new Date(snap.synced_at).toLocaleString()}
        </p>
        <p>
          <span className="meta-label">Cars</span> {visible.length}
          {q ? ` / ${snap.cars.length}` : ""}
        </p>
        <div className="roster-actions">
          <button
            type="button"
            className={`cta driver-toggle ${driverMode ? "is-on" : ""}`}
            onClick={() => setDriverMode((v) => !v)}
            aria-pressed={driverMode}
            title="Bright outdoor-friendly contrast for phone use"
          >
            {driverMode ? "Driver mode on" : "Driver mode"}
          </button>
          <button
            type="button"
            className={`cta ghost ${pending ? "is-pending" : ""}`}
            onClick={load}
            disabled={pending}
          >
            {pending ? "Refreshing…" : "Refresh now"}
          </button>
        </div>
      </div>

      <label className="car-search">
        <span className="field-label">Find your car</span>
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Unit, plate, or region (e.g. CT1 or ABC123)"
          autoComplete="off"
          enterKeyHint="search"
        />
      </label>

      {visible.length === 0 ? (
        <p className="search-empty" role="status">
          No cars match “{query}”. Clear the search to see the full roster.
        </p>
      ) : null}

      <ul className="car-cards" key={tick}>
        {visible.map((car, i) => {
          const rem = remainingMiles(car);
          const onHold = Boolean(car.hold_reason);
          const statusLabel = onHold ? car.hold_reason || "Hold" : "Live";
          return (
            <li
              key={car.pdi_id || car.efleets_id}
              className={`car-card ${onHold ? "is-hold" : "is-live"}`}
              style={{ animationDelay: `${Math.min(i, 12) * 40}ms` }}
            >
              <div className="car-card-top">
                <div className="car-card-unit">
                  <span className="field-label">Unit</span>
                  <span className="unit-name">{car.nickname || "—"}</span>
                </div>
                <span
                  className={`status-badge ${onHold ? "hold" : "ok"}`}
                  title={onHold ? car.hold_reason || "Hold" : "Live"}
                >
                  <span className="field-label status-label">Status</span>
                  <span className="status-text">{statusLabel}</span>
                </span>
              </div>

              <div className="car-card-plate">
                <span className="field-label">Plate</span>
                <span className="plate-value">{car.plate || "—"}</span>
              </div>

              <div className="car-card-grid">
                <div className="car-field">
                  <span className="field-label">Remaining</span>
                  <span className="field-value mono">
                    {onHold ? "—" : formatMiles(rem)}
                    {!onHold && rem != null ? (
                      <span className="field-unit"> mi</span>
                    ) : null}
                  </span>
                </div>
                <div className="car-field">
                  <span className="field-label">Region</span>
                  <span className="field-value">{car.region || "—"}</span>
                </div>
                <div className="car-field">
                  <span className="field-label">Last oil</span>
                  <span className="field-value mono">
                    {formatMiles(car.last_oil_miles)}
                  </span>
                </div>
                <div className="car-field">
                  <span className="field-label">Last reading</span>
                  <span className="field-value mono">
                    {onHold ? "—" : formatMiles(car.last_reading_miles)}
                  </span>
                </div>
              </div>

              <div className="car-card-ids">
                <span>
                  <span className="field-label">PDI</span> {car.pdi_id || "—"}
                </span>
                <span>
                  <span className="field-label">eFleets</span>{" "}
                  {car.efleets_id || "—"}
                </span>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
