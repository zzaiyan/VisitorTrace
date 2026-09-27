// Authorization prompts for actions that need confirmation or a fresh password.
// The server always validates the CSRF token, password, and target again.
(function () {
  "use strict";

  if (!window.fetch || !window.HTMLDialogElement || !window.FormData) return;
  if (window.__vtAuthDialogInstalled) return;
  window.__vtAuthDialogInstalled = true;

  var pending = null;
  var dialog = null;
  var resolving = false;

  function labels() { return document.body.dataset; }
  function authAttr(form, name) { return form.getAttribute("data-auth-" + name) || ""; }

  function login() {
    var url = new URL(labels().authLoginUrl, window.location.href);
    url.searchParams.set("next", window.location.pathname + window.location.search + window.location.hash);
    window.location.assign(url.href);
  }

  function context() {
    return fetch(labels().authContextUrl, { credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json" } })
      .then(function (response) {
        if (response.status === 401) { login(); throw new Error("session"); }
        if (!response.ok) throw new Error("context");
        return response.json();
      });
  }

  function putField(form, name, value) {
    var field = form.querySelector('input[type="hidden"][name="' + name + '"]');
    if (!field) {
      field = document.createElement("input");
      field.type = "hidden";
      field.name = name;
      form.appendChild(field);
    }
    field.value = value;
  }

  function submit(form) {
    // Native submission preserves uploads and restart responses. The browser
    // has already run constraint validation before the submit event fired.
    HTMLFormElement.prototype.submit.call(form);
  }

  function ensureDialog() {
    if (dialog) return dialog;
    dialog = document.createElement("dialog");
    dialog.className = "auth-dialog";
    var form = document.createElement("form");
    form.method = "dialog";
    form.innerHTML = '<h3 data-auth-heading></h3><p class="muted" data-auth-description></p>' +
      '<label class="field" data-auth-target-field><span data-auth-target-label></span><code data-auth-target-value></code><input type="text" autocomplete="off" spellcheck="false" data-auth-target-input></label>' +
      '<label class="field" data-auth-password-field><span data-auth-password-label></span><input type="password" autocomplete="current-password" minlength="8" maxlength="128" data-auth-password-input></label>' +
      '<p class="notice danger" role="alert" data-auth-error hidden></p>' +
      '<div class="auth-dialog-actions"><button type="button" class="button ghost" data-auth-cancel></button><button type="submit" class="button primary" data-auth-submit></button></div>';
    dialog.appendChild(form);
    document.documentElement.appendChild(dialog);
    form.querySelector("[data-auth-cancel]").addEventListener("click", function () { pending = null; dialog.close(); });
    dialog.addEventListener("close", function () { if (!dialog.open) pending = null; });
    form.addEventListener("submit", function (event) {
      event.preventDefault();
      if (!pending) return;
      var state = pending;
      var targetInput = form.querySelector("[data-auth-target-input]");
      var passwordInput = form.querySelector("[data-auth-password-input]");
      var error = form.querySelector("[data-auth-error]");
      if (state.level === "critical" && targetInput.value !== state.target) {
        error.textContent = labels().authTargetMismatch;
        error.hidden = false;
        targetInput.focus();
        return;
      }
      var button = form.querySelector("[data-auth-submit]");
      button.disabled = true;
      context().then(function (current) {
        if (pending !== state || !dialog.open) throw new Error("cancelled");
        putField(state.form, "csrf", current.csrf);
        if (state.level === "confirm") return;
        var body = new URLSearchParams({ csrf: current.csrf, password: passwordInput.value });
        return fetch(labels().authVerifyUrl, {
          method: "POST", credentials: "same-origin", cache: "no-store", body: body,
          headers: { Accept: "application/json" }
        }).then(function (response) {
          if (response.status === 401) { login(); throw new Error("session"); }
          if (response.status === 403 && response.headers.get("Vt-Auth") === "step-up") throw new Error("password");
          if (!response.ok) throw new Error("verify");
        });
      }).then(function () {
        if (pending !== state || !dialog.open) throw new Error("cancelled");
        if (state.level === "critical") putField(state.form, state.confirmName, state.target);
        if (state.level !== "confirm") putField(state.form, "password", passwordInput.value);
        dialog.close();
        submit(state.form);
      }).catch(function (reason) {
        if (reason.message === "session" || reason.message === "cancelled" || pending !== state) return;
        error.textContent = reason.message === "password" ? labels().authPasswordInvalid : labels().authRequestFailed;
        error.hidden = false;
        if (reason.message === "password") { passwordInput.value = ""; passwordInput.focus(); }
      }).finally(function () { button.disabled = false; });
    });
    return dialog;
  }

  function open(form, level) {
    var root = ensureDialog();
    var target = level === "critical" ? authAttr(form, "target") : "";
    if (level === "critical" && authAttr(form, "target-source")) {
      var source = form.querySelector(authAttr(form, "target-source"));
      target = source ? source.value : "";
    }
    pending = { form: form, level: level, target: target, confirmName: authAttr(form, "confirm-name") };
    var ui = root.querySelector("form");
    ui.querySelector("[data-auth-heading]").textContent = authAttr(form, "title") || labels().authConfirmTitle;
    ui.querySelector("[data-auth-description]").textContent = authAttr(form, "message");
    ui.querySelector("[data-auth-target-label]").textContent = authAttr(form, "target-label") || labels().authTargetLabel;
    ui.querySelector("[data-auth-target-value]").textContent = target;
    ui.querySelector("[data-auth-password-label]").textContent = labels().stepupPassword;
    ui.querySelector("[data-auth-cancel]").textContent = labels().stepupCancel;
    ui.querySelector("[data-auth-submit]").textContent = level === "confirm" ? labels().authConfirmTitle : labels().stepupSubmit;
    ui.querySelector("[data-auth-submit]").hidden = false;
    ui.querySelector("[data-auth-target-field]").hidden = level !== "critical";
    ui.querySelector("[data-auth-password-field]").hidden = level === "confirm";
    var targetInput = ui.querySelector("[data-auth-target-input]");
    var passwordInput = ui.querySelector("[data-auth-password-input]");
    targetInput.required = level === "critical";
    passwordInput.required = level !== "confirm";
    targetInput.value = "";
    passwordInput.value = "";
    ui.querySelector("[data-auth-error]").hidden = true;
    root.showModal();
    (level === "critical" ? targetInput : level === "password" ? passwordInput : ui.querySelector("[data-auth-submit]")).focus();
  }

  function showUnavailable() {
    var root = ensureDialog();
    var ui = root.querySelector("form");
    pending = null;
    ui.querySelector("[data-auth-heading]").textContent = labels().authRequestFailed;
    ui.querySelector("[data-auth-description]").textContent = labels().authUnavailable;
    ui.querySelector("[data-auth-target-field]").hidden = true;
    ui.querySelector("[data-auth-password-field]").hidden = true;
    ui.querySelector("[data-auth-error]").hidden = true;
    ui.querySelector("[data-auth-submit]").hidden = true;
    ui.querySelector("[data-auth-cancel]").textContent = labels().stepupCancel;
    root.showModal();
    ui.querySelector("[data-auth-cancel]").focus();
  }

  document.addEventListener("submit", function (event) {
    var form = event.target;
    if (!(form instanceof HTMLFormElement) || !form.hasAttribute("data-auth-level")) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    if (resolving) return;
    resolving = true;
    var level = authAttr(form, "level");
    context().then(function (current) {
      putField(form, "csrf", current.csrf);
      if (level === "password" && current.stepUpActive) submit(form);
      else open(form, level);
    }).catch(function (reason) {
      if (reason.message !== "session") showUnavailable();
    }).finally(function () {
      resolving = false;
    });
  }, true);
}());
