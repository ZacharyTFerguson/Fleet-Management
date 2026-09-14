"use client";

/* Static export + oilchange serve: plain <a> keeps trailing-slash routes working without a Next router. */
/* eslint-disable @next/next/no-html-link-for-pages */

export function DeskNav({
  current,
}: {
  current:
    | "oil"
    | "impreza"
    | "cards"
    | "history"
    | "devices"
    | "stations"
    | "boxscore"
    | "settings"
    | "secrets"
    | "login";
}) {
  return (
    <nav className="desk-nav" aria-label="Desk">
      <a className={current === "oil" ? "is-current" : ""} href="/">
        Oil
      </a>
      <a className={current === "impreza" ? "is-current" : ""} href="/impreza/">
        Impreza
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
      <a className={current === "boxscore" ? "is-current" : ""} href="/boxscore/">
        Box score
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
