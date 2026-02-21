"use client";

import { useAuth } from "@/providers/auth-provider";
import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { ConnectWalletButton } from "@/components/connect-wallet-button";
import { Separator } from "@/components/ui/separator";
import { Settings } from "lucide-react";
import Link from "next/link";

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const { isAuthenticated, isLoading, user } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (isLoading) return;
    if (!isAuthenticated) {
      router.push("/auth");
      return;
    }
    if (!user?.role) {
      router.push("/onboarding");
    }
  }, [isAuthenticated, isLoading, user, router]);

  if (isLoading || !isAuthenticated) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <p className="text-muted-foreground">Loading...</p>
      </div>
    );
  }

  return (
    <div className="min-h-screen">
      <header className="border-b">
        <div className="container flex h-16 items-center justify-between px-4">
          <h1 className="text-xl font-bold">
            <Link href="/dashboard">CPay</Link>
          </h1>
          <div className="flex items-center gap-2">
            <Link
              href="/dashboard/settings"
              className="inline-flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:text-foreground"
            >
              <Settings className="h-5 w-5" />
            </Link>
            <ConnectWalletButton />
          </div>
        </div>
      </header>
      <Separator />
      <main className="container px-4 py-8">{children}</main>
    </div>
  );
}
