"use client";

import { useEffect } from "react";
import { Button } from "@/components/ui/button";
import { useRouter } from "next/navigation";
import { useAuth } from "@/providers/auth-provider";

export default function Home() {
  const router = useRouter();
  const { isAuthenticated, isLoading, user } = useAuth();

  useEffect(() => {
    if (isLoading) return;
    if (isAuthenticated) {
      router.replace(user?.role ? "/dashboard" : "/onboarding");
    }
  }, [isLoading, isAuthenticated, user, router]);

  const handleGetStarted = () => {
    if (isAuthenticated) {
      if (!user?.role) {
        router.push("/onboarding");
      } else {
        router.push("/dashboard");
      }
    } else {
      router.push("/auth");
    }
  };

  return (
    <main className="flex min-h-screen flex-col items-center justify-center p-8">
      <div className="max-w-2xl text-center space-y-8">
        <h1 className="text-5xl font-bold tracking-tight">
          CPay
        </h1>
        <p className="text-xl text-muted-foreground">
          Accept crypto payments on Polygon. Create invoices, payment links, and
          recurring subscriptions.
        </p>
        <Button size="lg" onClick={handleGetStarted}>
          Get Started
        </Button>
      </div>
    </main>
  );
}
