"use client";

import { useState } from "react";
import {
  FLEET_DEFAULT_INTERVAL_MILES,
  IMPREZA_2024_OIL as spec,
  OIL_CHANGE_BY_USE,
  OIL_CONSUMPTION_DRIVERS,
  SUBARU_SCHEDULE_MONTHS,
  type OilChangeUseId,
} from "@/lib/imprezaOil";

export function ImprezaOilChangeByUse() {
  const [useId, setUse] = useState<OilChangeUseId>("fleet");
  const lane = OIL_CHANGE_BY_USE.find((l) => l.id === useId) ?? OIL_CHANGE_BY_USE[1];

  return (
    <section className="impreza-oil iod-use-block" aria-labelledby="iod-use-title">
      <div className="iod-diagram-wrap">
        <svg
          className="iod-svg iod-use-svg"
          viewBox="0 0 1100 560"
          aria-labelledby="iod-use-title iod-use-desc"
        >
          <title id="iod-use-title">2024 Subaru Impreza oil change by use</title>
          <desc id="iod-use-desc">
            Change interval depends on how the car is used. Highway mixed driving follows
            the Subaru booklet at {OIL_CHANGE_BY_USE[0].changeLabel}. This fleet clocks{" "}
            {FLEET_DEFAULT_INTERVAL_MILES.toLocaleString()} miles. Severe use changes sooner
            and checks oil every second fuel fill. Every change is {spec.requiredViscosity}{" "}
            and a new filter, about {spec.capacityWithFilterUsQt} US qt.
          </desc>
          <defs>
            <marker
              id="iod-use-arrow"
              viewBox="0 0 10 10"
              refX="8"
              refY="5"
              markerWidth="7"
              markerHeight="7"
              orient="auto-start-reverse"
            >
              <path d="M 0 0 L 10 5 L 0 10 z" fill="var(--iod-oil)" />
            </marker>
          </defs>

          <rect className="iod-board" x="8" y="8" width="1084" height="544" rx="10" />

          <text className="iod-kicker" x="32" y="40">
            2024 SUBARU IMPREZA
          </text>
          <text className="iod-headline" x="32" y="76">
            Oil change by use
          </text>
          <text className="iod-sub" x="32" y="100">
            Same pour every time · {spec.requiredViscosity} + filter · {spec.capacityWithFilterUsQt}{" "}
            qt guideline · clock depends on driving
          </text>

          {OIL_CHANGE_BY_USE.map((l, i) => (
            <UseLane
              key={l.id}
              lane={l}
              x={36 + i * 354}
              y={148}
              selected={useId === l.id}
              onPick={() => setUse(l.id)}
            />
          ))}

          <g className="iod-flows" pointerEvents="none">
            <path className="iod-flow" d="M 196 368 V 400" markerEnd="url(#iod-use-arrow)" />
            <path className="iod-flow" d="M 550 368 V 400" markerEnd="url(#iod-use-arrow)" />
            <path className="iod-flow" d="M 904 368 V 400" markerEnd="url(#iod-use-arrow)" />
          </g>

          <g>
            <rect className="iod-card iod-use-result" x="36" y="408" width="1028" height="120" rx="8" />
            <text className="iod-kicker" x="56" y="436">
              At the change — every use
            </text>
            <text className="iod-card-value" x="56" y="474">
              {spec.requiredViscosity} + new filter · {spec.capacityWithFilterUsQt} US qt
            </text>
            <text className="iod-sub" x="56" y="500">
              Drain, replace the filter, fill, then finish on the dipstick. L→F is{" "}
              {spec.lowToFullUsQt} qt. Do not go above F when cold.
            </text>
            <text className="iod-sub" x="56" y="518">
              Time still counts: Subaru {SUBARU_SCHEDULE_MONTHS} months even if miles are low.
            </text>
          </g>
        </svg>
      </div>

      <div className="iod-use-picks" role="tablist" aria-label="Driving use">
        {OIL_CHANGE_BY_USE.map((l) => (
          <button
            key={l.id}
            type="button"
            role="tab"
            aria-selected={useId === l.id}
            className={`iod-step ${useId === l.id ? "is-on" : ""}`}
            onClick={() => setUse(l.id)}
          >
            <span className="iod-n">{l.n}</span>
            <span className="iod-step-copy">
              <strong>{l.title}</strong>
              <span>{l.when}</span>
            </span>
          </button>
        ))}
      </div>

      <p className="iod-live" aria-live="polite">
        <span className="field-label">This use</span>
        <strong>
          {lane.n}. {lane.title} — {lane.changeLabel}.
        </strong>{" "}
        {lane.action} {lane.check}
      </p>

      <ol className="iod-use-path" aria-label="From use to change">
        <li>
          <span className="field-label">Use</span>
          {lane.when}
        </li>
        <li>
          <span className="field-label">Oil is used</span>
          Driving style, idle, dust, heat, and cold change how fast the fill drops. Same car,
          different drivers, different results.
        </li>
        <li>
          <span className="field-label">Check</span>
          {lane.check}
        </li>
        <li>
          <span className="field-label">Change</span>
          {lane.changeDetail}. Always {spec.requiredViscosity} and a new filter.
        </li>
      </ol>

      <p className="iod-use-note">
        Severe / high-use checks: {OIL_CONSUMPTION_DRIVERS.join(" · ")}.
      </p>
    </section>
  );
}

function UseLane({
  lane,
  x,
  y,
  selected,
  onPick,
}: {
  lane: (typeof OIL_CHANGE_BY_USE)[number];
  x: number;
  y: number;
  selected: boolean;
  onPick: () => void;
}) {
  const lines = wrapLaneWhen(lane.when);
  return (
    <g className={`iod-hot ${selected ? "is-on" : ""}`} onClick={onPick}>
      <rect className="iod-card" x={x} y={y} width="332" height="212" rx="8" />
      <text className="iod-kicker" x={x + 18} y={y + 32}>
        {lane.n} · {lane.title}
      </text>
      <text className="iod-card-value" x={x + 18} y={y + 78}>
        {lane.changeLabel}
      </text>
      <text className="iod-sub" x={x + 18} y={y + 102}>
        {lane.changeDetail}
      </text>
      {lines.map((line, i) => (
        <text key={line} className="iod-sub" x={x + 18} y={y + 132 + i * 18}>
          {line}
        </text>
      ))}
    </g>
  );
}

function wrapLaneWhen(when: string): string[] {
  const words = when.split(" ");
  const lines: string[] = [];
  let cur = "";
  for (const w of words) {
    const next = cur ? `${cur} ${w}` : w;
    if (next.length > 38) {
      if (cur) lines.push(cur);
      cur = w;
    } else {
      cur = next;
    }
  }
  if (cur) lines.push(cur);
  return lines.slice(0, 4);
}
