class ApiError extends Error {
  constructor(
    public status: number,
    message: string
  ) {
    super(message);
  }
}

async function request<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const token = typeof window !== "undefined" ? localStorage.getItem("token") : null;

  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string>),
  };

  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  // Don't set Content-Type for FormData (browser sets it with boundary)
  if (!(options.body instanceof FormData)) {
    headers["Content-Type"] = "application/json";
  }

  const res = await fetch(path, { ...options, headers });

  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new ApiError(res.status, body.error || res.statusText);
  }

  if (res.status === 204 || res.headers.get("Content-Length") === "0") {
    return undefined as T;
  }

  return res.json();
}

export const api = {
  auth: {
    nonce: () => request<{ nonce: string }>("/api/auth/siwe/nonce", { method: "POST" }),
    verify: (message: string, signature: string) =>
      request<{ token: string; user: { id: string; wallet_address: string; role: string; created_at: string; updated_at: string } }>(
        "/api/auth/siwe/verify",
        {
          method: "POST",
          body: JSON.stringify({ message, signature }),
        }
      ),
  },
  user: {
    profile: () => request<{ id: string; wallet_address: string; role: string; email?: string }>("/api/user/profile"),
    setRole: (role: string) =>
      request("/api/user/role", {
        method: "PUT",
        body: JSON.stringify({ role }),
      }),
    updateEmail: (email: string) =>
      request("/api/user/email", {
        method: "PUT",
        body: JSON.stringify({ email }),
      }),
  },
  shops: {
    create: (data: { name: string; website?: string; title?: string; avatar_url?: string }) =>
      request("/api/shops", { method: "POST", body: JSON.stringify(data) }),
    getMine: () => request("/api/shops"),
    update: (id: string, data: { name: string; website?: string; title?: string; avatar_url?: string }) =>
      request(`/api/shops/${id}`, { method: "PUT", body: JSON.stringify(data) }),
  },
  products: {
    create: (data: {
      name: string;
      description?: string;
      price: string;
      currency?: string;
      token_address?: string;
      chain_id?: number;
      image_url?: string;
    }) => request<Product>("/api/products", { method: "POST", body: JSON.stringify(data) }),
    list: () => request<Product[]>("/api/products"),
    getByID: (id: string) => request<Product>(`/api/products/${id}`),
    getPublic: (id: string) => request<Product>(`/api/p/${id}`),
    update: (
      id: string,
      data: {
        name: string;
        description?: string;
        price: string;
        currency?: string;
        token_address?: string;
        chain_id?: number;
        image_url?: string;
      }
    ) => request<Product>(`/api/products/${id}`, { method: "PUT", body: JSON.stringify(data) }),
    toggleActive: (id: string, active: boolean) =>
      request<{ active: boolean }>(`/api/products/${id}/active`, {
        method: "PATCH",
        body: JSON.stringify({ active }),
      }),
    delete: (id: string) =>
      request<void>(`/api/products/${id}`, { method: "DELETE" }),
  },
  upload: {
    avatar: async (file: File): Promise<{ url: string }> => {
      const formData = new FormData();
      formData.append("file", file);
      return request("/api/upload/avatar", { method: "POST", body: formData });
    },
    productImage: async (file: File): Promise<{ url: string }> => {
      const formData = new FormData();
      formData.append("file", file);
      return request("/api/upload/product-image", { method: "POST", body: formData });
    },
  },
  paymentLinks: {
    create: (data: {
      title: string;
      description?: string;
      amount: string;
      max_uses?: number;
    }) => request<PaymentLink>("/api/payment-links", { method: "POST", body: JSON.stringify(data) }),
    list: () => request<PaymentLink[]>("/api/payment-links"),
    toggleActive: (id: string, active: boolean) =>
      request<{ active: boolean }>(`/api/payment-links/${id}/active`, {
        method: "PATCH",
        body: JSON.stringify({ active }),
      }),
    delete: (id: string) => request<void>(`/api/payment-links/${id}`, { method: "DELETE" }),
    getPublic: (id: string) => request<PaymentLink>(`/api/pay/${id}`),
    recordUse: (id: string) =>
      request<{ use_count: number }>(`/api/pay/${id}/use`, { method: "POST" }),
    checkPayment: (linkId: string, payer: string) =>
      request<Payment | null>(`/api/pay/${linkId}/payment?payer=${encodeURIComponent(payer)}`),
    recordPayment: (id: string, data: RecordPaymentData) =>
      request<Payment>(`/api/pay/${id}/payment`, { method: "POST", body: JSON.stringify(data) }),
  },
  subscriptions: {
    create: (data: {
      title: string;
      description?: string;
      amount: string;
      token_address: string;
      chain_id?: number;
      period: string;
    }) => request<Subscription>("/api/subscriptions", { method: "POST", body: JSON.stringify(data) }),
    list: () => request<Subscription[]>("/api/subscriptions"),
    toggleActive: (id: string, active: boolean) =>
      request<Subscription>(`/api/subscriptions/${id}/active`, {
        method: "PATCH",
        body: JSON.stringify({ active }),
      }),
    delete: (id: string) => request<void>(`/api/subscriptions/${id}`, { method: "DELETE" }),
    getPublic: (id: string) => request<Subscription>(`/api/sub/${id}`),
    checkPayment: (subId: string, payer: string) =>
      request<SubscriptionPayment | null>(`/api/sub/${subId}/payment?payer=${encodeURIComponent(payer)}`),
    recordPayment: (id: string, data: RecordPaymentData) =>
      request<SubscriptionPayment>(`/api/sub/${id}/payment`, { method: "POST", body: JSON.stringify(data) }),
    listPayments: (id: string) =>
      request<SubscriptionPayment[]>(`/api/subscriptions/${id}/payments`),
    subscribe: (id: string, data: { payer_address: string; email: string; method: string; periods?: number; approved_amount?: string }) =>
      request<Subscriber>(`/api/sub/${id}/subscribe`, { method: "POST", body: JSON.stringify(data) }),
    cancel: (id: string, data: { payer_address: string }) =>
      request<Subscriber>(`/api/sub/${id}/cancel`, { method: "POST", body: JSON.stringify(data) }),
    getSubscriber: (subId: string, payer: string) =>
      request<Subscriber | null>(`/api/sub/${subId}/subscriber?payer=${encodeURIComponent(payer)}`),
    getRelayerAddress: () =>
      request<{ address: string }>("/api/relayer-address"),
    listSubscribers: (id: string) =>
      request<Subscriber[]>(`/api/subscriptions/${id}/subscribers`),
  },
  payments: {
    listMine: () => request<Payment[]>("/api/payments"),
    listByProduct: (productId: string) =>
      request<Payment[]>(`/api/products/${productId}/payments`),
    checkForProduct: (productId: string, payer: string) =>
      request<Payment | null>(`/api/p/${productId}/payment?payer=${encodeURIComponent(payer)}`),
    recordForProduct: (productId: string, data: RecordPaymentData) =>
      request<Payment>(`/api/p/${productId}/payment`, { method: "POST", body: JSON.stringify(data) }),
  },
  invoices: {
    create: (data: {
      title: string;
      memo?: string;
      amount: string;
      token_address?: string;
      chain_id?: number;
      recipient_email?: string;
      due_date?: string;
      line_items?: { description: string; quantity: number; unit_price: string; amount: string }[];
      notes?: string;
    }) => request<Invoice>("/api/invoices", { method: "POST", body: JSON.stringify(data) }),
    list: () => request<Invoice[]>("/api/invoices"),
    getByID: (id: string) => request<Invoice>(`/api/invoices/${id}`),
    update: (id: string, data: {
      title: string;
      memo?: string;
      amount: string;
      token_address?: string;
      chain_id?: number;
      recipient_email?: string;
      due_date?: string;
      line_items?: { description: string; quantity: number; unit_price: string; amount: string }[];
      notes?: string;
    }) => request<Invoice>(`/api/invoices/${id}`, { method: "PUT", body: JSON.stringify(data) }),
    send: (id: string) =>
      request<Invoice>(`/api/invoices/${id}/send`, { method: "POST" }),
    cancel: (id: string) =>
      request<Invoice>(`/api/invoices/${id}/cancel`, { method: "POST" }),
    delete: (id: string) =>
      request<void>(`/api/invoices/${id}`, { method: "DELETE" }),
    getPublic: (id: string) => request<Invoice>(`/api/inv/${id}`),
    recordPayment: (id: string, data: RecordPaymentData) =>
      request<Invoice>(`/api/inv/${id}/payment`, { method: "POST", body: JSON.stringify(data) }),
  },
  cf: {
    getAddress: () => request<{ address: string }>("/api/cf-address"),
    getBalance: (token: string) =>
      request<{ address: string; balance: string }>(`/api/cf-balance?token=${encodeURIComponent(token)}`),
    getWithdrawInfo: (token: string) =>
      request<{ cf_address: string; balance: string; is_deployed: boolean; token: string; chain_id: number }>(
        `/api/cf-withdraw-info?token=${encodeURIComponent(token)}`
      ),
    recordWithdrawal: (data: { token_address: string; chain_id: number; amount: string; tx_hash: string }) =>
      request<{ id: string; tx_hash: string }>("/api/withdrawals", { method: "POST", body: JSON.stringify(data) }),
  },
};

export interface PaymentLink {
  id: string;
  shop_id: string;
  title: string;
  description?: string;
  amount: string;
  token_address: string;
  chain_id: number;
  max_uses?: number;
  use_count: number;
  active: boolean;
  merchant_address?: string;
  created_at: string;
  updated_at: string;
}

export interface Product {
  id: string;
  shop_id: string;
  name: string;
  description?: string;
  price: string;
  currency: string;
  token_address: string;
  chain_id: number;
  image_url?: string;
  active: boolean;
  merchant_address?: string;
  created_at: string;
  updated_at: string;
}

export interface Subscription {
  id: string;
  shop_id: string;
  title: string;
  description?: string;
  amount: string;
  token_address: string;
  chain_id: number;
  period: string;
  active: boolean;
  merchant_address?: string;
  created_at: string;
  updated_at: string;
}

export interface Payment {
  id: string;
  shop_id: string;
  kind: string;
  product_id?: string;
  payment_link_id?: string;
  payer_address?: string;
  payer_email?: string;
  token_address: string;
  chain_id: number;
  amount: string;
  tx_hash: string;
  created_at: string;
}

export interface SubscriptionPayment {
  id: string;
  subscription_id: string;
  shop_id: string;
  payer_address: string;
  amount: string;
  token_address: string;
  chain_id: number;
  tx_hash: string;
  created_at: string;
}

export interface Subscriber {
  id: string;
  subscription_id: string;
  shop_id: string;
  payer_address: string;
  payer_email: string;
  status: string;
  method: string;
  periods_paid: number;
  periods_used: number;
  approved_amount?: string;
  spent_amount?: string;
  next_due?: string;
  cancelled_at?: string;
  expires_at?: string;
  created_at: string;
  updated_at: string;
}

export interface Invoice {
  id: string;
  shop_id: string;
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
  payer_address?: string;
  tx_hash?: string;
  paid_at?: string;
  merchant_address?: string;
  created_at: string;
  updated_at: string;
}

export interface RecordPaymentData {
  payer_address: string;
  payer_email?: string;
  token_address: string;
  chain_id: number;
  amount: string;
  tx_hash: string;
}
