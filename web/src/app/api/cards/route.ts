import { NextResponse } from "next/server";
import { loadCardsSnapshot } from "@/lib/cards";

export const dynamic = "force-dynamic";

export async function GET() {
  try {
    const snap = await loadCardsSnapshot();
    return NextResponse.json(snap, {
      headers: { "Cache-Control": "no-store" },
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : "cards load failed";
    return NextResponse.json({ error: message }, { status: 500 });
  }
}
