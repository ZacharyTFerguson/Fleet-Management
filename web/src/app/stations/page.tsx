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
          Proven download is Gas_Stations only (<code>/zone-group</code> then{" "}
          <code>/zone</code>). Review → third-party map → dry-run. API create is
          not proven — portal-first until a live write is confirmed. No shops. No
          bulk.
        </p>
      </header>
      <StationsBoard />
    </main>
  );
}
