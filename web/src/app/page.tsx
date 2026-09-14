import { CarsBoard } from "@/components/CarsBoard";
import { DeskNav } from "@/components/DeskNav";

export default function Home() {
  return (
    <main className="shell">
      <div className="atmosphere" aria-hidden="true" />
      <div className="grain" aria-hidden="true" />
      <DeskNav current="oil" />

      <header className="hero">
        <p className="brand">FLEET</p>
        <h1 className="headline">Oil Desk</h1>
        <p className="lede">
          Cars on the roster, kept current as sync lands through the day.{" "}
          <a className="lede-link" href="/impreza/">
            2024 Impreza oil change by use
          </a>
        </p>
      </header>

      <CarsBoard />
    </main>
  );
}
