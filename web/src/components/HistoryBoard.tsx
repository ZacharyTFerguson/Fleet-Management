"use client";

import { useCallback, useEffect, useState, useTransition, type ReactNode } from "react";
import type { FillBlock, HistoryBoard as Board } from "@/lib/history";
import { emptyBoard } from "@/lib/history";

const DRIVER_MODE_KEY = "fleet-driver-mode";

function when(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function miles(n?: number | null): string {
  if (n == null || Number.isNaN(n)) return "—";
  return `${n.toLocaleString("en-US")} mi`;
}

export function HistoryBoard() {
  const [board, setBoard] = useState<Board>(emptyBoard());
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [driverMode, setDriverMode] = useState(true);
  const [region, setRegion] = useState("");
  const [query, setQuery] = useState("");

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

  const load = useCallback((r?: string) => {
    startTransition(async () => {
      try {
        const q = new URLSearchParams();
        const use = r ?? region;
        if (use) q.set("region", use);
        const res = await fetch(`/api/history?${q}`, { cache: "no-store" });
        const body = await res.json();
        if (!res.ok) throw new Error(body.error || res.statusText);
        const next = body as Board;
        setBoard(next);
        if (!region && next.region) setRegion(next.region);
        setError(null);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Could not load history");
      }
    });
  }, [region]);

  useEffect(() => {
    load();
  }, [load]);

  const assign = (txKey: string, toEFleets: string, reason?: string) => {
    startTransition(async () => {
      try {
        const res = await fetch("/api/history/assign", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            tx_key: txKey,
            to_efleets_id: toEFleets,
            reason: reason || (toEFleets ? "manual_drag" : "undo"),
            region: board.region,
          }),
        });
        const body = await res.json();
        if (!res.ok) throw new Error(body.error || res.statusText);
        setBoard(body as Board);
        setError(null);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Could not move fill");
      }
    });
  };

  const turn = (dir: -1 | 1) => {
    const list = board.regions || [];
    if (list.length === 0) return;
    const i = Math.max(0, list.indexOf(board.region));
    const next = list[(i + dir + list.length) % list.length];
    setRegion(next);
    load(next);
  };

  const q = query.trim().toLowerCase();
  const match = (b: FillBlock) => {
    if (!q) return true;
    return [b.card_id, b.station, b.enterprise_name, b.gps_called_name, b.assigned_name, b.driver]
      .filter(Boolean)
      .join(" ")
      .toLowerCase()
      .includes(q);
  };

  if (error && board.swipes === 0) {
    return (
      <section className="panel error-panel" aria-live="polite">
        <h2>History needs sqlite</h2>
        <p>{error}</p>
        <button type="button" className="cta" onClick={() => load()}>
          Retry
        </button>
      </section>
    );
  }

  return (
    <section className="history-board" aria-label="Fill history">
      <div className="roster-meta">
        <p>
          <span className="meta-label">Assigned</span> {board.assigned_n}
        </p>
        <p>
          <span className="meta-label">Unassigned</span> {board.unassigned_n}
        </p>
        <p>
          <span className="meta-label">GPS review</span> {board.gps_flag_n}
        </p>
        <p>
          <span className="meta-label">Swipes</span> {board.swipes}
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
          <button type="button" className={`cta ghost ${pending ? "is-pending" : ""}`} onClick={() => load()} disabled={pending}>
            {pending ? "Saving…" : "Refresh"}
          </button>
        </div>
      </div>

      <div className="turnstile" role="group" aria-label="Region turnstile">
        <button type="button" className="cta ghost" onClick={() => turn(-1)} disabled={(board.regions || []).length < 2}>
          Previous
        </button>
        <p className="turnstile-region">
          <span className="field-label">Region</span>
          <span className="unit-name">{board.region || "—"}</span>
        </p>
        <button type="button" className="cta ghost" onClick={() => turn(1)} disabled={(board.regions || []).length < 2}>
          Next
        </button>
      </div>

      <label className="car-search">
        <span className="field-label">Find a fill</span>
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Card, station, unit"
          autoComplete="off"
        />
      </label>

      {error ? <p className="search-empty">{error}</p> : null}

      <DropLane
        label="Needs a home"
        hint="Drop here to undo. One block is one fill."
        onDrop={(key) => assign(key, "", "undo")}
      >
        {(board.unassigned || []).filter(match).map((b) => (
          <FillChip key={b.tx_key} block={b} onUndo={() => assign(b.tx_key, "", "undo")} />
        ))}
      </DropLane>

      <div className="history-columns">
        {(board.cars || []).map((car) => (
          <DropLane
            key={car.efleets_id}
            label={`${car.nickname || car.efleets_id} · ${car.pdi_id}`}
            hint={car.plate || car.efleets_id}
            onDrop={(key) => assign(key, car.efleets_id, "manual_drag")}
          >
            {(car.fills || []).filter(match).map((b) => (
              <FillChip key={b.tx_key} block={b} onUndo={() => assign(b.tx_key, "", "undo")} />
            ))}
          </DropLane>
        ))}
      </div>
    </section>
  );
}

function DropLane({
  label,
  hint,
  onDrop,
  children,
}: {
  label: string;
  hint: string;
  onDrop: (txKey: string) => void;
  children: ReactNode;
}) {
  const [over, setOver] = useState(false);
  return (
    <section
      className={`history-lane ${over ? "is-over" : ""}`}
      onDragOver={(e) => {
        e.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        e.preventDefault();
        setOver(false);
        const key = e.dataTransfer.getData("text/tx-key") || e.dataTransfer.getData("text/plain");
        if (key) onDrop(key);
      }}
    >
      <header className="history-lane-head">
        <h3>{label}</h3>
        <p>{hint}</p>
      </header>
      <div className="history-lane-body">{children}</div>
    </section>
  );
}

function FillChip({ block, onUndo }: { block: FillBlock; onUndo: () => void }) {
  return (
    <article
      className={`fill-chip ${block.gps_disagrees ? "is-flag" : ""}`}
      draggable
      onDragStart={(e) => {
        e.dataTransfer.setData("text/tx-key", block.tx_key);
        e.dataTransfer.setData("text/plain", block.tx_key);
        e.dataTransfer.effectAllowed = "move";
      }}
    >
      <p className="fill-chip-card">{block.card_id}</p>
      <p className="fill-chip-when">
        <span className="field-label">Date</span> {when(block.at)}
      </p>
      <p className="fill-chip-miles">
        <span className="field-label">Miles</span> {miles(block.odometer)}
      </p>
      <p className="fill-chip-station">{block.station || "—"}</p>
      <p className="fill-chip-meta">
        Enterprise {block.enterprise_name || block.enterprise_pdi_id || "—"}
        {block.gps_called_name ? ` · GPS ${block.gps_called_name}` : ""}
      </p>
      {block.gps_disagrees ? <p className="fill-chip-flag">GPS later disagrees — review</p> : null}
      {block.assigned_efleets_id ? (
        <button type="button" className="cta ghost fill-undo" onClick={onUndo}>
          Undo
        </button>
      ) : null}
    </article>
  );
}
