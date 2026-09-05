"use client";

import { ApplyDeviceInfoButton } from "@/components/ApplyDeviceInfoButton";
import { DeskNav } from "@/components/DeskNav";

/**
 * Devices desk: apply a saved OneStep Device Information JSON when live /device is cooling down.
 * Join on factory_id (imei on the report). display_name is a label only.
 */
export default function DevicesPage() {
  return (
    <main className="shell">
      <div className="atmosphere" aria-hidden="true" />
      <div className="grain" aria-hidden="true" />
      <DeskNav current="devices" />
      <header className="hero">
        <p className="brand">FLEET</p>
        <h1 className="headline">Devices</h1>
        <p className="lede">
          GPS boxes keyed by factory_id. When OneStep is cooling down, save Device Information JSON
          to data/runtime/device-information.json and click the button — do not spam live /device.
        </p>
      </header>
      <section className="roster" aria-label="Saved Device Information">
        <ApplyDeviceInfoButton />
      </section>
    </main>
  );
}
