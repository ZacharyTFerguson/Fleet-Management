import { DeskNav } from "@/components/DeskNav";
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
          How oil moves through the 2.0 L FB20 BOXER, what to pour, how much,
          and when it is consumed between changes. Specs from the 2024
          Subaru Impreza Owner’s Manual — not Last Reading.
        </p>
      </header>
      <ImprezaOilDiagram />
    </main>
  );
}
