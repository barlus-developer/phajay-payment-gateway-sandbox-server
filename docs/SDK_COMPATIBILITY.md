# SDK compatibility investigation

Status: **implemented and tested through the actual SDK in process**. All six
sandbox routes, response decoding, token lookup/confirmation, and signed QR
callbacks pass integration tests. The verifier is in the explicit managed
`sdk-fork/`; upstream users require that fork or a release containing its patch.
Browser and Docker validation remain blocked; see DEVELOPMENT_STATUS.md.

Inspected on 2026-10-11: local SDK repository
`../phajay-payment-gateway`, commit
`1ebf09402433d71a9b629b9573b3dce0818183c9` (clean working tree).
Its source, rather than inferred gateway conventions, is the primary reference.
The remote SDK HEAD could not be checked because shell DNS access is unavailable.

## Client configuration

`phajay.New(Config, ...Option)` accepts `APIKey`, `BaseURL`,
`WebhookSignatureKey`, and `BypassSignatureVerification`. `WithSandbox()`
selects test paths; `BaseURL` supplies the server origin. `WithBaseURL()` can
override the configuration value. Verification defaults to enabled.

Expose only the following SDK payment routes. The server cannot observe whether
the caller used the option; requests to production paths must receive 404.

| SDK operation | Method and path | Authentication |
| --- | --- | --- |
| `CreatePaymentLink` | `POST /v1/api/test/payment/get-payment-link` | `Authorization: Basic <base64(raw API key)>` |
| `GenerateQR(BankJDB)` | `POST /v1/api/test/payment/generate-jdb-qr` | `secretKey: <API key>` |
| `GenerateQR(BankLDB)` | `POST /v1/api/test/payment/generate-ldb-qr` | `secretKey: <API key>` |
| `GenerateQR(BankIB)` | `POST /v1/api/test/payment/generate-ib-qr` | `secretKey: <API key>` |
| `GenerateQR(BankBCEL)` | `POST /v1/api/test/payment/generate-bcel-qr` | `secretKey: <API key>` |
| `GenerateQR(BankSTB)` | `POST /v1/api/test/payment/generate-stb-qr` | `secretKey: <API key>` |

Basic authentication encodes the key alone, without a colon or username/password
pair. `net/http.Request.BasicAuth()` therefore does not match this contract.
See `request.go`, `payment_link.go`, `payment_qr.go`, and `sandbox_test.go`.

M MoneyX, transaction-status queries, refunds, credit cards, and subscriptions
have no supported sandbox routes. The SDK rejects them before making a request.
Portal transaction queries and SSE are separate internal APIs.

## Requests and responses

Both payment requests contain numeric `amount` (`float64` in the SDK), string
`description`, and string `orderNo`, `tag1`, `tag2`, `tag3`. QR `orderNo` uses
`omitempty`; link `orderNo` and all tag fields do not. There is no request
currency field. The SDK validates positive finite amounts and nonblank descriptions;
it does not enforce a currency scale or BCEL character restrictions.

Parse the incoming JSON number as an exact decimal before conversion to the
selected currency's fixed-point integer storage. Reject unsupported precision,
overflow, and invalid amounts; do not round silently or use floating-point SQL.
Generate an order reference when `orderNo` is empty and enforce project-scoped
uniqueness when supplied.

Payment-link response fields are string `message`, **`redirectURL`**, and
`orderNo`. QR response fields are string `message`, `transactionId`, `qrCode`,
and `link`, with no extra compatibility fields. `qrCode` must contain the opaque
sandbox payment URL to encode as a QR image; `link` must open the working
simulator. No bank-transfer payloads or bank-app deep links may be generated.

Non-2xx responses become `*phajay.APIError`, including status, operation, and up
to 1 MiB of response body. Return sanitized JSON errors. The SDK accepts any
2xx response that decodes; success-message values are not validated.

## Callback contracts

Use `SandboxPaymentLinkWebhookCallback` for payment links, not the production
`PaymentLinkWebhookCallback`. Its wire fields are:

```text
message, _id, orderNo, linkCode, amount, currency, descriptione, status,
user, tag1..tag6, isMerchantIdPayment, isAffiliatePayment, createdAt, updateAt
```

All fields are optional pointers in the SDK. `amount` is numeric, the two flags
are boolean, and the remaining fields are strings. Preserve `descriptione` and
`updateAt` exactly. Link callbacks use `amount` and `user`, rather than the
production callback's `txnAmount` and `userId`.

QR callbacks use `PaymentQRWebhookCallback`: `paymentMethod` and `transactionId`
are strings and `txnAmount` is numeric. Optional fields include `status`,
`message`, `billNumber`, `refNo`, `exReferenceNo`, `merchantName`, `description`,
`txnDateTime`, `sourceAccount`, `sourceName`, `sourceCurrency`, `userId`, and
`tag1` through `tag6`. `refNo` accepts a string or JSON number through
`FlexibleString`; other optional fields are string pointers.

`PAYMENT_COMPLETED` is the completion value. The SDK also defines `WAITING` and
`EXPIRED`. `FAILED` and `CANCELED` must be documented as simulator concepts;
do not claim undocumented production callback semantics.

## Documentation differences and selected contract

Cross-checked the public [sandbox link documentation](https://payment-doc.lailaolab.com/v1/sandbox/paymentLink-sandbox)
and [sandbox QR documentation](https://payment-doc.lailaolab.com/v1/sandbox/paymentQR-sandbox).

| Difference | Selected behavior |
| --- | --- |
| Master prompt mentions `redirectUrl`; SDK and official guide use `redirectURL`. | Emit `redirectURL` exactly. |
| Official link response example omits `orderNo`; SDK models it. | Return the stored/generated `orderNo` alongside the URL. |
| Link guide lists two tags; SDK sends three. | Accept and preserve `tag1`, `tag2`, `tag3`. |
| QR field-table descriptions swap transaction ID and QR meanings. | Follow SDK types and the guide's example: `transactionId` is the ID, `qrCode` is the payload. |
| Official QR example encodes a bank-like payload and uses an app deep link. | Retain field types but use only a self-hosted simulator URL, as required by this platform's safety contract. |
| Link guide has callback fields beyond the SDK model. | Target the modeled sandbox callback fields; do not replace it with the production type. |
| Examples vary between `SUCCESS` and `SUCCESSFULLY`. | SDK accepts arbitrary message strings; this server returns descriptive sandbox messages. |

## Required SDK signature patch

In this SDK snapshot, `VerifyWebhookSignature()` returns
`ErrWebhookSignatureKeyMissing` for a blank signing key and
`ErrWebhookSignatureVerificationUnavailable` for every configured-key request,
including `WithSandbox()`. `DecodeWebhook()` verifies before unmarshaling and
leaves the callback unchanged on verification failure. Consequently, the
default-enabled sandbox verification does not work in the unpatched upstream
snapshot. The explicitly managed `sdk-fork/` now implements the following patch.

Patch the separate SDK repository or an explicitly identified local fork:

1. Keep the existing API and default verification policy.
2. After the configured-key check, dispatch to the custom verifier only when
   `p.sandbox` is true; preserve production's existing failure behavior.
3. Verify HMAC-SHA256 over the exact bytes of `{timestamp}.{rawRequestBody}`
   with `WebhookSignatureKey`; require lowercase hex prefixed by `v1=`.
4. Read `X-Phajay-Sandbox-Timestamp`, `X-Phajay-Sandbox-Signature`, and
   `X-Phajay-Sandbox-Event-ID`; enforce a five-minute timestamp tolerance and
   constant-time digest comparison. Preserve event IDs for consumer deduplication.
5. Test correct/incorrect/missing signatures, changed bodies, stale and future
   timestamps, wrong keys, rotation overlap, and unchanged production behavior.

This is a custom sandbox protocol, not Phajay's private production algorithm.
The [official signature guide](https://payment-doc.lailaolab.com/v1/verify-webhook-signature)
does not publish that algorithm. Never use API keys as signing keys or enable
bypass as the normal integration path. A rotation overlap can retain both keys
at the consumer using separate configured sandbox clients, without changing
the SDK's public API. Consumers must deduplicate durable event IDs because
delivery is at least once.

## Validation status

The SDK's existing sandbox tests explicitly cover all six paths, raw-key Basic
authentication, `secretKey`, and response decoding. Its current webhook-policy
test explicitly asserts that sandbox mode does not bypass verification.
The platform additionally tests all six operations using the real SDK against
Gin through an in-process HTTP transport, avoiding prohibited local listeners.
Amounts use LAK scale two; link checkout currently simulates JDB. Token URLs
are the only QR payload. Execution results and remaining blockers are recorded in
[DEVELOPMENT_STATUS.md](DEVELOPMENT_STATUS.md).
