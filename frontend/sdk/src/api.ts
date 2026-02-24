let _baseUrl = "";

export function setBaseUrl(url: string) {
  _baseUrl = url;
}

export function getBaseUrl() {
  return _baseUrl;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(_baseUrl + path, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export interface ProductData {
  id: string;
  name: string;
  description?: string;
  price: string;
  currency: string;
  token_address: string;
  chain_id: number;
  image_url?: string;
  merchant_address?: string;
}

export interface PaymentLinkData {
  id: string;
  title: string;
  description?: string;
  amount: string;
  token_address: string;
  chain_id: number;
  active: boolean;
  merchant_address?: string;
}

export interface SubscriptionData {
  id: string;
  title: string;
  description?: string;
  amount: string;
  token_address: string;
  chain_id: number;
  period: string;
  merchant_address?: string;
}

export interface InvoiceData {
  id: string;
  invoice_number: number;
  title: string;
  memo?: string;
  amount: string;
  token_address: string;
  chain_id: number;
  status: string;
  recipient_email?: string;
  due_date?: string;
  line_items?: { description: string; quantity: number; unit_price: string; amount: string }[];
  notes?: string;
  merchant_address?: string;
}

export interface SubscriberData {
  id: string;
  status: string;
  method: string;
  periods_paid: number;
  periods_used: number;
  next_due?: string;
}

export interface RecordPayment {
  payer_address: string;
  payer_email?: string;
  token_address: string;
  chain_id: number;
  amount: string;
  tx_hash: string;
}

export const sdk = {
  getProduct: (id: string) => request<ProductData>(`/api/p/${id}`),
  recordProductPayment: (id: string, data: RecordPayment) =>
    request(`/api/p/${id}/payment`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    }),

  getPaymentLink: (id: string) => request<PaymentLinkData>(`/api/pay/${id}`),
  recordPaymentLinkPayment: (id: string, data: RecordPayment) =>
    request(`/api/pay/${id}/payment`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    }),
  recordPaymentLinkUse: (id: string) =>
    request(`/api/pay/${id}/use`, { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" }),

  getSubscription: (id: string) => request<SubscriptionData>(`/api/sub/${id}`),
  recordSubPayment: (id: string, data: RecordPayment) =>
    request(`/api/sub/${id}/payment`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    }),
  subscribe: (id: string, data: Record<string, unknown>) =>
    request<SubscriberData>(`/api/sub/${id}/subscribe`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    }),
  getSubscriber: (subId: string, payer: string) =>
    request<SubscriberData | null>(`/api/sub/${subId}/subscriber?payer=${encodeURIComponent(payer)}`),
  cancelSubscription: (id: string, payer: string) =>
    request<SubscriberData>(`/api/sub/${id}/cancel`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ payer_address: payer }),
    }),
  getInvoice: (id: string) => request<InvoiceData>(`/api/inv/${id}`),
  recordInvoicePayment: (id: string, data: RecordPayment) =>
    request(`/api/inv/${id}/payment`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    }),

  getRelayerAddress: () => request<{ address: string }>(`/api/relayer-address`),
};
