import { DeskNav } from "@/components/DeskNav";
import { StatusBoard } from "@/components/StatusBoard";

export default function SettingsPage() {
  return (
    <main className="shell">
      <div className="atmosphere" aria-hidden="true" />
      <div className="grain" aria-hidden="true" />
      <DeskNav current="settings" />
      <header className="hero">
        <p className="brand">FLEET</p>
        <h1 className="headline">Connection status</h1>
        <p className="lede">
          SQLite is the working store. Neon is the backup. Supabase publishes
          Oil Desk <code>fleet_cars</code>. No passwords or DSNs are shown.
        </p>
      </header>
      <StatusBoard />
    </main>
  );
}
