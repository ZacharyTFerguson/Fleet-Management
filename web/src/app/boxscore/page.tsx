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
          Recorded is the gas card transaction odometer at punch time. Expected
          is the last good maintenance odometer plus OneStep drive-stop miles
          since that stamp. Never invent miles. Never OneStep odometer. Suspect
          and HOLD stay visible and out of the trend until corrected or
          dismissed.
        </p>
      </header>
      <BoxScoreBoard />
    </main>
  );
}
