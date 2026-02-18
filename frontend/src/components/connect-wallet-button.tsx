"use client";

import { useAccount, useConnect, useDisconnect, useSignMessage } from "wagmi";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Wallet, LogOut, Copy } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useAuth } from "@/providers/auth-provider";
import { api } from "@/lib/api";

function formatAddress(address: string) {
  return `${address.slice(0, 6)}...${address.slice(-4)}`;
}

export function ConnectWalletButton() {
  const { address, isConnected } = useAccount();
  const { connectAsync, connectors } = useConnect();
  const { disconnect } = useDisconnect();
  const { signMessageAsync } = useSignMessage();
  const { signIn, signOut, isAuthenticated } = useAuth();
  const [mounted, setMounted] = useState(false);
  const [isAuthenticating, setIsAuthenticating] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  const hasInjectedProvider = useCallback(() => {
    if (typeof window === "undefined") return false;
    return Boolean((window as Window & { ethereum?: unknown }).ethereum);
  }, []);

  const handleConnect = useCallback(async () => {
    const injectedAvailable = hasInjectedProvider();
    const injectedConnector = connectors.find((c) => c.id === "injected");
    const walletConnectConnector = connectors.find((c) => c.id === "walletConnect");
    const connectorToUse = injectedAvailable
      ? (injectedConnector ?? walletConnectConnector)
      : (walletConnectConnector ?? injectedConnector);

    if (!connectorToUse) return;

    setIsConnecting(true);
    try {
      await connectAsync({ connector: connectorToUse });
    } catch (err) {
      console.error("Wallet connection failed:", err);
    } finally {
      setIsConnecting(false);
    }
  }, [connectAsync, connectors, hasInjectedProvider]);

  // SIWE auth after wallet connects
  useEffect(() => {
    if (!isConnected || !address || isAuthenticated) return;

    const authLockKey = `siwe-auth-in-flight:${address.toLowerCase()}`;
    if (sessionStorage.getItem(authLockKey) === "1") return;
    sessionStorage.setItem(authLockKey, "1");

    async function authenticate() {
      setIsAuthenticating(true);
      try {
        const { nonce } = await api.auth.nonce();

        const domain = window.location.host;
        const origin = window.location.origin;
        const message = `${domain} wants you to sign in with your Ethereum account:\n${address}\n\nSign in to CPay\n\nURI: ${origin}\nVersion: 1\nChain ID: 137\nNonce: ${nonce}\nIssued At: ${new Date().toISOString()}`;

        const signature = await signMessageAsync({ message });
        const { token, user } = await api.auth.verify(message, signature);

        signIn(token, user);
      } catch (err) {
        console.error("Authentication failed:", err);
        disconnect();
      } finally {
        sessionStorage.removeItem(authLockKey);
        setIsAuthenticating(false);
      }
    }

    authenticate();
  }, [isConnected, address, signMessageAsync, disconnect, isAuthenticated, signIn]);

  const handleDisconnect = () => {
    signOut();
    disconnect();
  };

  if (!mounted) {
    return (
      <Button variant="outline" disabled>
        <Wallet className="mr-2 h-4 w-4" />
        Connect Wallet
      </Button>
    );
  }

  if (!isConnected) {
    return (
      <Button
        variant="outline"
        onClick={handleConnect}
        disabled={isAuthenticating || isConnecting}
      >
        <Wallet className="mr-2 h-4 w-4" />
        {isConnecting ? "Connecting..." : isAuthenticating ? "Signing in..." : "Connect Wallet"}
      </Button>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline">
          <Wallet className="mr-2 h-4 w-4" />
          {formatAddress(address!)}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          onClick={() => navigator.clipboard.writeText(address!)}
        >
          <Copy className="mr-2 h-4 w-4" />
          Copy Address
        </DropdownMenuItem>
        <DropdownMenuItem onClick={handleDisconnect}>
          <LogOut className="mr-2 h-4 w-4" />
          Disconnect
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
