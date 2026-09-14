import { DeskNav } from "@/components/DeskNav";
import { ImprezaOilChangeByUse } from "@/components/ImprezaOilChangeByUse";
import { ImprezaOilDiagram } from "@/components/ImprezaOilDiagram";

export default function ImprezaOilPage() {
  return (
    <main className="shell iod-shell">
      <div className="atmosphere" aria-hidden="true" />
      <div className="grain" aria-hidden="true" />
      <DeskNav current="impreza" />
      <header className="hero">
        <p className="brand">FLEET</p>
        <h1 className="headline">2024 Impreza oil</h1>
        <p className="lede">
          Oil change clock from how the car is used — not Last Reading. Highway
          mixed driving follows Subaru’s 6,000 miles / 6 months. This fleet
          clocks 5,000. Severe use changes sooner and checks every second fuel
          fill. Same pour every time: 0W-16 and a new filter.
        </p>
      </header>
      <ImprezaOilChangeByUse />
      <h2 className="iod-section-title">How oil is used in the engine</h2>
      <ImprezaOilDiagram />
    </main>
  );
}

