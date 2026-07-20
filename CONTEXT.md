# VisitorTrace / 访迹

This context defines the language used by VisitorTrace (访迹) to describe traffic observed across a small set of independently managed personal websites.

## Language

**Site**:
An independently configured website whose observations and statistics are isolated from every other Site and follow its own calendar timezone. Its timezone becomes fixed once the Site has accepted a Pageview and can change only after its observations and statistics are reset.
_Avoid_: Project, property, account

**Site ID**:
A public identifier that associates tracking reports and public views with one Site. It is not a secret or proof of authenticity and is never reassigned to another Site.
_Avoid_: API key, secret, access token

**Allowed Origin**:
A browser origin configured for a Site as an accepted source of tracking reports. It limits ordinary misuse but does not prove that a determined sender is authentic.
_Avoid_: Trusted client, authenticated origin

**Pageview**:
One accepted report that a page belonging to a Site was viewed.
_Avoid_: Visit, hit, visitor

**Pageview Record**:
A temporarily retained, individual record of one Pageview. It remains distinct from the Aggregate derived from it.
_Avoid_: Visitor profile, permanent history

**Aggregate**:
A durable statistical summary derived from Pageviews that remains valid after its source Pageview Records expire.
_Avoid_: Raw data, visitor log

**Retention Period**:
The Site-configured length of time a Pageview Record remains available before automatic deletion. It does not limit the lifetime of Aggregates.
_Avoid_: Deduplication Window, archive period

**Unique Visitor**:
An estimated visitor identity observed by one Site during one Deduplication Window. It is not a verified person and is never shared across Sites. Counts spanning multiple windows are the sum of the window counts and are not deduplicated again.
_Avoid_: User, real person, global visitor

**Browser Visitor ID**:
A random, Site-scoped value maintained by a visitor's browser and reported with Pageviews to improve Unique Visitor estimation. It is not shared between Sites and may disappear when the visitor clears browser data.
_Avoid_: User ID, account ID, third-party cookie

**Visitor Digest**:
A Site-scoped, non-reversible representation derived from a Browser Visitor ID or fallback observations and used to recognize a Unique Visitor within a Deduplication Window.
_Avoid_: Browser Visitor ID, raw fingerprint, global identifier

**Deduplication Window**:
A Site-configured period of one through thirty calendar days during which repeated Pageviews from the same estimated visitor count as one Unique Visitor. It starts at midnight in the Site's timezone and defaults to one day. A configuration change applies only to future windows and does not redefine completed windows.
_Avoid_: Visitor merge period, session

**Public Map**:
An embeddable, aggregate-only representation of one Site's geographic distribution, Pageviews, and Unique Visitors.
_Avoid_: Visitor log, analytics dashboard

**Map Preset**:
The Administrator-defined default presentation of a Site's Public Map, including its dimensions, visible labels, and typography. An embed may override supported values without changing the Map Preset.
_Avoid_: Embed override, Site settings

**Integrated Widget**:
A public integration mode in which one embedded widget workflow records a Pageview and displays the Site's Public Map together. It is a single integration experience, not a separate counting model or a secret-bearing client.
_Avoid_: Map API, authenticated widget

**Separated Integration**:
A public integration mode in which Pageview tracking and Public Map display are installed independently, so a Site can track without a map or load the map later.
_Avoid_: Partial widget, secondary tracker

**Public Analytics**:
An unauthenticated, aggregate-only view of non-sensitive traffic statistics for one Site.
_Avoid_: Visitor version, public backend, visitor feed

**Admin Console**:
A password-protected view of all Pageview Records, Aggregates, and settings across managed Sites.
_Avoid_: Admin version, public analytics

**Administrator**:
The sole trusted operator who can use the Admin Console and manage every Site.
_Avoid_: User, account, role, Site owner

## Example Dialogue

> **Site owner:** This Site received five Pageviews today. How many Unique Visitors were there?
>
> **Developer:** Three within its current Deduplication Window. Visits to your other Site are counted independently.
>
> **Site owner:** Can the Public Map show which pages one visitor opened?
>
> **Developer:** No. The Public Map and Public Analytics expose aggregates, not an individual's browsing sequence. Only the Admin Console can access Pageview Records.
>
> **Site owner:** What happens when Pageview Records reach their Retention Period?
>
> **Developer:** The records are deleted, while their Aggregates remain available.
>
> **Site owner:** I want one MapMyVisitors-like embed. Which integration should I use?
>
> **Developer:** Use the Integrated Widget. If the map should be lazy-loaded or omitted on some pages, use Separated Integration with the tracker and Public Map installed independently.
