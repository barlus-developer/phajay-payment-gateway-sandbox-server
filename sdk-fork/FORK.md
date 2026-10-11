# Managed sandbox SDK fork

Explicit local fork of `github.com/barlus-developer/phajay-payment-gateway`, copied from source commit `1ebf09402433d71a9b629b9573b3dce0818183c9`. Original license, source, and tests are retained. Production endpoint and verification behavior are unchanged.

Changes: `webhook.go` selects the custom verifier only with `WithSandbox()`; `webhook_sandbox.go` and tests implement HMAC-SHA256 over timestamp plus exact body. One original policy assertion now expects missing sandbox signatures to fail. Existing public method signatures are unchanged; two descriptive errors are added.

The platform explicitly uses a local module `replace` for integration testing. Consumers must use this fork through their own explicit replacement, or release these changes from the SDK repository first. The current upstream SDK does not enable this verifier.

Rotation: retain both merchant keys during the advertised five-minute overlap. Verify with the current SDK client, then the previous client only before its recorded `validUntil`. Record event IDs for idempotency. The required event header is not part of the signed message; also use stable transaction identifiers when processing payments.
