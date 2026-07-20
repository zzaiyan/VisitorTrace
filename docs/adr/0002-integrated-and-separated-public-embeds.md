# Offer integrated and separated public embeds

Status: accepted

The service exposes both an Integrated Widget, which gives a site one script entry point that records a Pageview and displays the Public Map, and a Separated Integration, which exposes tracking and map display independently. Both modes reuse the same collection and rendering primitives; the map request never records a Pageview because browser or proxy caching, missing Browser Visitor IDs, and image retries would otherwise make counting dependent on rendering behavior.
