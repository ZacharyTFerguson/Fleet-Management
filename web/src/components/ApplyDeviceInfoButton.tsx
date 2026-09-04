"use client";

import { useCallback, useEffect, useState, useTransition } from "react";

type VINFromFileResult = {
  path?: string;
  exists?: boolean;
  parsed?: number;
  upserted?: number;
  asked?: number;
  linked?: number;
  already?: number;
  no_vin?: number;
  no_roster?: number;
  skipped_existing_map?: number;
  error?: string;
  links?: { factory_id: string; vin: string; efleets_id: string }[];
};

export function ApplyDeviceInfoButton() {
  const [fileState, setFileState] = useState<VINFromFileResult | null>(null);
  const [result, setResult] = useState<VINFromFileResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  const probe = useCallback(() => {
    fetch("/api/devices/vin-from-file", { cache: "no-store" })
      .then((res) => res.json())
      .then((body: VINFromFileResult) => setFileState(body))
      .catch(() => {
        /* GET is optional; POST still shows an honest error */
      });
  }, []);

  useEffect(() => {
    probe();
  }, [probe]);

  const apply = () => {
    startTransition(async () => {
      setError(null);
      setResult(null);
      try {
        const res = await fetch("/api/devices/vin-from-file", { method: "POST" });
        const body = (await res.json()) as VINFromFileResult & { error?: string };
        if (!res.ok) {
          setError(body.error || res.statusText);
          return;
        }
        setResult(body);
        setFileState({ path: body.path, exists: true });
      } catch (e) {
        setError(e instanceof Error ? e.message : "Could not apply saved device information");
      }
    });
  };

  return (
    <div className="vin-file-apply">
      <button type="button" className={`cta ${pending ? "is-pending" : ""}`} onClick={apply} disabled={pending}>
        {pending ? "Applying saved file…" : "Apply saved OneStep device information"}
      </button>
      <p className="vin-file-hint">
        Pairs unpaired GPS boxes by exact 17-char VIN from the saved Device Information JSON (
        <code>{fileState?.path || "data/runtime/device-information.json"}</code>
        ). Does not GET live <code>/device</code>. display_name is never a join.
      </p>
      {fileState && fileState.exists === false ? (
        <p className="search-empty" role="status">
          No saved file at {fileState.path}. Drop the OneStep Device Information JSON there while the API is cooling
          down.
        </p>
      ) : null}
      {error ? (
        <p className="vin-file-error" role="alert">
          {error}
        </p>
      ) : null}
      {result && !error ? (
        <p className="vin-file-ok" role="status">
          Paired {result.linked ?? 0} boxes from the saved file ({result.parsed ?? 0} parsed, {result.already ?? 0}{" "}
          already mapped, {result.asked ?? 0} live GETs). Never Last Reading.
        </p>
      ) : null}
    </div>
  );
}
