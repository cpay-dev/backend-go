"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Copy, Pencil, Send, XCircle, Trash2 } from "lucide-react";
import Link from "next/link";
import type { Invoice } from "@/lib/api";
import { EmbedButton, EmbedPanel } from "@/components/embed-snippet";

const STATUS_BADGE: Record<string, { variant: "default" | "secondary" | "outline" | "destructive"; label: string }> = {
  draft: { variant: "outline", label: "Draft" },
  sent: { variant: "default", label: "Sent" },
  paid: { variant: "default", label: "Paid" },
  overdue: { variant: "destructive", label: "Overdue" },
  cancelled: { variant: "secondary", label: "Cancelled" },
};

interface InvoiceCardProps {
  invoice: Invoice;
  onSend: (id: string) => void;
  onCancel: (id: string) => void;
  onDelete: (id: string) => void;
}

export function InvoiceCard({ invoice: inv, onSend, onCancel, onDelete }: InvoiceCardProps) {
  const [showEmbed, setShowEmbed] = useState(false);
  const publicUrl = typeof window !== "undefined"
    ? `${window.location.origin}/inv/${inv.id}`
    : `/inv/${inv.id}`;

  const badge = STATUS_BADGE[inv.status] ?? { variant: "outline" as const, label: inv.status };
  const isDraft = inv.status === "draft";
  const isSent = inv.status === "sent";
  const faded = inv.status === "cancelled" || inv.status === "draft";

  const invoiceNum = `INV-${String(inv.invoice_number).padStart(3, "0")}`;

  return (
    <Card className={faded ? "opacity-60" : ""}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <p className="text-xs font-mono text-muted-foreground">{invoiceNum}</p>
            <CardTitle className="truncate text-base">{inv.title}</CardTitle>
            {inv.memo && (
              <p className="mt-0.5 truncate text-sm text-muted-foreground">{inv.memo}</p>
            )}
          </div>
          <Badge variant={badge.variant} className={inv.status === "paid" ? "bg-green-600" : ""}>
            {badge.label}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xl font-bold">{inv.amount} USDC</p>
            {inv.due_date && (
              <p className="text-xs text-muted-foreground">
                Due {new Date(inv.due_date).toLocaleDateString()}
              </p>
            )}
          </div>
          <div className="flex items-center gap-1">
            {isSent && (
              <Button
                variant="ghost"
                size="icon"
                title="Copy invoice link"
                onClick={() => navigator.clipboard.writeText(publicUrl)}
              >
                <Copy className="h-4 w-4" />
              </Button>
            )}
            {isSent && (
              <EmbedButton onClick={() => setShowEmbed(!showEmbed)} />
            )}
            {isDraft && (
              <Button
                variant="ghost"
                size="icon"
                title="Send invoice"
                onClick={() => onSend(inv.id)}
              >
                <Send className="h-4 w-4" />
              </Button>
            )}
            {isSent && (
              <Button
                variant="ghost"
                size="icon"
                title="Cancel invoice"
                onClick={() => {
                  if (confirm("Cancel this invoice?")) onCancel(inv.id);
                }}
              >
                <XCircle className="h-4 w-4" />
              </Button>
            )}
            {isDraft && (
              <Link href={`/dashboard/invoices/${inv.id}/edit`}>
                <Button variant="ghost" size="icon" title="Edit">
                  <Pencil className="h-4 w-4" />
                </Button>
              </Link>
            )}
            {isDraft && (
              <Button
                variant="ghost"
                size="icon"
                title="Delete"
                onClick={() => {
                  if (confirm("Delete this invoice?")) onDelete(inv.id);
                }}
              >
                <Trash2 className="h-4 w-4 text-destructive" />
              </Button>
            )}
          </div>
        </div>
        {showEmbed && <EmbedPanel type="invoice" id={inv.id} />}
      </CardContent>
    </Card>
  );
}
