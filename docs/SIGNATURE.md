# Custom sandbox webhook signatures

This is a self-hosted sandbox protocol. It does not implement Phajay's private
production signing algorithm. Production SDK verification remains unchanged.
The current upstream SDK needs the patch maintained in `sdk-fork/`; see its
`FORK.md` before integrating. Default verification is enabled and requires no bypass.

## Wire format

The signing key is a dedicated project secret, never an API key. For every
delivery attempt, choose a current Unix timestamp in seconds and compute:

```text
message = ASCII(timestamp) + "." + exactRawHTTPBody
signature = "v1=" + lowercaseHex(HMAC-SHA256(signingKey, message))
```

Send `X-Phajay-Sandbox-Timestamp`, `X-Phajay-Sandbox-Signature`, and
`X-Phajay-Sandbox-Event-ID`. The event ID stays constant across automatic and
manual retries; the timestamp and signature are refreshed. The event header is
required but is not itself included in the signed message. Deduplicate payments
by the signed transaction identifier as well as event ID.

Read raw body bytes once (maximum 1 MiB) before JSON decoding. Do not reserialize
JSON, trim whitespace, or normalize fields. Reject missing/duplicate headers,
malformed timestamps/signatures, incorrect digests, changed bodies and timestamps
more than 300 seconds in the past or future. The SDK compares binary digests
with constant-time `hmac.Equal`.

## SDK receiver

Configure `WebhookSignatureKey` with the shown-once project signing secret and
select `WithSandbox()`. Use `DecodeWebhook` with the original bytes and headers.
QR payments and test events decode into `PaymentQRWebhookCallback`; link payments
decode into `SandboxPaymentLinkWebhookCallback`. Verification failure leaves
the destination callback untouched.

After successful verification, atomically store the event ID and apply your
business operation. Check transaction ID, amount and expected currency/project
context against your saved order. Acknowledge already-processed events with 2xx.
Delivery is at least once; a lost HTTP acknowledgement can cause a duplicate.

## Rotation and storage

Generate a new signing key in the portal and securely update your callback
configuration. The server immediately signs new attempts with the active key.
Previous keys show `validUntil = rotationTime + 300 seconds`. Keep two sandbox
SDK clients during that overlap: try current, then previous only before its
recorded expiry. Remove old verifiers at expiry or immediately on revocation.
There is no automatic fallback to an API credential or verification bypass.

Secrets are encrypted with AES-GCM, random nonces and project-scoped additional
authenticated data. The 32-byte base64 master key stays outside SQLite. Keep it
stable and protect it independently alongside database backups.

## Known test vector

Timestamp: `1700000000`; body bytes: `{"amount":100}`; key: `signing-key`.

```text
v1=91b6fb8cac6f35bf5b1234f49fdcad62ee625615202b5af35e8fb4e8b30add5f
```

An old fixture timestamp will be rejected by live verification; the unit test
injects a deterministic clock. Tests cover valid signatures, wrong keys,
tampered payloads, missing/duplicate headers, expired/future timestamps,
boundary tolerance, rotation and unchanged production policy.
