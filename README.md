# VisitorTrace · 访迹

A tiny self-hosted visitor map and pageview tracker.

VisitorTrace is a lightweight, self-hosted service for recording Pageviews, maintaining aggregate visitor statistics, and publishing embeddable visitor maps for personal websites.

The project is currently in the architecture and early implementation stage.

## Documentation

- [Architecture](./ARCHITECTURE.md)
- [Domain language](./CONTEXT.md)
- [Architecture decisions](./docs/adr/)
- [Academic homepage and MapMyVisitors research](./docs/research/academic-homepage-and-mapmyvisitors.md)

## Planned Runtime

- One Go service and executable
- SQLite as the sole durable datastore
- Local IP geolocation
- Script-free SVG Public Maps
- Integrated Widget and Separated Integration modes
- Public Analytics and a password-protected Admin Console

