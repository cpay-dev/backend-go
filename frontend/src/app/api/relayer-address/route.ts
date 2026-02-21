import { NextResponse } from "next/server";

const API_URL = process.env.INTERNAL_API_URL || "http://localhost:8080";

export async function GET() {
  const res = await fetch(`${API_URL}/api/relayer-address`);
  const data = await res.json();
  return NextResponse.json(data, { status: res.status });
}
