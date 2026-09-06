"use client";

export function DeskNav({
  current,
}: {
  current: "oil" | "cards" | "history" | "devices" | "stations" | "settings" | "secrets" | "login";
}) {
  return (
    <nav className="desk-nav" aria-label="Desk">
      <a className={current === "oil" ? "is-current" : ""} href="/">
        Oil
      </a>
      <a className={current === "cards" ? "is-current" : ""} href="/cards/">
        Cards
      </a>
      <a className={current === "history" ? "is-current" : ""} href="/history/">
        History
      </a>
      <a className={current === "devices" ? "is-current" : ""} href="/devices/">
        Devices
      </a>
      <a className={current === "stations" ? "is-current" : ""} href="/stations/">
        Stations
      </a>
      <a className={current === "settings" ? "is-current" : ""} href="/settings/">
        Status
      </a>
      <a className={current === "secrets" ? "is-current" : ""} href="/secrets/">
        Secrets
      </a>
      <a className={current === "login" ? "is-current" : ""} href="/login/">
        Login
      </a>
    </nav>
  );
}
