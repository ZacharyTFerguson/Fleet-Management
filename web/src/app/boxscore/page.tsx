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
          Recorded mileage is the <strong>gas card transaction</strong> (WEX /
          Enterprise fuel punch odometer + Provider Transaction Time)—not a
          vague Enterprise reading. Expected is good maintenance odometer plus
          OneStep drive-stop miles since that stamp. Suspect/HOLD stay in their
          own bucket and do not move the trend until corrected or dismissed.
        </p>
      </header>
      <BoxScoreBoard />
    </main>
  );
}
