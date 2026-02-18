"use client";

import { useEffect, useState, use } from "react";
import { useRouter } from "next/navigation";
import { ProductForm } from "@/components/product-form";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import type { Product } from "@/lib/api";

export default function EditProductPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const [product, setProduct] = useState<Product | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.products
      .getByID(id)
      .then((p) => setProduct(p))
      .catch(() => router.push("/dashboard"))
      .finally(() => setLoading(false));
  }, [id, router]);

  const handleSubmit = async (data: {
    name: string;
    description?: string;
    price: string;
    currency?: string;
    image_url?: string;
  }) => {
    await api.products.update(id, data);
    router.push("/dashboard");
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <p className="text-muted-foreground">Loading product...</p>
      </div>
    );
  }

  if (!product) {
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
      <ProductForm
        initialData={product}
        onSubmit={handleSubmit}
        submitLabel="Update Product"
      />
    </div>
  );
}
