import { CardsBoard } from "@/components/CardsBoard";
import { DeskNav } from "@/components/DeskNav";

export default function CardsPage() {
  return (
    <main className="shell">
      <div className="atmosphere" aria-hidden="true" />
      <div className="grain" aria-hidden="true" />
      <DeskNav current="cards" />
      <header className="hero">
        <p className="brand">FLEET</p>
        <h1 className="headline">Card Desk</h1>
        <p className="lede">
          GPS stop times at the marker, then the card that swiped in that
          window. Enterprise Vehicle is not the join.
        </p>
      </header>
      <CardsBoard />
    </main>
  );
}
