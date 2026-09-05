import { DeskNav } from "@/components/DeskNav";
import { HistoryBoard } from "@/components/HistoryBoard";
import { InstallHint } from "@/components/InstallHint";

export default function HistoryPage() {
  return (
    <main className="shell history-shell">
      <div className="atmosphere" aria-hidden="true" />
      <div className="grain" aria-hidden="true" />
      <DeskNav current="history" />
      <InstallHint />
      <header className="hero">
        <p className="brand">FLEET</p>
        <h1 className="headline">History</h1>
        <p className="lede">
          One fill is one block. Drag it onto the car that used that card. The
          database files each fill under one car; the board turns by region.
        </p>
      </header>
      <HistoryBoard />
    </main>
  );
}
