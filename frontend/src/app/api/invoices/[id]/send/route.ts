import { NextRequest, NextResponse } from "next/server";

const API_URL = process.env.INTERNAL_API_URL || "http://localhost:8080";

export async function POST(
  req: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id } = await params;
  const res = await fetch(`${API_URL}/api/invoices/${id}/send`, {
    method: "POST",
    headers: { Authorization: req.headers.get("Authorization") || "" },
  });
  const data = await res.json();
  return NextResponse.json(data, { status: res.status });
}
