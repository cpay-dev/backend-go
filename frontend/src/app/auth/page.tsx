"use client";

import { ConnectWalletButton } from "@/components/connect-wallet-button";
import { useAuth } from "@/providers/auth-provider";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

export default function AuthPage() {
  const { isAuthenticated, user, isLoading } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (isLoading) return;
    if (isAuthenticated) {
      if (!user?.role) {
        router.push("/onboarding");
      } else {
        router.push("/dashboard");
      }
    }
  }, [isAuthenticated, user, isLoading, router]);

  return (
    <main className="flex min-h-screen flex-col items-center justify-center p-8">
      <div className="max-w-md text-center space-y-8">
        <h1 className="text-3xl font-bold">Welcome to CPay</h1>
        <p className="text-muted-foreground">
          Connect your wallet to sign in. We use Sign-In with Ethereum for
          secure, passwordless authentication.
        </p>
        <div className="flex justify-center">
          <ConnectWalletButton />
        </div>
        <p className="text-xs text-muted-foreground">
          Polygon network
        </p>
      </div>
    </main>
  );
}
