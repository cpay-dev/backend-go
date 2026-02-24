"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Plus, Trash2 } from "lucide-react";
import type { Invoice } from "@/lib/api";

interface LineItem {
  description: string;
  quantity: number;
  unit_price: string;
  amount: string;
}

export interface InvoiceFormData {
  title: string;
  memo?: string;
  amount: string;
  recipient_email?: string;
  due_date?: string;
  line_items?: LineItem[];
  notes?: string;
}

interface InvoiceFormProps {
  initialData?: Invoice;
  onSubmit: (data: InvoiceFormData) => Promise<void>;
  submitLabel: string;
}

export function InvoiceForm({ initialData, onSubmit, submitLabel }: InvoiceFormProps) {
  const [title, setTitle] = useState(initialData?.title ?? "");
  const [memo, setMemo] = useState(initialData?.memo ?? "");
  const [amount, setAmount] = useState(initialData?.amount ?? "");
  const [recipientEmail, setRecipientEmail] = useState(initialData?.recipient_email ?? "");
  const [dueDate, setDueDate] = useState(initialData?.due_date ? initialData.due_date.slice(0, 10) : "");
  const [notes, setNotes] = useState(initialData?.notes ?? "");
  const [lineItems, setLineItems] = useState<LineItem[]>(
    initialData?.line_items ?? []
  );
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState("");

  const addLineItem = () => {
    setLineItems([...lineItems, { description: "", quantity: 1, unit_price: "", amount: "" }]);
  };

  const removeLineItem = (index: number) => {
    setLineItems(lineItems.filter((_, i) => i !== index));
  };

  const updateLineItem = (index: number, field: keyof LineItem, value: string | number) => {
    const updated = [...lineItems];
    updated[index] = { ...updated[index], [field]: value };
    // Auto-calculate amount
    const qty = typeof updated[index].quantity === "number" ? updated[index].quantity : parseFloat(String(updated[index].quantity)) || 0;
    const price = parseFloat(updated[index].unit_price) || 0;
    updated[index].amount = (qty * price).toFixed(2);
    setLineItems(updated);

    // Auto-sum total
    const total = updated.reduce((sum, item) => sum + (parseFloat(item.amount) || 0), 0);
    if (total > 0) {
      setAmount(total.toFixed(2));
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");

    if (!title.trim()) {
      setError("Title is required");
      return;
    }
    if (!amount.trim() || isNaN(Number(amount)) || Number(amount) <= 0) {
      setError("Amount must be a positive number");
      return;
    }

    setIsSubmitting(true);
    try {
      await onSubmit({
        title: title.trim(),
        memo: memo.trim() || undefined,
        amount: amount.trim(),
        recipient_email: recipientEmail.trim() || undefined,
        due_date: dueDate ? new Date(dueDate + "T00:00:00Z").toISOString() : undefined,
        line_items: lineItems.length > 0 ? lineItems : undefined,
        notes: notes.trim() || undefined,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Something went wrong");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{initialData ? "Edit Invoice" : "New Invoice"}</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="title">Title *</Label>
            <Input
              id="title"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Invoice title"
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="memo">Memo</Label>
            <textarea
              id="memo"
              value={memo}
              onChange={(e) => setMemo(e.target.value)}
              placeholder="Optional memo"
              rows={2}
              className="flex min-h-[60px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="recipient_email">Recipient Email</Label>
              <Input
                id="recipient_email"
                type="email"
                value={recipientEmail}
                onChange={(e) => setRecipientEmail(e.target.value)}
                placeholder="client@example.com"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="due_date">Due Date</Label>
              <Input
                id="due_date"
                type="date"
                value={dueDate}
                onChange={(e) => setDueDate(e.target.value)}
              />
            </div>
          </div>

          {/* Line Items */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>Line Items</Label>
              <Button type="button" variant="outline" size="sm" onClick={addLineItem}>
                <Plus className="mr-1 h-3 w-3" /> Add Item
              </Button>
            </div>
            {lineItems.map((item, index) => (
              <div key={index} className="flex items-end gap-2">
                <div className="flex-1 space-y-1">
                  {index === 0 && <Label className="text-xs">Description</Label>}
                  <Input
                    value={item.description}
                    onChange={(e) => updateLineItem(index, "description", e.target.value)}
                    placeholder="Item description"
                  />
                </div>
                <div className="w-16 space-y-1">
                  {index === 0 && <Label className="text-xs">Qty</Label>}
                  <Input
                    type="number"
                    min="1"
                    value={item.quantity}
                    onChange={(e) => updateLineItem(index, "quantity", parseInt(e.target.value) || 1)}
                  />
                </div>
                <div className="w-24 space-y-1">
                  {index === 0 && <Label className="text-xs">Price</Label>}
                  <Input
                    value={item.unit_price}
                    onChange={(e) => updateLineItem(index, "unit_price", e.target.value)}
                    placeholder="0.00"
                    inputMode="decimal"
                  />
                </div>
                <div className="w-24 space-y-1">
                  {index === 0 && <Label className="text-xs">Amount</Label>}
                  <Input value={item.amount} readOnly className="bg-muted" />
                </div>
                <Button type="button" variant="ghost" size="icon" onClick={() => removeLineItem(index)}>
                  <Trash2 className="h-4 w-4 text-destructive" />
                </Button>
              </div>
            ))}
          </div>

          <div className="space-y-2">
            <Label htmlFor="amount">Total Amount (USDC) *</Label>
            <Input
              id="amount"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              placeholder="0.00"
              inputMode="decimal"
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="notes">Notes</Label>
            <textarea
              id="notes"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="Payment terms, instructions, etc."
              rows={2}
              className="flex min-h-[60px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button type="submit" disabled={isSubmitting} className="w-full">
            {isSubmitting ? "Saving..." : submitLabel}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
