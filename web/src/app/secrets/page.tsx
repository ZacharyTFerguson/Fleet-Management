import { DeskNav } from "@/components/DeskNav";
import { SecretsBoard } from "@/components/SecretsBoard";

export default function SecretsPage() {
  return (
    <main className="shell">
      <div className="atmosphere" aria-hidden="true" />
      <div className="grain" aria-hidden="true" />
      <DeskNav current="secrets" />
      <header className="hero">
        <p className="brand">FLEET</p>
        <h1 className="headline">Secrets</h1>
        <p className="lede">
          Server-side encrypted vault. After save you only see a mask and byte
          count — never the full PEM, password, or key.
        </p>
      </header>
      <SecretsBoard />
    </main>
  );
}
