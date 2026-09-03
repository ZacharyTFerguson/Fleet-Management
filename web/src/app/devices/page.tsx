import type { OneStepDevice } from "@/types/onestep-devices";

/**
 * Minimal Devices list for merge with the cars UI sibling.
 * Route: /devices — does not touch car pages.
 */
const PLACEHOLDER: OneStepDevice[] = [];

export default function DevicesPage() {
  const devices = PLACEHOLDER;
  return (
    <main style={{ fontFamily: "Georgia, serif", padding: "2rem", maxWidth: 960, margin: "0 auto" }}>
      <h1>OneStep devices</h1>
      <p>Registry keyed by factory_id. display_name is label only (never a join).</p>
      <table style={{ width: "100%", borderCollapse: "collapse" }}>
        <thead>
          <tr>
            <th align="left">factory_id</th>
            <th align="left">device_id</th>
            <th align="left">efleets_id</th>
            <th align="left">status</th>
            <th align="left">display_name</th>
          </tr>
        </thead>
        <tbody>
          {devices.length === 0 ? (
            <tr>
              <td colSpan={5}>No devices loaded — run oilchange devices sync.</td>
            </tr>
          ) : (
            devices.map((d) => (
              <tr key={d.factory_id}>
                <td>{d.factory_id}</td>
                <td>{d.device_id}</td>
                <td>{d.linked_car_efleets_id ?? ""}</td>
                <td>{d.dead || !d.active ? "retired" : "active"}</td>
                <td>{d.display_name ?? ""}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </main>
  );
}
