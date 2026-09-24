// VisitorTrace application navigation.
//
// Progressively enhances same-origin link clicks and GET form submissions into
// fetch-based body swaps so browsing state (scroll position, history) survives
// without full page reloads. POST submissions, non-HTML targets, and browsers
// without the required APIs keep the native full-page behaviour.
//
// Contract with page scripts: before every swap the navigator dispatches
// "vt:before-swap" on the document. Scripts that attach window/document-level
// resources must listen once and release them there.

(function () {
  "use strict";

  if (window.__vtNavInstalled) return;
  if (!window.fetch || !window.DOMParser || !window.AbortController || typeof window.CustomEvent !== "function") return;
  window.__vtNavInstalled = true;

  window.history.scrollRestoration = "manual";

  var currentURL = window.location.href;
  var activeRequest = null;
  var scrollStampTimer = null;
  var lastTriggerHref = null;

  function absoluteURL(value) {
    return new URL(value, window.location.href);
  }

  function staticAsset(url) {
    return /\.(?:csv|json|svg|txt|zip)(?:$|[?])/i.test(url.pathname);
  }

  function navigable(url) {
    return (url.protocol === "http:" || url.protocol === "https:") && url.origin === window.location.origin && !staticAsset(url);
  }

  function optedOut(element) {
    return Boolean(element.closest("[data-vt-nav-off]"));
  }

  function sameDocument(url) {
    return url.pathname === window.location.pathname && url.search === window.location.search;
  }

  function differsOnlyByHash(from, to) {
    return from.pathname === to.pathname && from.search === to.search;
  }

  // Keep the current history entry's scroll offset fresh so back/forward can
  // restore it. Debounced because browsers rate-limit replaceState calls.
  function stampScrollNow() {
    scrollStampTimer = null;
    var state = window.history.state;
    if (state && typeof state.vtScroll === "number" && state.vtScroll === window.scrollY) return;
    window.history.replaceState({ vtScroll: window.scrollY }, "", window.location.href);
  }

  function stampScroll() {
    if (scrollStampTimer === null) scrollStampTimer = window.setTimeout(stampScrollNow, 250);
  }

  function flushScrollStamp() {
    if (scrollStampTimer === null) return;
    window.clearTimeout(scrollStampTimer);
    stampScrollNow();
  }

  window.addEventListener("scroll", stampScroll, { passive: true });

  function requestDocument(target, init, options) {
    if (activeRequest) activeRequest.abort();
    var request = new AbortController();
    activeRequest = request;
    init.signal = request.signal;
    init.credentials = "same-origin";
    init.headers = { Accept: "text/html" };
    window.fetch(target, init)
      .then(function (response) {
        if (request.signal.aborted) return;
        // Rendered error pages (403 CSRF, 400 form, 404, ...) are valid HTML:
        // swap them in place so the failure is diagnosed instead of masked.
        var contentType = response.headers.get("Content-Type") || "";
        if (contentType.indexOf("text/html") === -1) throw new Error("non-HTML response");
        var finalURL = absoluteURL(response.url || target);
        // A redirected fetch would silently swap a login page into the admin
        // layout after a session expires; fall back to a real navigation.
        if (response.redirected && finalURL.pathname.indexOf("/admin/login") !== -1 && window.location.pathname.indexOf("/admin") === 0) {
          window.location.href = finalURL.href;
          return;
        }
        return response.text().then(function (html) {
          if (request.signal.aborted) return;
          var merged = options;
          if (init.method === "POST") {
            if (!response.redirected) {
              // A direct POST response (rendered error page) must not put the
              // POST-only URL into the address bar; keep the current entry.
              merged = Object.assign({}, options, { history: "replace", url: window.location.href });
            } else if (options.history === "push" && finalURL.href === window.location.href) {
              // A POST landing back on the same URL replaces the entry
              // instead of piling up duplicates.
              merged = Object.assign({}, options, { history: "replace" });
            }
          }
          applyDocument(html, finalURL, merged);
        });
      })
      .catch(function () {
        if (request.signal.aborted || request !== activeRequest) return;
        // A POST target cannot be replayed over GET; reload the current page.
        if (init.method === "POST") window.location.reload();
        else window.location.href = target;
      });
  }

  function goTo(url, options) {
    requestDocument(url.href, {}, options);
  }

  function applyDocument(html, finalURL, options) {
    var incoming = new DOMParser().parseFromString(html, "text/html");
    if (incoming.title) document.title = incoming.title;
    document.documentElement.lang = incoming.documentElement.lang;
    syncStylesheets(incoming);

    document.dispatchEvent(new CustomEvent("vt:before-swap"));

    // Adopt the rendered body and re-create <script> elements: imported
    // scripts never execute on their own, so page bootstrapping re-runs.
    var body = document.body;
    var previousActive = document.activeElement;
    var triggerHref = lastTriggerHref;
    lastTriggerHref = null;
    body.className = incoming.body.className;
    body.replaceChildren();
    Array.prototype.forEach.call(incoming.body.childNodes, function (node) {
      body.appendChild(document.importNode(node, true));
    });
    rerunScripts(body);

    var scroll = typeof options.scroll === "number" ? options.scroll : 0;
    var historyURL = options.url || finalURL.href;
    if (options.history === "push") window.history.pushState({ vtScroll: scroll }, "", historyURL);
    else window.history.replaceState({ vtScroll: scroll }, "", historyURL);
    currentURL = historyURL;

    var anchor = finalURL.hash ? document.getElementById(finalURL.hash.slice(1)) : null;
    if (anchor) anchor.scrollIntoView();
    else window.scrollTo(0, scroll);
    restoreFocus(previousActive, triggerHref);
  }

  // Keep keyboard and screen-reader position across a swap: return to the
  // previously focused element when the new document has one with the same
  // id, otherwise to the link that triggered the navigation.
  function restoreFocus(previousActive, triggerHref) {
    var target = null;
    if (previousActive && previousActive.id) target = document.getElementById(previousActive.id);
    if (!target && triggerHref) {
      var anchors = document.getElementsByTagName("a");
      for (var index = 0; index < anchors.length; index += 1) {
        if (anchors[index].href === triggerHref) {
          target = anchors[index];
          break;
        }
      }
    }
    if (target) target.focus({ preventScroll: true });
  }

  function rerunScripts(root) {
    Array.prototype.forEach.call(root.querySelectorAll("script"), function (existing) {
      var fresh = document.createElement("script");
      Array.prototype.forEach.call(existing.attributes, function (attribute) {
        fresh.setAttribute(attribute.name, attribute.value);
      });
      fresh.textContent = existing.textContent;
      existing.parentNode.replaceChild(fresh, existing);
    });
  }

  function syncStylesheets(incoming) {
    var hrefOf = function (link) { return absoluteURL(link.getAttribute("href")).href; };
    var incomingLinks = Array.prototype.slice.call(incoming.querySelectorAll('link[rel="stylesheet"]'));
    var incomingHrefs = incomingLinks.map(hrefOf);
    Array.prototype.slice.call(document.querySelectorAll('head link[rel="stylesheet"]')).forEach(function (link) {
      if (incomingHrefs.indexOf(hrefOf(link)) === -1) link.remove();
    });
    var currentHrefs = Array.prototype.slice.call(document.querySelectorAll('head link[rel="stylesheet"]')).map(hrefOf);
    incomingLinks.forEach(function (link, index) {
      if (currentHrefs.indexOf(incomingHrefs[index]) === -1) document.head.appendChild(document.importNode(link, true));
    });
  }

  document.addEventListener("click", function (event) {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    if (typeof event.target.closest !== "function") return;
    var anchor = event.target.closest("a[href]");
    if (!anchor || optedOut(anchor)) return;
    if (anchor.target && anchor.target !== "_self") return;
    if (anchor.hasAttribute("download")) return;
    if (anchor.relList && anchor.relList.contains("external")) return;
    var url = absoluteURL(anchor.href);
    if (!navigable(url) || sameDocument(url)) return;
    // In-page section anchors survive query changes on the same path.
    if (!url.hash && window.location.hash && url.pathname === window.location.pathname) url.hash = window.location.hash;
    event.preventDefault();
    flushScrollStamp();
    lastTriggerHref = url.href;
    var samePath = url.pathname === window.location.pathname;
    goTo(url, { history: "push", scroll: samePath ? window.scrollY : 0 });
  });

  document.addEventListener("submit", function (event) {
    // Page scripts may cancel a submission (confirmation dialogs); respect it.
    if (event.defaultPrevented) return;
    var form = event.target;
    if (!(form instanceof HTMLFormElement) || optedOut(form)) return;
    var method = (form.getAttribute("method") || "get").toLowerCase();
    var action = absoluteURL(form.getAttribute("action") || window.location.href);
    if (!navigable(action)) return;
    if (method === "get") {
      var params = new URLSearchParams();
      new FormData(form).forEach(function (value, key) {
        if (typeof value === "string" && value !== "") params.append(key, value);
      });
      action.hash = "";
      action.search = params.toString();
      event.preventDefault();
      flushScrollStamp();
      goTo(action, { history: "push", scroll: window.scrollY });
    } else if (form.hasAttribute("data-vt-nav")) {
      // Opt-in POST submissions follow the PRG redirect and swap in place;
      // forms that restart the process or upload files stay full-page.
      // Send url-encoded like a native form; a URLSearchParams body makes
      // fetch set that Content-Type automatically.
      event.preventDefault();
      flushScrollStamp();
      var body = new URLSearchParams();
      new FormData(form).forEach(function (value, key) {
        if (typeof value === "string") body.append(key, value);
      });
      requestDocument(action.href, { method: "POST", body: body }, { history: "push", scroll: window.scrollY });
    }
  });

  window.addEventListener("popstate", function (event) {
    var target = absoluteURL(window.location.href);
    if (differsOnlyByHash(absoluteURL(currentURL), target)) {
      // Same-document hash navigation was handled natively.
      currentURL = target.href;
      if (event.state && typeof event.state.vtScroll === "number") window.scrollTo(0, event.state.vtScroll);
      return;
    }
    var scroll = event.state && typeof event.state.vtScroll === "number" ? event.state.vtScroll : 0;
    goTo(target, { history: "replace", scroll: scroll });
  });
}());
