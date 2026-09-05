"use client";

export function DeskNav({ current }: { current: "oil" | "cards" | "history" | "devices" }) {
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
    </nav>
  );
}
