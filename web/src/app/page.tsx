import { CarsBoard } from "@/components/CarsBoard";

export default function Home() {
  return (
    <main className="shell">
      <div className="atmosphere" aria-hidden="true" />
      <div className="grain" aria-hidden="true" />

      <header className="hero">
        <p className="brand">FLEET</p>
        <h1 className="headline">Oil Desk</h1>
        <p className="lede">
          Cars on the roster, kept current as sync lands through the day.
        </p>
      </header>

      <CarsBoard />
    </main>
  );
}
