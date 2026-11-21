import { http } from "@/services/api/request";
import type { CreditSummary } from "@/services/api/auth-assets";

export interface PaymentProductItem {
  id: string;
  title: string;
  amountCents: number;
  creditMicrocredits: number;
  bonusMicrocredits: number;
}

export interface PaymentOrderItem {
  id: string;
  outTradeNo: string;
  productId: string;
  amountCents: number;
  creditMicrocredits: number;
  status: "pending" | "paid" | "closed" | "failed";
  checkoutJson?: string;
  paidAt?: string;
  createdAt: string;
}

export interface CreditLedgerItem {
  id: string;
  delta: number;
  balanceAfter: number;
  kind: string;
  refType: string;
  refId: string;
  createdAt: string;
}

export interface AdminOverviewData {
  totalUsers: number;
  totalTasks: number;
  succeededTasks: number;
  totalBillingOrders: number;
  paidPaymentOrders: number;
  totalRevenueCents: number;
  totalRedeemBatches: number;
}

export interface RedeemBatchItem {
  id: string;
  name: string;
  creditMicrocredits: number;
  totalCount: number;
  usedCount: number;
  createdAt: string;
}

export const billingApi = {
  listProducts: () => http.get<{ items: PaymentProductItem[] }>("/v1/payments/products"),
  listOrders: () => http.get<{ items: PaymentOrderItem[] }>("/v1/payments/orders"),
  createOrder: (productId: string) =>
    http.post<{ order: PaymentOrderItem; checkout: { checkoutType: string; codeUrl?: string } }>(
      "/v1/payments/orders",
      { productId },
    ),
  redeemCode: (code: string) => http.post<{ credits: CreditSummary }>("/v1/credits/redeem", { code }),
  listLedger: () => http.get<{ items: CreditLedgerItem[] }>("/v1/credits/ledger"),

  getAdminOverview: () => http.get<AdminOverviewData>("/v1/admin/analytics/overview"),
  listRedeemBatches: () => http.get<{ items: RedeemBatchItem[] }>("/v1/admin/redeem-batches"),
  createRedeemBatch: (name: string, creditMicrocredits: number, count: number) =>
    http.post<{ batch: RedeemBatchItem; codes: string[] }>("/v1/admin/redeem-batches", {
      name,
      creditMicrocredits,
      count,
    }),
};
