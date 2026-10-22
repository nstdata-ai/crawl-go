# Changelog

## Unreleased — v1.0.0

- Use the Nstdata API origin by default; add configurable API origin and per-response byte limit.
- Validate nil requests, integer timing/action fields, task IDs, cursor options and API envelopes.
- Preserve context cancellation, close responses, refuse redirects and omit sensitive bodies/URLs from errors.
- Decode inline binary content, distinguish missing from empty content, cache reference reads safely and retain task IDs, truncation and additional metadata.
- Replace unbounded examples with separate submission/continuation commands; add public-interface, installation, documentation and CI checks.

Migration from v0.1.1: handle `ErrContentUnavailable`, `ErrResponseTooLarge` and `*APIError`; larger responses need an explicit `MaxResponseBytes`. Task failures remain response states. Existing method signatures, request numeric field types, action helpers and module import path are retained. See README for default timeouts, supported configuration and service limitations.
