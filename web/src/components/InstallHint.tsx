"use client";

import { useEffect, useState } from "react";

const KEY = "fleet-install-hint";

export function InstallHint() {
  const [show, setShow] = useState(false);

  useEffect(() => {
    try {
      if (window.localStorage.getItem(KEY) === "0") return;
      const standalone = window.matchMedia("(display-mode: standalone)").matches || (navigator as Navigator & { standalone?: boolean }).standalone === true;
      if (standalone) return;
      setShow(true);
    } catch {
      /* ignore */
    }
  }, []);

  if (!show) return null;

  return (
    <aside className="install-hint" role="note">
      <p>
        <strong>Make this an app.</strong> On iPhone: Share → Add to Home Screen. On
        the computer: <code>oilchange desk</code> opens a window with no browser
        chrome.
      </p>
      <button
        type="button"
        className="cta ghost"
        onClick={() => {
          try {
            window.localStorage.setItem(KEY, "0");
          } catch {
            /* ignore */
          }
          setShow(false);
        }}
      >
        Hide
      </button>
    </aside>
  );
}
