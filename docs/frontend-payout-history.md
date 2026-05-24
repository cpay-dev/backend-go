# Frontend Payout History Integration

## Current backend surface

Payout history is currently exposed through the authenticated payments list, not through a standalone payouts endpoint.

Use:

```http
GET /v1/payments?limit=50&offset=0
Authorization: Bearer <access_token>
```

Do not use `GET /v1/payouts`; that route is not available.

## Query parameters

| Parameter | Type | Notes |
| --- | --- | --- |
| `limit` | number | Optional. Defaults to `50`, max `200`. |
| `offset` | number | Optional. Defaults to `0`. |
| `product_id` | string | Optional. Filters by product. |
| `payment_link_id` | string | Optional. Filters by payment link. |

## Response shape

```ts
type PaymentsResponse = {
  data: PaymentListItem[]
  limit: number
  offset: number
  total: number
}

type PaymentListItem = {
  id: string
  checkout_session_id: string
  payment_link_id: string
  product_id: string | null
  product_name: string | null
  payment_link_title: string
  payment_link_code: string

  amount: number
  currency: string
  customer_email: string | null

  status: string
  chain: string
  token_symbol: string
  expected_amount: number
  received_amount: number
  tx_hash: string | null
  confirmations: number
  required_confirmations: number

  expires_at: string
  created_at: string
  updated_at: string

  wallet_type: string | null
  payout_status: "scheduled" | "processing" | "completed" | "failed" | null
  payout_item_status: "pending" | "processing" | "completed" | "failed" | null
  payout_tx_hash: string | null
  payout_last_error: string | null
}
```

## Field semantics

| Field | Meaning |
| --- | --- |
| `tx_hash` | Customer payment transaction hash. |
| `payout_tx_hash` | Merchant settlement/sweep transaction hash. Null until payout item completes. |
| `status` | Payment intent status. Treat `settled` as fully swept to merchant settlement wallet. |
| `payout_status` | Batch payout status for the payment's latest payout row. |
| `payout_item_status` | Per-payment payout item status. Use this for row-level payout state. |
| `payout_last_error` | Latest per-payment payout error. Show only in admin/debug UI. |

## Suggested UI mapping

| Condition | Display |
| --- | --- |
| `status === "settled"` | Settled |
| `payout_item_status === "completed"` | Settled |
| `payout_item_status === "processing"` | Payout processing |
| `payout_status === "scheduled"` | Payout scheduled |
| `payout_item_status === "failed"` | Payout failed |
| no payout fields yet and payment is confirmed/overpaid | Awaiting payout |

Show two transaction links if both hashes exist:

- `tx_hash`: customer payment.
- `payout_tx_hash`: merchant settlement payout.

Use the row `chain` to choose the block explorer.

## Example fetch

```ts
export async function listPayments(token: string, params = new URLSearchParams()) {
  params.set("limit", params.get("limit") ?? "50")
  params.set("offset", params.get("offset") ?? "0")

  const res = await fetch(`/v1/payments?${params.toString()}`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })

  if (!res.ok) {
    throw new Error(`Failed to load payments: ${res.status}`)
  }

  return (await res.json()) as PaymentsResponse
}
```

## Notes

- Payouts are automatic. There is no manual withdrawal action in the current product version.
- Settlement wallet configuration is managed through `GET /v1/merchant/settings` and `PATCH /v1/merchant/settings`.
- The payout hash field was added as `payout_tx_hash`; older backend builds may not include it until `api-gateway` is rebuilt and restarted.
