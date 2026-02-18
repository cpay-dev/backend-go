"use client";

import { useRouter } from "next/navigation";
import { ProductForm } from "@/components/product-form";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ArrowLeft } from "lucide-react";
import Link from "next/link";

export default function NewProductPage() {
  const router = useRouter();

  const handleSubmit = async (data: {
    name: string;
    description?: string;
    price: string;
    currency?: string;
    image_url?: string;
  }) => {
    await api.products.create(data);
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
      <ProductForm onSubmit={handleSubmit} submitLabel="Create Product" />
    </div>
  );
}
