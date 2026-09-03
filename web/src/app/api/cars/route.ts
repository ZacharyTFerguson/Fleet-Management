import { NextResponse } from "next/server";
import { loadFleetSnapshot } from "@/lib/fleet";

export const dynamic = "force-dynamic";

export async function GET() {
  try {
    const snap = await loadFleetSnapshot();
    return NextResponse.json(snap, {
      headers: {
        "Cache-Control": "no-store",
      },
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : "fleet load failed";
    return NextResponse.json({ error: message }, { status: 500 });
  }
}
