"use client";

import { useRouter } from "next/navigation";
import { PaymentLinkForm } from "@/components/payment-link-form";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ArrowLeft } from "lucide-react";
import Link from "next/link";

export default function NewPaymentLinkPage() {
  const router = useRouter();

  const handleSubmit = async (data: {
    title: string;
    description?: string;
    amount: string;
    max_uses?: number;
  }) => {
    await api.paymentLinks.create(data);
    router.push("/dashboard");
  };

  return (
    <div className="mx-auto max-w-lg space-y-4 p-4">
      <Link href="/dashboard">
        <Button variant="ghost" size="sm">
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Dashboard
        </Button>
      </Link>
      <PaymentLinkForm onSubmit={handleSubmit} submitLabel="Create Payment Link" />
    </div>
  );
}
