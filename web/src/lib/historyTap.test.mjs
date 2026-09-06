import { historyChipTap } from "./historyTap.ts";
import assert from "node:assert/strict";

const assigned = { tx_key: "B", assigned_efleets_id: "27VA19" };
const unassigned = { tx_key: "C", assigned_efleets_id: "" };

assert.deepEqual(historyChipTap("", assigned), { kind: "toggle" });
assert.deepEqual(historyChipTap("A", assigned), {
  kind: "place",
  toEFleets: "27VA19",
  reason: "manual_drag",
});
assert.deepEqual(historyChipTap("A", unassigned), { kind: "hold", txKey: "C" });
assert.deepEqual(historyChipTap("C", unassigned), { kind: "toggle" });
assert.notEqual(historyChipTap("A", unassigned).kind, "place");

console.log("historyChipTap: ok");
