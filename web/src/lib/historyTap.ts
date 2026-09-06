export type HistoryChipTap =
  | { kind: "toggle" }
  | { kind: "place"; toEFleets: string; reason: string }
  | { kind: "hold"; txKey: string };

// historyChipTap is the History tap-to-place decision.
// Holding a fill and tapping another assigned chip places onto that car.
// Tapping an unassigned chip switches the hold — it must not persist undo.
// Undo is the Needs-a-home lane or the Undo button.
export function historyChipTap(
  held: string,
  block: { tx_key: string; assigned_efleets_id?: string },
): HistoryChipTap {
  if (!held || held === block.tx_key) {
    return { kind: "toggle" };
  }
  const dest = (block.assigned_efleets_id || "").trim();
  if (dest) {
    return { kind: "place", toEFleets: dest, reason: "manual_drag" };
  }
  return { kind: "hold", txKey: block.tx_key };
}
