"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/providers/auth-provider";
import { RoleSelector } from "@/components/role-selector";
import { ShopSetupForm } from "@/components/shop-setup-form";
import { api } from "@/lib/api";
import { useEffect } from "react";

export default function OnboardingPage() {
  const { user, isAuthenticated, isLoading, updateAuth } = useAuth();
  const router = useRouter();
  const [step, setStep] = useState<"role" | "shop">("role");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const didRedirect = useRef(false);

  useEffect(() => {
    if (isLoading || didRedirect.current) return;
    if (!isAuthenticated) {
      didRedirect.current = true;
      router.push("/auth");
      return;
    }
    if (user?.role) {
      didRedirect.current = true;
      router.push("/dashboard");
    }
    // Only run on initial load — not when user/token updates during onboarding
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isLoading]);

  const handleRoleSelect = async (role: "merchant" | "user") => {
    setIsSubmitting(true);
    try {
      const { token, user: updated } = await api.user.setRole(role) as { token: string; user: { id: string; wallet_address: string; role: string | null; created_at: string; updated_at: string } };

      updateAuth(token, updated);

      if (role === "merchant") {
        setStep("shop");
      } else {
        router.push("/dashboard");
      }
    } catch (err) {
      console.error("Failed to set role:", err);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleShopSubmit = async (data: {
    name: string;
    website?: string;
    title?: string;
    avatar_url?: string;
  }) => {
    setIsSubmitting(true);
    try {
      await api.shops.create(data);
      router.push("/dashboard");
    } catch (err) {
      console.error("Failed to create shop:", err);
    } finally {
      setIsSubmitting(false);
    }
  };

  if (isLoading) {
    return (
      <main className="flex min-h-screen items-center justify-center">
        <p className="text-muted-foreground">Loading...</p>
      </main>
    );
  }

  return (
    <main className="flex min-h-screen flex-col items-center justify-center p-8">
      <div className="w-full max-w-2xl space-y-8">
        <div className="text-center">
          <h1 className="text-3xl font-bold">
            {step === "role" ? "Choose Your Role" : "Set Up Your Shop"}
          </h1>
          <p className="mt-2 text-muted-foreground">
            {step === "role"
              ? "How will you use CPay?"
              : "Tell us about your business"}
          </p>
        </div>

        {step === "role" ? (
          <RoleSelector onSelect={handleRoleSelect} isLoading={isSubmitting} />
        ) : (
          <ShopSetupForm onSubmit={handleShopSubmit} isLoading={isSubmitting} />
        )}
      </div>
    </main>
  );
}
