(function () {
  "use strict";

  var script = document.currentScript;
  if (!script) return;

  var scriptURL = new URL(script.src, document.baseURI);
  var siteID = scriptURL.searchParams.get("site_id");
  if (!siteID) return;

  var endpoint = new URL(
    "/api/v1/sites/" + encodeURIComponent(siteID) + "/pageviews",
    scriptURL.origin
  ).href;
  var state = window.__visitorTraceState || (window.__visitorTraceState = {});
  var siteState = state[siteID] || (state[siteID] = { sent: {}, visitorID: null });

  function getVisitorID() {
    if (siteState.visitorID) return siteState.visitorID;
    var storageKey = "visitortrace:" + siteID;
    try {
      siteState.visitorID = window.localStorage.getItem(storageKey);
      if (siteState.visitorID && /^[0-9a-f]{32}$/.test(siteState.visitorID)) {
        return siteState.visitorID;
      }
    } catch (_) {}

    if (window.crypto && window.crypto.getRandomValues) {
      var bytes = new Uint8Array(16);
      window.crypto.getRandomValues(bytes);
      siteState.visitorID = Array.prototype.map.call(bytes, function (value) {
        return (value < 16 ? "0" : "") + value.toString(16);
      }).join("");
      try { window.localStorage.setItem(storageKey, siteState.visitorID); } catch (_) {}
      return siteState.visitorID;
    }
    return "";
  }

  function normalizePath(value) {
    value = value || "/";
    value = value.split("?")[0].split("#")[0];
    return value.charAt(0) === "/" ? value : "/" + value;
  }

  function send(path) {
    path = normalizePath(path);
    var key = siteID + "|" + path;
    if (siteState.sent[key]) return;
    siteState.sent[key] = true;
    var body = JSON.stringify({ path: path, visitor_id: getVisitorID() });
    var blob = new Blob([body], { type: "text/plain" });
    if (navigator.sendBeacon && navigator.sendBeacon(endpoint, blob)) return;
    if (window.fetch) {
      window.fetch(endpoint, {
        method: "POST",
        mode: "cors",
        credentials: "omit",
        headers: { "Content-Type": "text/plain" },
        body: body,
        keepalive: true
      }).catch(function () {});
    }
  }

  window.VisitorTrace = window.VisitorTrace || {};
  window.VisitorTrace.track = send;
  send(window.location.pathname);
}());
