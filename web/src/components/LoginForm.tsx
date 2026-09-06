"use client";

import { FormEvent, useEffect, useState } from "react";

type Session = {
  ok: boolean;
  username?: string;
  bootstrap?: boolean;
  users?: number;
};

export function LoginForm() {
  const [session, setSession] = useState<Session | null>(null);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = () => {
    fetch("/api/auth/session")
      .then((r) => r.json())
      .then((j) => setSession(j))
      .catch(() => setSession({ ok: false }));
  };

  useEffect(() => {
    refresh();
  }, []);

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setMsg("");
    try {
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password }),
      });
      const j = await res.json();
      if (!res.ok) {
        setMsg(j.error || "login failed");
        return;
      }
      setPassword("");
      setMsg(j.bootstrap ? "Operator created. You are signed in." : "Signed in.");
      refresh();
    } finally {
      setBusy(false);
    }
  };

  const logout = async () => {
    await fetch("/api/auth/logout", { method: "POST" });
    setPassword("");
    refresh();
  };

  return (
    <section className="panel secret-panel" aria-label="Login">
      {session?.ok ? (
        <div>
          <p className="lede">
            Signed in as <strong>{session.username}</strong>.
          </p>
          <button className="cta" type="button" onClick={logout}>
            Sign out
          </button>
        </div>
      ) : (
        <form onSubmit={onSubmit} className="secret-form">
          <p className="matchup-why">
            {session?.bootstrap
              ? "No desk user yet — this sign-in creates the first operator."
              : "Use the desk username and password."}
          </p>
          <label className="field-label">
            Username
            <input
              className="secret-input"
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
            />
          </label>
          <label className="field-label">
            Password
            <input
              className="secret-input"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </label>
          <button className="cta" type="submit" disabled={busy}>
            {busy ? "Working…" : session?.bootstrap ? "Create and sign in" : "Sign in"}
          </button>
        </form>
      )}
      {msg ? <p className="matchup-why">{msg}</p> : null}
    </section>
  );
}
