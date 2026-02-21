import { NextRequest, NextResponse } from "next/server";

const API_URL = process.env.INTERNAL_API_URL || "http://localhost:8080";

export async function PUT(req: NextRequest) {
  const body = await req.text();
  const res = await fetch(`${API_URL}/api/user/email`, {
    method: "PUT",
    headers: {
      Authorization: req.headers.get("Authorization") || "",
      "Content-Type": "application/json",
    },
    body,
  });
  const data = await res.json();
  return NextResponse.json(data, { status: res.status });
}
