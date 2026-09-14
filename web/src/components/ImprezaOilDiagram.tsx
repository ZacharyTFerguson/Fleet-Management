"use client";

import { useState } from "react";
import {
  FLEET_DEFAULT_INTERVAL_MILES,
  IMPREZA_2024_OIL as spec,
  OIL_CIRCUIT_STEPS,
  OIL_CONSUMPTION_DRIVERS,
  SUBARU_SCHEDULE_MILES,
  SUBARU_SCHEDULE_MONTHS,
  type OilCircuitStepId,
} from "@/lib/imprezaOil";

export function ImprezaOilDiagram() {
  const uid = "impreza-oil";
  const titleId = `${uid}-title`;
  const descId = `${uid}-desc`;
  const [step, setStep] = useState<OilCircuitStepId>("filter");
  const current = OIL_CIRCUIT_STEPS.find((s) => s.id === step) ?? OIL_CIRCUIT_STEPS[2];

  return (
    <section className="impreza-oil" aria-labelledby={titleId}>
      <div className="iod-diagram-wrap">
        <svg
          className="iod-svg"
          viewBox="0 0 1100 660"
          aria-labelledby={`${titleId} ${descId}`}
        >
          <title id={titleId}>2024 Subaru Impreza engine oil use</title>
          <desc id={descId}>
            {spec.displacement} {spec.engine}. Required {spec.requiredViscosity}{" "}
            {spec.requiredForm}, {spec.requiredGrade}. {spec.capacityWithFilterUsQt} US
            quarts with filter. Low to Full {spec.lowToFullUsQt} US quarts.
          </desc>
          <defs>
            <linearGradient id={`${uid}-oil`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--iod-oil)" stopOpacity="0.95" />
              <stop offset="100%" stopColor="var(--iod-oil-deep)" />
            </linearGradient>
            <linearGradient id={`${uid}-metal`} x1="0" y1="0" x2="1" y2="1">
              <stop offset="0%" stopColor="var(--iod-metal-hi)" />
              <stop offset="100%" stopColor="var(--iod-metal)" />
            </linearGradient>
            <filter id={`${uid}-glow`} x="-20%" y="-20%" width="140%" height="140%">
              <feGaussianBlur stdDeviation="3" result="b" />
              <feMerge>
                <feMergeNode in="b" />
                <feMergeNode in="SourceGraphic" />
              </feMerge>
            </filter>
            <marker
              id={`${uid}-arrow`}
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

          <rect className="iod-board" x="8" y="8" width="1084" height="644" rx="10" />

          <text className="iod-kicker" x="32" y="40">
            2024 SUBARU IMPREZA
          </text>
          <text className="iod-headline" x="32" y="76">
            Engine oil use
          </text>
          <text className="iod-sub" x="32" y="100">
            {spec.engine} · {spec.displacement} · DOHC DIT · firing {spec.firingOrder}
          </text>

          {/* Boxer block */}
          <g className={on("gallery", step)} onClick={() => setStep("gallery")}>
            <rect
              className="iod-block"
              x="248"
              y="200"
              width="230"
              height="138"
              rx="14"
              fill={`url(#${uid}-metal)`}
            />
            <rect className="iod-gallery" x="208" y="258" width="384" height="16" rx="8" />
            <text className="iod-node-label" x="363" y="232" textAnchor="middle">
              Crankcase
            </text>
            <text className="iod-node-label iod-on-oil iod-small" x="363" y="270" textAnchor="middle">
              Main gallery
            </text>
          </g>

          <g className={on("banks", step)} onClick={() => setStep("banks")}>
            <Bank x={88} y={198} side="left" />
            <Bank x={478} y={198} side="right" />
          </g>

          {/* Pump */}
          <g className={on("pump", step)} onClick={() => setStep("pump")}>
            <circle className="iod-pump" cx="363" cy="372" r="30" />
            <circle className="iod-pump-hub" cx="363" cy="372" r="12" />
            <text className="iod-node-label" x="318" y="376" textAnchor="end">
              Oil pump
            </text>
          </g>

          {/* Filter — right of pan, left of spec cards */}
          <g className={on("filter", step)} onClick={() => setStep("filter")}>
            <rect className="iod-filter" x="618" y="318" width="48" height="90" rx="12" />
            <rect className="iod-filter-cap" x="612" y="308" width="60" height="16" rx="4" />
            <text className="iod-node-label" x="642" y="430" textAnchor="middle">
              Filter
            </text>
          </g>

          {/* Pan */}
          <g className={on("pan", step)} onClick={() => setStep("pan")}>
            <path
              className="iod-pan"
              d="M 188 438 H 508 Q 538 438 538 462 V 500 Q 538 524 508 524 H 188 Q 158 524 158 500 V 462 Q 158 438 188 438 Z"
            />
            <rect
              className="iod-sump"
              x="174"
              y="470"
              width="348"
              height="42"
              rx="12"
              fill={`url(#${uid}-oil)`}
            />
            <text className="iod-node-label iod-on-oil" x="348" y="496" textAnchor="middle">
              Oil pan · {spec.capacityWithFilterUsQt} qt with filter
            </text>
          </g>

          {/* Dipstick — left of the banks, below the title */}
          <g className={on("check", step)} onClick={() => setStep("check")}>
            <rect className="iod-stick-handle" x="46" y="168" width="32" height="20" rx="6" />
            <rect className="iod-stick" x="57" y="186" width="10" height="248" rx="3" />
            <text className="iod-node-label" x="62" y="158" textAnchor="middle">
              Dipstick
            </text>
            <g transform="translate(8 214)">
              <rect className="iod-gauge" x="0" y="0" width="52" height="124" rx="6" />
              <text className="iod-gauge-mark" x="26" y="26" textAnchor="middle">
                F
              </text>
              <rect
                className="iod-sump"
                x="13"
                y="38"
                width="26"
                height="50"
                rx="3"
                fill={`url(#${uid}-oil)`}
              />
              <text className="iod-gauge-mark" x="26" y="110" textAnchor="middle">
                L
              </text>
            </g>
            <text className="iod-call-mini" x="10" y="358">
              L→F {spec.lowToFullUsQt} qt
            </text>
          </g>

          {/* Filler sits on the right bank, clear of spec cards */}
          <g className={on("check", step)} onClick={() => setStep("check")}>
            <rect className="iod-neck" x="540" y="148" width="16" height="40" rx="4" />
            <circle className="iod-filler" cx="548" cy="140" r="14" />
            <text className="iod-node-label" x="548" y="128" textAnchor="middle">
              Fill
            </text>
          </g>

          {/* Flow paths */}
          <g className="iod-flows" filter={`url(#${uid}-glow)`}>
            <path
              className="iod-flow"
              d="M 348 470 V 402"
              markerEnd={`url(#${uid}-arrow)`}
            />
            <path
              className="iod-flow"
              d="M 393 372 H 618"
              markerEnd={`url(#${uid}-arrow)`}
            />
            <path
              className="iod-flow"
              d="M 642 308 V 266 H 500"
              markerEnd={`url(#${uid}-arrow)`}
            />
            <path
              className="iod-flow iod-return"
              d="M 248 266 H 196 V 448"
              markerEnd={`url(#${uid}-arrow)`}
            />
            <path className="iod-flow iod-return" d="M 500 266 H 642 V 308" />
            <path
              className="iod-flow iod-return"
              d="M 292 338 V 438"
              markerEnd={`url(#${uid}-arrow)`}
            />
            <path
              className="iod-flow iod-return"
              d="M 430 338 V 438"
              markerEnd={`url(#${uid}-arrow)`}
            />
          </g>

          {/* Spec stack */}
          <g className="iod-specs">
            <SpecCard
              x={745}
              y={145}
              kicker="Required oil"
              value={spec.requiredViscosity}
              detail={`${spec.requiredForm} · ${spec.requiredGrade}`}
            />
            <SpecCard
              x={745}
              y={253}
              kicker="Change + filter"
              value={`${spec.capacityWithFilterUsQt} US qt`}
              detail={`${spec.capacityWithFilterL} L · oil only ${spec.capacityOilOnlyUsQt} qt`}
            />
            <SpecCard
              x={745}
              y={361}
              kicker="Subaru schedule"
              value={`${SUBARU_SCHEDULE_MILES.toLocaleString()} mi`}
              detail={`or ${SUBARU_SCHEDULE_MONTHS} months · whichever first`}
            />
            <SpecCard
              x={745}
              y={469}
              kicker="Oil Desk due clock"
              value={`${FLEET_DEFAULT_INTERVAL_MILES.toLocaleString()} mi`}
              detail="Default when interval_miles is unset"
            />
          </g>

          <text className="iod-footnote" x="32" y="638">
            {spec.source}. Capacities are guidelines — confirm on the level gauge.
          </text>
        </svg>
      </div>

      <ol className="iod-steps" aria-label="Oil circuit">
        {OIL_CIRCUIT_STEPS.map((s) => (
          <li key={s.id}>
            <button
              type="button"
              className={`iod-step ${step === s.id ? "is-on" : ""}`}
              onClick={() => setStep(s.id)}
              aria-pressed={step === s.id}
            >
              <span className="iod-n">{s.n}</span>
              <span className="iod-step-copy">
                <strong>{s.title}</strong>
                <span>{s.body}</span>
              </span>
            </button>
          </li>
        ))}
      </ol>

      <p className="iod-live" aria-live="polite">
        <span className="field-label">Selected</span>
        <strong>
          {current.n}. {current.title}.
        </strong>{" "}
        {current.body}
      </p>

      <div className="iod-grid">
        <article className="iod-panel">
          <h2>How to check</h2>
          <ol>
            <li>Park on a level surface and stop the engine.</li>
            <li>Wait at least {spec.checkWaitMinutes} minutes so oil drains back to the pan.</li>
            <li>Pull the level gauge, wipe it, and seat it fully until it stops.</li>
            <li>Read both sides — the true level is the lower mark.</li>
            <li>
              If below L, add {spec.requiredViscosity} slowly through the filler. L to F is about{" "}
              {spec.lowToFullUsQt} US qt ({spec.lowToFullL} L). Do not go above F when cold.
            </li>
          </ol>
        </article>

        <article className="iod-panel">
          <h2>Between changes — oil is used</h2>
          <p>
            Some engine oil is consumed while driving. Under the conditions below, Subaru says to
            check at least every second fuel fill and change more often. Different drivers in the
            same car can see different results.
          </p>
          <ul className="iod-chips">
            {OIL_CONSUMPTION_DRIVERS.map((d) => (
              <li key={d}>{d}</li>
            ))}
          </ul>
        </article>

        <article className="iod-panel">
          <h2>What to pour</h2>
          <p>
            Preferred: Subaru-approved {spec.requiredViscosity} {spec.requiredForm},{" "}
            {spec.requiredGrade}. If that can is not on the shelf, {spec.altGrade} is the listed
            alternative. If {spec.requiredViscosity} is unavailable, {spec.replenishIfUnavailable}{" "}
            may be used to top off, then return to {spec.requiredViscosity} at the next change.
          </p>
          <p>
            US 2024 Impreza is the 2.0 L {spec.engine}. The same oil spec is printed for the 2.5 L
            FB25 if that engine appears on the vehicle. CVT fluid ({spec.engine} models 10.8 US qt)
            and front/rear differential gear oil are not engine oil.
          </p>
        </article>
      </div>
    </section>
  );
}

function on(id: OilCircuitStepId, step: OilCircuitStepId) {
  return `iod-hot ${step === id ? "is-on" : ""}`;
}

function Bank({ x, y, side }: { x: number; y: number; side: "left" | "right" }) {
  const label = side === "left" ? "LH 2 · 4" : "RH 1 · 3";
  const cx = x + 68;
  return (
    <g>
      <ellipse className="iod-cyl" cx={cx} cy={y + 28} rx="64" ry="26" />
      <ellipse className="iod-cyl" cx={cx} cy={y + 94} rx="64" ry="26" />
      <text className="iod-node-label iod-small" x={cx} y={y + 138} textAnchor="middle">
        {label}
      </text>
    </g>
  );
}

function SpecCard({
  x,
  y,
  kicker,
  value,
  detail,
}: {
  x: number;
  y: number;
  kicker: string;
  value: string;
  detail: string;
}) {
  return (
    <g transform={`translate(${x} ${y})`}>
      <rect className="iod-card" x="0" y="0" width="320" height="96" rx="8" />
      <text className="iod-kicker" x="18" y="28">
        {kicker}
      </text>
      <text className="iod-card-value" x="18" y="64">
        {value}
      </text>
      <text className="iod-sub" x="18" y="84">
        {detail}
      </text>
    </g>
  );
}
