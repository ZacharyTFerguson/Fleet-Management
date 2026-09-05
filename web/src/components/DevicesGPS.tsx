"use client";

import { useState, useTransition } from "react";
import type { BoxEvidence, FillEvidence } from "@/lib/history";

export function DevicesGPS() {
  const [txKey, setTxKey] = useState("");
  const [factoryID, setFactoryID] = useState("");
  const [deviceID, setDeviceID] = useState("");
  const [hours, setHours] = useState("48");
  const [fill, setFill] = useState<FillEvidence | null>(null);
  const [box, setBox] = useState<BoxEvidence | null>(null);
  const [probe, setProbe] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  const loadFill = () => {
    startTransition(async () => {
      setError(null);
      setFill(null);
      try {
        const res = await fetch(`/api/devices/evidence?tx_key=${encodeURIComponent(txKey)}`, { cache: "no-store" });
        const body = await res.json();
        if (!res.ok) throw new Error(body.error || res.statusText);
        setFill(body as FillEvidence);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Could not load fill evidence");
      }
    });
  };

  const loadBox = () => {
    startTransition(async () => {
      setError(null);
      setBox(null);
      try {
        const q = new URLSearchParams();
        if (factoryID) q.set("factory_id", factoryID);
        if (deviceID) q.set("device_id", deviceID);
        const res = await fetch(`/api/devices/evidence?${q}`, { cache: "no-store" });
        const body = await res.json();
        if (!res.ok) throw new Error(body.error || res.statusText);
        setBox(body as BoxEvidence);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Could not load box evidence");
      }
    });
  };

  const runProbe = () => {
    startTransition(async () => {
      setError(null);
      setProbe(null);
      try {
        const res = await fetch("/api/devices/probe", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            factory_id: factoryID,
            device_id: deviceID,
            tx_key: txKey,
            hours: Number(hours) || 48,
          }),
        });
        const body = await res.json();
        if (!res.ok) throw new Error(body.error || res.statusText);
        setProbe(`device ${body.device_id}: ${body.miles} miles (${body.from} → ${body.to})`);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Probe failed");
      }
    });
  };

  return (
    <section className="roster" aria-label="GPS evidence">
      <p className="lede" style={{ marginTop: 0 }}>
        Devices are the GPS. Cache first. Live OneStep is one fill or one box —
        Watch / Probe-all comes after this path is safe.
      </p>
      {error ? <p className="search-empty">{error}</p> : null}

      <label className="car-search">
        <span className="field-label">Fill key (from History)</span>
        <input value={txKey} onChange={(e) => setTxKey(e.target.value)} placeholder="card|time|enterprise|odo" />
      </label>
      <div className="roster-actions" style={{ margin: "0 0 1rem" }}>
        <button type="button" className="cta" onClick={loadFill} disabled={pending || !txKey.trim()}>
          Show fill GPS
        </button>
      </div>

      {fill ? (
        <div className="car-card is-live">
          <p>
            <span className="field-label">Card</span> {fill.card_id}
          </p>
          <p>
            <span className="field-label">Enterprise</span> {fill.enterprise_efleets_id || "—"}
          </p>
          <p>
            <span className="field-label">Owner</span> {fill.assigned_efleets_id || "unassigned"}
          </p>
          <p>
            <span className="field-label">GPS called</span> {fill.gps_called_efleets_id || "—"}
            {fill.gps_disagrees ? " · review" : ""}
          </p>
          <p className="matchup-why">{fill.live_note}</p>
          <ul>
            {(fill.nearby || []).map((n) => (
              <li key={`${n.efleets_id}-${n.from}`}>
                {n.nickname || n.efleets_id} {n.pdi_id ? `· ${n.pdi_id}` : ""} · box {n.factory_id || "—"}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <label className="car-search">
        <span className="field-label">One box — factory_id</span>
        <input value={factoryID} onChange={(e) => setFactoryID(e.target.value)} placeholder="factory_id" />
      </label>
      <label className="car-search">
        <span className="field-label">Or device_id</span>
        <input value={deviceID} onChange={(e) => setDeviceID(e.target.value)} placeholder="device_id" />
      </label>
      <label className="car-search">
        <span className="field-label">Probe hours (max 48)</span>
        <input value={hours} onChange={(e) => setHours(e.target.value)} />
      </label>
      <div className="roster-actions" style={{ margin: "0 0 1rem" }}>
        <button type="button" className="cta ghost" onClick={loadBox} disabled={pending || (!factoryID && !deviceID)}>
          Show box
        </button>
        <button type="button" className="cta" onClick={runProbe} disabled={pending || (!factoryID && !deviceID && !txKey)}>
          {pending ? "Probing…" : "Live probe one box"}
        </button>
      </div>
      {box ? (
        <div className="car-card is-live">
          <p>
            <span className="field-label">factory_id</span> {box.factory_id}
          </p>
          <p>
            <span className="field-label">device_id</span> {box.device_id}
          </p>
          <p>
            <span className="field-label">VIN</span> {box.vin || "—"}
          </p>
          <p>
            <span className="field-label">Linked car</span> {box.linked_car_efleets_id || "unpaired"}
            {box.linked_car_pdi_id ? ` · ${box.linked_car_pdi_id}` : ""}
          </p>
          <p>
            <span className="field-label">Cached stops</span> {box.stops_in_cache ?? 0}
          </p>
          <p className="matchup-why">display_name is a label only: {box.display_name || "—"}</p>
        </div>
      ) : null}
      {probe ? <p className="search-empty">{probe}</p> : null}
    </section>
  );
}
