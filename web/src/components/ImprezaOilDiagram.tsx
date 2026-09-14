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
          viewBox="0 0 1100 640"
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

          <rect className="iod-board" x="8" y="8" width="1084" height="624" rx="10" />

          <text className="iod-kicker" x="36" y="44">
            2024 SUBARU IMPREZA
          </text>
          <text className="iod-headline" x="36" y="82">
            Engine oil use
          </text>
          <text className="iod-sub" x="36" y="108">
            {spec.engine} · {spec.displacement} · DOHC DIT · firing {spec.firingOrder}
          </text>

          {/* Boxer block */}
          <g className={on("gallery", step)} onClick={() => setStep("gallery")}>
            <rect
              className="iod-block"
              x="250"
              y="168"
              width="240"
              height="150"
              rx="14"
              fill={`url(#${uid}-metal)`}
            />
            <rect className="iod-gallery" x="210" y="228" width="400" height="18" rx="8" />
            <text className="iod-node-label" x="370" y="204" textAnchor="middle">
              Crankcase
            </text>
            <text className="iod-node-label iod-small" x="370" y="242" textAnchor="middle">
              Main gallery
            </text>
          </g>

          <g className={on("banks", step)} onClick={() => setStep("banks")}>
            <Bank x={78} y={176} side="left" />
            <Bank x={498} y={176} side="right" />
          </g>

          {/* Pump */}
          <g className={on("pump", step)} onClick={() => setStep("pump")}>
            <circle className="iod-pump" cx="370" cy="348" r="32" />
            <circle className="iod-pump-hub" cx="370" cy="348" r="12" />
            <text className="iod-node-label" x="370" y="394" textAnchor="middle">
              Oil pump
            </text>
          </g>

          {/* Filter */}
          <g className={on("filter", step)} onClick={() => setStep("filter")}>
            <rect className="iod-filter" x="528" y="300" width="52" height="98" rx="12" />
            <rect className="iod-filter-cap" x="522" y="292" width="64" height="16" rx="4" />
            <text className="iod-node-label" x="554" y="428" textAnchor="middle">
              Filter
            </text>
          </g>

          {/* Pan */}
          <g className={on("pan", step)} onClick={() => setStep("pan")}>
            <path
              className="iod-pan"
              d="M 190 412 H 530 Q 560 412 560 438 V 478 Q 560 502 530 502 H 190 Q 160 502 160 478 V 438 Q 160 412 190 412 Z"
            />
            <rect
              className="iod-sump"
              x="176"
              y="448"
              width="368"
              height="44"
              rx="12"
              fill={`url(#${uid}-oil)`}
            />
            <text className="iod-node-label iod-on-oil" x="360" y="476" textAnchor="middle">
              Oil pan · {spec.capacityWithFilterUsQt} qt with filter
            </text>
          </g>

          {/* Dipstick */}
          <g className={on("check", step)} onClick={() => setStep("check")}>
            <rect className="iod-stick" x="128" y="150" width="10" height="268" rx="3" />
            <rect className="iod-stick-handle" x="116" y="132" width="34" height="22" rx="6" />
            <text className="iod-node-label" x="133" y="118" textAnchor="middle">
              Dipstick
            </text>
            <g transform="translate(36 200)">
              <rect className="iod-gauge" x="0" y="0" width="56" height="130" rx="6" />
              <text className="iod-gauge-mark" x="28" y="28" textAnchor="middle">
                F
              </text>
              <rect
                className="iod-sump"
                x="14"
                y="40"
                width="28"
                height="52"
                rx="3"
                fill={`url(#${uid}-oil)`}
              />
              <text className="iod-gauge-mark" x="28" y="114" textAnchor="middle">
                L
              </text>
            </g>
            <text className="iod-call-mini" x="28" y="360">
              L→F {spec.lowToFullUsQt} qt
            </text>
          </g>

          {/* Filler */}
          <g className={on("check", step)} onClick={() => setStep("check")}>
            <rect className="iod-neck" x="612" y="128" width="18" height="48" rx="4" />
            <circle className="iod-filler" cx="621" cy="118" r="16" />
            <text className="iod-node-label" x="662" y="124">
              Fill cap
            </text>
            <text className="iod-call-mini" x="662" y="146">
              {spec.requiredViscosity} only
            </text>
          </g>

          {/* Flow paths */}
          <g className="iod-flows" filter={`url(#${uid}-glow)`}>
            <path
              className="iod-flow"
              d="M 360 448 V 380"
              markerEnd={`url(#${uid}-arrow)`}
            />
            <path
              className="iod-flow"
              d="M 400 348 H 522"
              markerEnd={`url(#${uid}-arrow)`}
            />
            <path
              className="iod-flow"
              d="M 554 300 V 246 H 500"
              markerEnd={`url(#${uid}-arrow)`}
            />
            <path
              className="iod-flow iod-return"
              d="M 250 246 H 210 V 430"
              markerEnd={`url(#${uid}-arrow)`}
            />
            <path
              className="iod-flow iod-return"
              d="M 490 246 H 554 V 292"
            />
            <path
              className="iod-flow iod-return"
              d="M 290 318 V 412"
              markerEnd={`url(#${uid}-arrow)`}
            />
            <path
              className="iod-flow iod-return"
              d="M 430 318 V 412"
              markerEnd={`url(#${uid}-arrow)`}
            />
          </g>

          {/* Spec stack */}
          <g className="iod-specs">
            <SpecCard
              x={720}
              y={132}
              kicker="Required oil"
              value={spec.requiredViscosity}
              detail={`${spec.requiredForm} · ${spec.requiredGrade}`}
            />
            <SpecCard
              x={720}
              y={248}
              kicker="Change + filter"
              value={`${spec.capacityWithFilterUsQt} US qt`}
              detail={`${spec.capacityWithFilterL} L · oil only ${spec.capacityOilOnlyUsQt} qt`}
            />
            <SpecCard
              x={720}
              y={364}
              kicker="Subaru schedule"
              value={`${SUBARU_SCHEDULE_MILES.toLocaleString()} mi`}
              detail={`or ${SUBARU_SCHEDULE_MONTHS} months · whichever first`}
            />
            <SpecCard
              x={720}
              y={480}
              kicker="Oil Desk due clock"
              value={`${FLEET_DEFAULT_INTERVAL_MILES.toLocaleString()} mi`}
              detail="Default when interval_miles is unset"
            />
          </g>

          <text className="iod-footnote" x="36" y="612">
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
  return (
    <g>
      <ellipse className="iod-cyl" cx={x + 70} cy={y + 28} rx="72" ry="28" />
      <ellipse className="iod-cyl" cx={x + 70} cy={y + 96} rx="72" ry="28" />
      <text className="iod-node-label iod-small" x={x + 70} y={y + 148} textAnchor="middle">
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
      <rect className="iod-card" x="0" y="0" width="340" height="100" rx="8" />
      <text className="iod-kicker" x="20" y="32">
        {kicker}
      </text>
      <text className="iod-card-value" x="20" y="68">
        {value}
      </text>
      <text className="iod-sub" x="20" y="88">
        {detail}
      </text>
    </g>
  );
}
