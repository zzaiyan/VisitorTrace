# Use server-rendered Admin Console and Public Analytics

Status: accepted

The first browser interface uses Go server-rendered HTML with embedded CSS and small dependency-free scripts for live Map Preset previews and trend bars. It stays in the same VisitorTrace process as ingestion and SVG rendering.

This keeps production deployment to one executable, avoids a Node.js runtime and a separate frontend release, and gives the domestic personal-site deployment a small asset and operational footprint. The trade-off is that this first interface does not provide a general-purpose client-side analytics application; richer interactions can be added later without changing the public aggregation or SVG contracts.
