# Accept public browser tracking ingestion

The tracked Sites are static and cannot keep client secrets, so the tracking endpoint accepts unauthenticated reports addressed by a public Site ID and constrains them with Allowed Origin checks, CORS, rate limits, payload validation, and basic bot filtering. This preserves static-site compatibility and low operational cost while explicitly accepting that a determined sender can spoof requests and pollute statistics; stronger authenticity would require a server-side proxy or dynamic signature service for every Site.
