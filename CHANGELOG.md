# Changelog

## v1.0.0

- Use the Nstdata API origin by default; add configurable API origin and per-response byte limit.
- Validate nil requests, integer timing/action fields, task IDs, cursor options and API envelopes.
- Preserve context cancellation, close responses, refuse redirects and omit sensitive bodies/URLs from errors.
- Decode inline binary content, distinguish missing from empty content, cache reference reads safely and retain task IDs, truncation and additional metadata.
- Require valid UTF-8 for API JSON and HTML/Markdown references; failed text reads are not cached. Raw storage/binary bytes remain unchanged.
- Reject unpaired Unicode surrogate escapes in inline text instead of silently replacing content.
- Serialize `OnlyMainContent` explicitly as true/false in scrape and crawl requests. The zero value now sends false rather than delegating to the service default; set true when main-content filtering is required. Existing bool field types are retained.
- Propagate cancellation, deadlines and output errors immediately from the site example.
- Replace unbounded examples with separate submission/continuation commands; add public-interface, installation, documentation and CI checks.

Migration from v0.1.1: handle `ErrContentUnavailable`, `ErrResponseTooLarge` and `*APIError`; larger responses need an explicit `MaxResponseBytes`. Task failures remain response states. Existing method signatures, request numeric field types, action helpers and module import path are retained. See the usage guide linked from README for timeouts, configuration and service limitations.
