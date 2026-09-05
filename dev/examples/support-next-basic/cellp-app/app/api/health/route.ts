export const dynamic = "force-dynamic";
export const runtime = "nodejs";

import { NextResponse } from "next/server";

export async function GET(request: Request) {
  const ts = new Date().toISOString();
  let pathname = "/api/health";
  try {
    pathname = new URL(request.url).pathname || pathname;
  } catch {
    /* celld absolute request.url */
  }
  return NextResponse.json({
    marker: "cellp-support-next-basic-v2",
    pathname,
    ts,
  });
}
