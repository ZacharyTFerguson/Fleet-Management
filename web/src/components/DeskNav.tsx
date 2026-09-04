"use client";

export function DeskNav({ current }: { current: "oil" | "cards" }) {
  return (
    <nav className="desk-nav" aria-label="Desk">
      <a className={current === "oil" ? "is-current" : ""} href="/">
        Oil
      </a>
      <a className={current === "cards" ? "is-current" : ""} href="/cards/">
        Cards
      </a>
    </nav>
  );
}
