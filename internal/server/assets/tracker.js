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
    var hostname = window.location.hostname;
    var key = siteID + "|" + hostname + "|" + path;
    if (siteState.sent[key]) return;
    siteState.sent[key] = true;
    var body = JSON.stringify({ path: path, visitor_id: getVisitorID(), hostname: hostname });
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
    var frameURL = new URL("embed/widget", appURL);
    scriptURL.searchParams.forEach(function (value, key) {
      frameURL.searchParams.append(key, value);
    });
    var wrapper = document.createElement("span");
    wrapper.className = "visitortrace-widget";
    wrapper.style.display = "inline-block";
    wrapper.style.width = "100%";
    var frame = document.createElement("iframe");
    frame.src = frameURL.href;
    var width = parseInt(scriptURL.searchParams.get("w"), 10);
    var height = parseInt(scriptURL.searchParams.get("h"), 10);
    width = width >= 160 && width <= 1200 ? width : 720;
    height = height >= 90 && height <= 800 ? height : 400;

    function sizeFrame(nextWidth, nextHeight) {
      if (!isFinite(nextWidth) || !isFinite(nextHeight) || nextWidth < 160 || nextWidth > 1200 || nextHeight < 90 || nextHeight > 800) return;
      width = nextWidth;
      height = nextHeight;
      wrapper.style.maxWidth = width + "px";
      frame.width = width;
      frame.height = height;
      frame.style.aspectRatio = width + " / " + height;
    }

    frame.loading = "lazy";
    frame.title = "VisitorTrace interactive visitor map";
    frame.setAttribute("scrolling", "no");
    frame.setAttribute("sandbox", "allow-scripts allow-popups allow-popups-to-escape-sandbox");
    frame.style.display = "block";
    frame.style.width = "100%";
    frame.style.height = "auto";
    frame.style.border = "0";
    sizeFrame(width, height);
    window.addEventListener("message", function (event) {
      var data = event.data;
      if (event.source !== frame.contentWindow || !data || data.type !== "visitortrace:resize") return;
      sizeFrame(Number(data.width), Number(data.height));
    });
    wrapper.appendChild(frame);
    if (script.parentNode) script.parentNode.insertBefore(wrapper, script.nextSibling);
  }
  send(window.location.pathname);
}());
