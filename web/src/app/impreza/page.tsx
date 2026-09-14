import { DeskNav } from "@/components/DeskNav";
import { ImprezaOilChangeByUse } from "@/components/ImprezaOilChangeByUse";
import { ImprezaOilDiagram } from "@/components/ImprezaOilDiagram";

export default function ImprezaOilPage() {
  return (
    <main className="shell iod-shell">
      <div className="atmosphere" aria-hidden="true" />
      <div className="grain" aria-hidden="true" />
      <DeskNav current="oil" />
      <header className="hero">
        <p className="brand">FLEET</p>
        <h1 className="headline">2024 Impreza oil</h1>
        <p className="lede">
          Warranty &amp; Maintenance Booklet Note 1: normal driving 6,000 miles or
          6 months (whichever first); this fleet clocks 5,000 miles; severe
          conditions 3,000 miles or 3 months. Same pour every time: 0W-16 and a
          new filter.
        </p>
      </header>
      <ImprezaOilChangeByUse />
      <h2 className="iod-section-title">How oil is used in the engine</h2>
      <ImprezaOilDiagram />
    </main>
  );
}

