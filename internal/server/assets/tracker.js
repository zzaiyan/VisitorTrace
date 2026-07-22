(function () {
  "use strict";

  var script = document.currentScript;
  if (!script) return;

  var scriptURL = new URL(script.src, document.baseURI);
	var appURL = new URL("../", scriptURL);
  var siteID = scriptURL.searchParams.get("site_id");
  if (!siteID) return;

  var endpoint = new URL(
		"api/v1/sites/" + encodeURIComponent(siteID) + "/pageviews",
		appURL
  ).href;
  var state = window.__visitorTraceState || (window.__visitorTraceState = {});
  var siteState = state[siteID] || (state[siteID] = { sent: {}, visitorID: null });
  var integrated = /\/widget\.js$/.test(scriptURL.pathname);

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
  if (integrated) {
    var mapURL = new URL(
			"api/v1/sites/" + encodeURIComponent(siteID) + "/map.svg",
			appURL
    );
    scriptURL.searchParams.forEach(function (value, key) {
      if (key !== "site_id") mapURL.searchParams.append(key, value);
    });
    var wrapper = document.createElement("span");
    wrapper.className = "visitortrace-widget";
    var link = document.createElement("a");
    link.href = new URL(
			"public/" + encodeURIComponent(siteID) + "/analytics",
			appURL
    ).href;
    link.target = "_blank";
    link.rel = "noopener";
    var image = document.createElement("img");
    image.src = mapURL.href;
    var width = parseInt(scriptURL.searchParams.get("w"), 10);
    var height = parseInt(scriptURL.searchParams.get("h"), 10);
	if (width >= 160 && width <= 1200) image.width = width;
	if (height >= 90 && height <= 800) image.height = height;
    image.loading = "eager";
    image.alt = "Visitor map";
    image.title = "VisitorTrace Public Map | IP geolocation by DB-IP";
    link.appendChild(image);
    wrapper.appendChild(link);
    if (script.parentNode) script.parentNode.insertBefore(wrapper, script.nextSibling);
  }
  send(window.location.pathname);
}());
