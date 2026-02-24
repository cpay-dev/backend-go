"use client";

import { useEffect, useState, use } from "react";
import { useRouter } from "next/navigation";
import { InvoiceForm } from "@/components/invoice-form";
import type { InvoiceFormData } from "@/components/invoice-form";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import type { Invoice } from "@/lib/api";

export default function EditInvoicePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const [invoice, setInvoice] = useState<Invoice | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.invoices
      .getByID(id)
      .then((inv) => setInvoice(inv))
      .catch(() => router.push("/dashboard"))
      .finally(() => setLoading(false));
  }, [id, router]);

  const handleSubmit = async (data: InvoiceFormData) => {
    await api.invoices.update(id, data);
    router.push("/dashboard");
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <p className="text-muted-foreground">Loading invoice...</p>
      </div>
    );
  }

  if (!invoice) {
    return null;
  }

  return (
    <div className="mx-auto max-w-lg space-y-4">
      <Link href="/dashboard">
        <Button variant="ghost" size="sm">
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Dashboard
        </Button>
      </Link>
      <InvoiceForm
        initialData={invoice}
        onSubmit={handleSubmit}
        submitLabel="Update Invoice"
      />
    </div>
  );
}
