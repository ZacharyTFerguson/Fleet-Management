"use client";

import { ApplyDeviceInfoButton } from "@/components/ApplyDeviceInfoButton";
import { DevicesGPS } from "@/components/DevicesGPS";
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
          Devices are the GPS. Cache first. Live checks are one fill or one box.
          When OneStep is cooling down, apply saved Device Information JSON — do not spam /device.
        </p>
      </header>
      <section className="roster" aria-label="Saved Device Information">
        <ApplyDeviceInfoButton />
      </section>
      <DevicesGPS />
    </main>
  );
}
