"use client";

import { useRouter } from "next/navigation";
import { InvoiceForm } from "@/components/invoice-form";
import type { InvoiceFormData } from "@/components/invoice-form";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ArrowLeft } from "lucide-react";
import Link from "next/link";

export default function NewInvoicePage() {
  const router = useRouter();

  const handleSubmit = async (data: InvoiceFormData) => {
    await api.invoices.create(data);
    router.push("/dashboard");
  };

  return (
    <div className="mx-auto max-w-lg space-y-4">
      <Link href="/dashboard">
        <Button variant="ghost" size="sm">
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Dashboard
        </Button>
      </Link>
      <InvoiceForm onSubmit={handleSubmit} submitLabel="Create Invoice" />
    </div>
  );
}
