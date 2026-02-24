import { useEffect, useState } from "react";
import { useAccount, useWriteContract, useSwitchChain } from "wagmi";
import { parseUnits } from "viem";
import { sdk, type ProductData } from "../api";
import { WalletButton } from "./WalletButton";
import type { CheckoutCallbacks } from "../App";

const ERC20_TRANSFER_ABI = [
  {
    name: "transfer",
    type: "function",
    inputs: [
      { name: "to", type: "address" },
      { name: "value", type: "uint256" },
    ],
    outputs: [{ name: "", type: "bool" }],
    stateMutability: "nonpayable",
  },
] as const;

interface Props {
  id: string;
  callbacks: CheckoutCallbacks;
}

export function ProductCheckout({ id, callbacks }: Props) {
  const [product, setProduct] = useState<ProductData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [email, setEmail] = useState("");
  const [payStatus, setPayStatus] = useState<"idle" | "sending" | "success" | "error">("idle");

  const { address, isConnected, chainId } = useAccount();
  const { writeContractAsync } = useWriteContract();
  const { switchChainAsync } = useSwitchChain();

  useEffect(() => {
    sdk
      .getProduct(id)
      .then(setProduct)
      .catch(() => setError("Product not found or unavailable."))
      .finally(() => setLoading(false));
  }, [id]);

  const handlePay = async () => {
    if (!product || !isConnected || !product.merchant_address || !address || !email.trim()) return;
    setPayStatus("sending");
    try {
      if (chainId !== product.chain_id) {
        try {
          await switchChainAsync({ chainId: product.chain_id });
        } catch {
          // wallet may handle internally
        }
      }

      const cfAddress = product.merchant_address as `0x${string}`;
      const amountInUnits = parseUnits(product.price, 6);

      const hash = await writeContractAsync({
        chainId: product.chain_id,
        address: product.token_address as `0x${string}`,
        abi: ERC20_TRANSFER_ABI,
        functionName: "transfer",
        args: [cfAddress, amountInUnits],
      });

      await sdk.recordProductPayment(id, {
        payer_address: address,
        payer_email: email.trim(),
        token_address: product.token_address,
        chain_id: product.chain_id,
        amount: product.price,
        tx_hash: hash,
      });

      setPayStatus("success");
      callbacks.onSuccess?.({
        type: "product",
        id,
        tx_hash: hash,
        payer_address: address,
        amount: product.price,
      });
    } catch (err: unknown) {
      console.error("Payment failed:", err);
      setPayStatus("error");
      callbacks.onError?.({
        code: "PAYMENT_FAILED",
        message: err instanceof Error ? err.message : "Transaction failed",
      });
    }
  };

  if (loading) {
    return (
      <div className="cpay-card">
        <div className="cpay-body">
          <p style={{ textAlign: "center", color: "#71717a" }}>Loading...</p>
        </div>
      </div>
    );
  }

  if (error || !product) {
    return (
      <div className="cpay-card">
        <div className="cpay-body">
          <p className="cpay-err">{error || "Product not found."}</p>
        </div>
      </div>
    );
  }

  if (payStatus === "success") {
    return (
      <div className="cpay-card">
        <div className="cpay-ok">
          <div className="cpay-ok-icon">&#x2705;</div>
          <p className="cpay-ok-title">Payment Sent!</p>
          <p className="cpay-ok-sub">Thank you for purchasing {product.name}.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="cpay-card">
      {product.image_url && (
        <img src={product.image_url} alt={product.name} className="cpay-img" />
      )}
      <div className="cpay-body">
        <h3 className="cpay-title">{product.name}</h3>
        {product.description && <p className="cpay-desc">{product.description}</p>}

        <div className="cpay-price-box">
          <p className="cpay-price">{product.price}</p>
          <p className="cpay-price-sub">{product.currency} on Polygon</p>
        </div>

        {payStatus === "error" && (
          <p className="cpay-err">Transaction failed. Please try again.</p>
        )}

        {!product.merchant_address ? (
          <p className="cpay-err">Merchant wallet not configured.</p>
        ) : !isConnected ? (
          <WalletButton />
        ) : (
          <>
            <div className="cpay-group">
              <label className="cpay-label">Email for receipt</label>
              <input
                className="cpay-input"
                type="email"
                placeholder="you@example.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>
            <button
              className="cpay-btn cpay-btn-primary"
              onClick={handlePay}
              disabled={payStatus === "sending" || !email.trim()}
            >
              {payStatus === "sending"
                ? "Sending..."
                : `Pay ${product.price} ${product.currency}`}
            </button>
            <p className="cpay-addr">
              {address?.slice(0, 6)}...{address?.slice(-4)}
            </p>
          </>
        )}

        <div className="cpay-powered">
          Powered by <a href="https://cpay.dev" target="_blank" rel="noopener">CPay</a>
        </div>
      </div>
    </div>
  );
}
