"use client";

import { useRouter } from "next/navigation";
import { SubscriptionForm } from "@/components/subscription-form";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ArrowLeft } from "lucide-react";
import Link from "next/link";

export default function NewSubscriptionPage() {
  const router = useRouter();

  const handleSubmit = async (data: {
    title: string;
    description?: string;
    amount: string;
    token_address: string;
    chain_id: number;
    period: string;
  }) => {
    await api.subscriptions.create(data);
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
      <SubscriptionForm onSubmit={handleSubmit} />
    </div>
  );
}
