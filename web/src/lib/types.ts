export type Developer = { id: string; email: string; name: string };
export type Session = {
  id: string;
  createdAt: number;
  expiresAt: number;
  current: boolean;
};
export type Account = {
  developer: Developer;
  session: Session;
  csrfToken: string;
};
export type Project = {
  id: string;
  name: string;
  merchantName: string;
  description: string;
  paymentTTL: number;
  webhookMaxAttempts: number;
  createdAt: number;
};
export type PaymentState =
  "WAITING" | "PAYMENT_COMPLETED" | "FAILED" | "EXPIRED" | "CANCELED";
export type Transaction = {
  id: string;
  projectId: string;
  orderNo: string;
  kind: string;
  bank: string;
  merchantName: string;
  amountMinor: number;
  currency: string;
  description: string;
  status: PaymentState;
  createdAt: number;
  expiresAt: number;
  completedAt: number | null;
  receiptRef: string;
  paymentURL: string;
  checkoutURL: string;
  tag1: string;
  tag2: string;
  tag3: string;
};
export type History = { id: string; status: PaymentState; createdAt: number };
export type Key = {
  id: string;
  name?: string;
  prefix: string;
  createdAt: number;
  lastUsedAt?: number | null;
  revokedAt: number | null;
  active?: boolean;
  validUntil?: number | null;
};
export type Endpoint = {
  id: string;
  url: string;
  enabled: boolean;
  events: string[];
  createdAt: number;
};
export type Job = {
  id: string;
  eventId: string;
  endpointId: string;
  url: string;
  status: string;
  attempts: number;
  nextAttemptAt: number;
};
export type Delivery = {
  id: string;
  attempt: number;
  httpStatus: number;
  response: string;
  error: string;
  createdAt: number;
  durationMS: number;
};
export type APILog = {
  id: string;
  method: string;
  path: string;
  status: number;
  durationMS: number;
  error: string;
  createdAt: number;
};
export type DashboardData = {
  total: number;
  completed: number;
  pending: number;
  failed: number;
  volumeMinor: number;
  webhookDelivered: number;
  webhookFailed: number;
  activity: { day: string; total: number; completed: number }[];
};
export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};
