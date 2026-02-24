import { useAccount, useConnect, useDisconnect, useConnectors } from "wagmi";

interface WalletButtonProps {
  onConnected?: () => void;
}

const CONNECTOR_META: Record<string, { icon: string; label: string }> = {
  injected: { icon: "🦊", label: "Browser Wallet" },
  metaMask: { icon: "🦊", label: "MetaMask" },
  walletConnect: { icon: "🔗", label: "WalletConnect" },
  coinbaseWallet: { icon: "💰", label: "Coinbase Wallet" },
};

export function WalletButton({ onConnected }: WalletButtonProps) {
  const { isConnected, address } = useAccount();
  const connectors = useConnectors();
  const { connectAsync, isPending } = useConnect();
  const { disconnect } = useDisconnect();

  if (isConnected && address) {
    return (
      <div>
        <p className="cpay-addr">
          {address.slice(0, 6)}...{address.slice(-4)}{" "}
          <button
            className="cpay-back"
            onClick={() => disconnect()}
            style={{ display: "inline", marginLeft: 4 }}
          >
            disconnect
          </button>
        </p>
      </div>
    );
  }

  const handleConnect = async (connector: (typeof connectors)[number]) => {
    try {
      await connectAsync({ connector });
      onConnected?.();
    } catch {
      // user rejected or error — silently ignore
    }
  };

  if (isPending) {
    return (
      <button className="cpay-btn cpay-btn-primary" disabled>
        Connecting...
      </button>
    );
  }

  // If only one connector, show a simple button
  if (connectors.length === 1) {
    return (
      <button
        className="cpay-btn cpay-btn-primary"
        onClick={() => handleConnect(connectors[0])}
      >
        🔗 Connect Wallet to Pay
      </button>
    );
  }

  // Multiple connectors — show list
  return (
    <div className="cpay-wallets">
      {connectors.map((c) => {
        const meta = CONNECTOR_META[c.id] || CONNECTOR_META[c.type] || {
          icon: "🔗",
          label: c.name,
        };
        return (
          <button
            key={c.uid}
            className="cpay-wallet-btn"
            onClick={() => handleConnect(c)}
          >
            <span style={{ fontSize: 20 }}>{meta.icon}</span>
            <span className="cpay-wallet-name">{c.name || meta.label}</span>
          </button>
        );
      })}
    </div>
  );
}
