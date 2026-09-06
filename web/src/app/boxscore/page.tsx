import { DeskNav } from "@/components/DeskNav";
import { BoxScoreBoard } from "@/components/BoxScoreBoard";

export default function BoxScorePage() {
  return (
    <main className="shell">
      <div className="atmosphere" aria-hidden="true" />
      <div className="grain" aria-hidden="true" />
      <DeskNav current="boxscore" />
      <header className="hero">
        <p className="brand">FLEET</p>
        <h1 className="headline">Mileage box score</h1>
        <p className="lede">
          When a <strong>gas card / fuel punch</strong> posts:{" "}
          <strong>expected</strong> is last good maintenance odometer + OneStep
          drive-stop miles since that stamp. <strong>Recorded</strong> is the
          punch odometer at Provider Transaction Time.{" "}
          <strong>Difference</strong> is recorded − expected (overage if the
          card is ahead; shortage if behind), plus |gap|. Trend is whether
          |gap| is growing (worse vs maint+GPS) or shrinking (improving). Last
          Reading on the Oil sheet is a different number — Enterprise last-good
          @ a known second + OneStep since that second. Never last oil +
          interval. HOLD when the maintenance base or OneStep pairing is
          missing.
        </p>
      </header>
      <BoxScoreBoard />
    </main>
  );
}
