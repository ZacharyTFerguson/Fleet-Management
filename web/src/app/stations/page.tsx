import { DeskNav } from "@/components/DeskNav";
import { StationsBoard } from "@/components/StationsBoard";

export default function StationsPage() {
  return (
    <main className="shell">
      <div className="atmosphere" aria-hidden="true" />
      <div className="grain" aria-hidden="true" />
      <DeskNav current="stations" />
      <header className="hero">
        <p className="brand">FLEET</p>
        <h1 className="headline">Gas Stations</h1>
        <p className="lede">
          Canon labels only (<code>GeneralCode_Type_Branding_TopTier_TopTierGrade</code>).
          Review → third-party map check → dry-run → one confirm-gated send.
          No shops. No bulk OneStep.
        </p>
      </header>
      <StationsBoard />
    </main>
  );
}
