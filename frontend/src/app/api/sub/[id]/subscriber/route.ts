import { NextRequest, NextResponse } from "next/server";

const API_URL = process.env.INTERNAL_API_URL || "http://localhost:8080";

export async function GET(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const payer = req.nextUrl.searchParams.get("payer") ?? "";
  const res = await fetch(`${API_URL}/api/sub/${id}/subscriber?payer=${encodeURIComponent(payer)}`);
  const data = await res.json();
  return NextResponse.json(data, { status: res.status });
}
