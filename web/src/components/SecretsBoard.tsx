"use client";

import { FormEvent, useEffect, useState } from "react";

type Field = {
  key: string;
  label: string;
  kind: string;
  hint?: string;
  set: boolean;
  bytes?: number;
  mask?: string;
  source?: string;
};

export function SecretsBoard() {
  const [fields, setFields] = useState<Field[]>([]);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [err, setErr] = useState("");
  const [note, setNote] = useState("");

  const load = () => {
    fetch("/api/secrets")
      .then(async (r) => {
        const j = await r.json();
        if (r.status === 401) {
          setErr("Sign in on Login to manage secrets.");
          return;
        }
        if (!r.ok) {
          setErr(j.error || "unavailable");
          return;
        }
        setErr("");
        setFields(j.fields || []);
        setNote(j.note || "");
      })
      .catch(() => setErr("unavailable"));
  };

  useEffect(() => {
    load();
  }, []);

  const save = async (e: FormEvent, key: string) => {
    e.preventDefault();
    const value = drafts[key] || "";
    const res = await fetch("/api/secrets", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key, value }),
    });
    const j = await res.json();
    if (!res.ok) {
      setErr(j.error || "save failed");
      return;
    }
    setDrafts((d) => ({ ...d, [key]: "" }));
    load();
  };

  return (
    <section className="roster" aria-label="Secrets vault">
      {err ? <p className="search-empty">{err}</p> : null}
      {note ? <p className="matchup-why">{note}</p> : null}
      <div className="secret-list">
        {fields.map((f) => (
          <form key={f.key} className="car-card secret-card" onSubmit={(e) => save(e, f.key)}>
            <div className="car-card-head">
              <h2>{f.label}</h2>
              <span className={"status-badge " + (f.set ? "ok" : "hold")}>
                <span className="status-label">vault</span>
                <span className="status-text">{f.set ? "set" : "empty"}</span>
              </span>
            </div>
            {f.set ? (
              <p className="mono">
                {f.mask} · {f.bytes} bytes · {f.source}
              </p>
            ) : (
              <p className="matchup-why">Not stored.</p>
            )}
            {f.hint ? <p className="matchup-why">{f.hint}</p> : null}
            {f.kind === "textarea" ? (
              <textarea
                className="secret-input secret-pem"
                spellCheck={false}
                autoComplete="off"
                placeholder="Paste PEM — it will not be shown again"
                value={drafts[f.key] || ""}
                onChange={(e) => setDrafts((d) => ({ ...d, [f.key]: e.target.value }))}
              />
            ) : (
              <input
                className="secret-input"
                type={f.kind === "password" ? "password" : "text"}
                autoComplete="off"
                placeholder={f.set ? "•••• replace value" : "enter value"}
                value={drafts[f.key] || ""}
                onChange={(e) => setDrafts((d) => ({ ...d, [f.key]: e.target.value }))}
              />
            )}
            <div className="roster-actions">
              <button className="cta" type="submit">
                Save
              </button>
            </div>
          </form>
        ))}
      </div>
    </section>
  );
}
