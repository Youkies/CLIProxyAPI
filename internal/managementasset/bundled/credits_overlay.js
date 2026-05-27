(function () {
  "use strict";

  var files = [];
  var renderTimer = null;
  var observer = null;

  function norm(value) {
    return String(value || "").replace(/\s+/g, " ").trim();
  }

  function lower(value) {
    return norm(value).toLowerCase();
  }

  function numberValue(value) {
    if (typeof value === "number" && isFinite(value)) {
      return value;
    }
    if (typeof value === "string") {
      var parsed = Number(value.replace(/,/g, ""));
      if (isFinite(parsed)) {
        return parsed;
      }
    }
    return null;
  }

  function formatNumber(value) {
    var num = numberValue(value);
    if (num === null) {
      return "--";
    }
    return new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(num);
  }

  function creditsFor(file) {
    if (!file || lower(file.provider || file.type) !== "antigravity") {
      return null;
    }
    return file.ai_credits || file.aiCredits || file.antigravity_credits || file.antigravityCredits || null;
  }

  function scheduleRender() {
    if (renderTimer !== null) {
      return;
    }
    renderTimer = window.setTimeout(function () {
      renderTimer = null;
      render();
    }, 80);
  }

  function isAuthFilesURL(input) {
    var url = "";
    if (typeof input === "string") {
      url = input;
    } else if (input && typeof input.url === "string") {
      url = input.url;
    }
    if (!url) {
      return false;
    }
    return url.indexOf("/v0/management/auth-files") !== -1 &&
      url.indexOf("/v0/management/auth-files/models") === -1 &&
      url.indexOf("/v0/management/auth-files/download") === -1;
  }

  function isAPICallURL(input) {
    var url = "";
    if (typeof input === "string") {
      url = input;
    } else if (input && typeof input.url === "string") {
      url = input.url;
    }
    return !!url && url.indexOf("/v0/management/api-call") !== -1;
  }

  function parseJSON(text) {
    if (typeof text !== "string" || !text) {
      return null;
    }
    try {
      return JSON.parse(text);
    } catch (err) {
      return null;
    }
  }

  function requestBodyFromFetchArgs(args) {
    if (!args || !args[1]) {
      return "";
    }
    var body = args[1].body;
    return typeof body === "string" ? body : "";
  }

  function applyAuthFilesPayload(payload) {
    if (payload && Array.isArray(payload.files)) {
      files = payload.files;
      scheduleRender();
    }
  }

  function firstDefined() {
    for (var i = 0; i < arguments.length; i++) {
      if (arguments[i] !== undefined && arguments[i] !== null && arguments[i] !== "") {
        return arguments[i];
      }
    }
    return undefined;
  }

  function arrayAtPath(obj, paths) {
    for (var i = 0; i < paths.length; i++) {
      var current = obj;
      var parts = paths[i].split(".");
      for (var j = 0; j < parts.length && current; j++) {
        current = current[parts[j]];
      }
      if (Array.isArray(current)) {
        return current;
      }
    }
    return null;
  }

  function stringAtPath(obj, paths) {
    for (var i = 0; i < paths.length; i++) {
      var current = obj;
      var parts = paths[i].split(".");
      for (var j = 0; j < parts.length && current; j++) {
        current = current[parts[j]];
      }
      if (current !== undefined && current !== null && String(current).trim() !== "") {
        return String(current).trim();
      }
    }
    return "";
  }

  function extractCreditsFromLoadCodeAssist(bodyText) {
    var payload = parseJSON(bodyText);
    if (!payload) {
      return null;
    }
    var tier = stringAtPath(payload, ["paidTier.id", "paid_tier.id", "currentTier.id", "current_tier.id"]);
    var credits = arrayAtPath(payload, [
      "paidTier.availableCredits",
      "paidTier.available_credits",
      "paid_tier.availableCredits",
      "paid_tier.available_credits",
      "currentTier.availableCredits",
      "currentTier.available_credits",
      "current_tier.availableCredits",
      "current_tier.available_credits"
    ]);
    if (!credits) {
      return { known: true, available: false, paid_tier_id: tier, paidTierID: tier, tier_id: tier };
    }
    for (var i = 0; i < credits.length; i++) {
      var credit = credits[i] || {};
      var creditType = lower(firstDefined(credit.creditType, credit.credit_type, credit.type));
      if (creditType !== "google_one_ai") {
        continue;
      }
      var amount = numberValue(firstDefined(credit.creditAmount, credit.credit_amount, credit.amount, credit.availableAmount, credit.available_amount));
      if (amount === null) {
        continue;
      }
      var min = numberValue(firstDefined(credit.minimumCreditAmountForUsage, credit.minimum_credit_amount_for_usage, credit.minCreditAmount, credit.min_credit_amount));
      if (min === null) {
        min = 1;
      }
      return {
        known: true,
        available: amount >= min,
        credit_amount: amount,
        creditAmount: amount,
        min_credit_amount: min,
        minimumCreditAmountForUsage: min,
        minimum_credit_amount_for_usage: min,
        paid_tier_id: tier,
        paidTierID: tier,
        tier_id: tier
      };
    }
    return { known: true, available: false, paid_tier_id: tier, paidTierID: tier, tier_id: tier };
  }

  function findFileByAuthIndex(authIndex) {
    var needle = norm(authIndex);
    if (!needle) {
      return null;
    }
    for (var i = 0; i < files.length; i++) {
      if (norm(files[i].auth_index) === needle || norm(files[i].authIndex) === needle) {
        return files[i];
      }
    }
    return null;
  }

  function applyAPICallPayload(requestBody, payload) {
    var req = parseJSON(requestBody);
    if (!req || !payload || typeof payload.body !== "string") {
      return;
    }
    var url = lower(req.url);
    if (url.indexOf("loadcodeassist") === -1) {
      return;
    }
    var file = findFileByAuthIndex(firstDefined(req.auth_index, req.authIndex, req.AuthIndex));
    if (!file || lower(file.provider || file.type) !== "antigravity") {
      return;
    }
    var credits = extractCreditsFromLoadCodeAssist(payload.body);
    if (!credits) {
      return;
    }
    file.ai_credits = credits;
    file.aiCredits = credits;
    file.antigravity_credits = credits;
    file.antigravityCredits = credits;
    scheduleRender();
  }

  if (window.fetch && !window.__cliproxyAICreditsFetchPatched) {
    window.__cliproxyAICreditsFetchPatched = true;
    var nativeFetch = window.fetch.bind(window);
    window.fetch = function () {
      var args = arguments;
      var requestBody = requestBodyFromFetchArgs(args);
      return nativeFetch.apply(null, args).then(function (response) {
        try {
          if (isAuthFilesURL(args[0])) {
            response.clone().json().then(function (payload) {
              applyAuthFilesPayload(payload);
            }).catch(function () {});
          } else if (isAPICallURL(args[0])) {
            response.clone().json().then(function (payload) {
              applyAPICallPayload(requestBody, payload);
            }).catch(function () {});
          }
        } catch (err) {}
        return response;
      });
    };
  }

  if (window.XMLHttpRequest && !window.__cliproxyAICreditsXHRPatched) {
    window.__cliproxyAICreditsXHRPatched = true;
    var nativeOpen = window.XMLHttpRequest.prototype.open;
    var nativeSend = window.XMLHttpRequest.prototype.send;
    window.XMLHttpRequest.prototype.open = function (method, url) {
      this.__cliproxyAICreditsURL = url;
      return nativeOpen.apply(this, arguments);
    };
    window.XMLHttpRequest.prototype.send = function (body) {
      var xhr = this;
      var requestBody = typeof body === "string" ? body : "";
      xhr.addEventListener("loadend", function () {
        try {
          var text = xhr.responseText || "";
          if (isAuthFilesURL(xhr.__cliproxyAICreditsURL)) {
            applyAuthFilesPayload(parseJSON(text));
          } else if (isAPICallURL(xhr.__cliproxyAICreditsURL)) {
            applyAPICallPayload(requestBody, parseJSON(text));
          }
        } catch (err) {}
      });
      return nativeSend.apply(this, arguments);
    };
  }

  function candidateNeedles(file) {
    var values = [file.name, file.email, file.id, file.account].filter(Boolean);
    return values.map(lower).filter(function (value) { return value.length >= 4; });
  }

  function findCard(file) {
    var needles = candidateNeedles(file);
    if (!needles.length) {
      return null;
    }
    var best = null;
    var bestScore = Infinity;
    var nodes = document.querySelectorAll("article, section, div");
    for (var i = 0; i < nodes.length; i++) {
      var node = nodes[i];
      if (node.classList && node.classList.contains("cliproxy-ai-credits")) {
        continue;
      }
      var text = lower(node.textContent);
      if (text.indexOf("antigravity") === -1) {
        continue;
      }
      var hasNeedle = needles.some(function (needle) { return text.indexOf(needle) !== -1; });
      if (!hasNeedle) {
        continue;
      }
      if (text.indexOf("claude/gpt") === -1 && text.indexOf("gemini 3.1") === -1 && text.indexOf("ai credits") === -1) {
        continue;
      }
      var rect = node.getBoundingClientRect();
      if (rect.width < 220 || rect.height < 260) {
        continue;
      }
      if (text.length > 12000) {
        continue;
      }
      var score = rect.width * rect.height + text.length * 12;
      if (score < bestScore) {
        best = node;
        bestScore = score;
      }
    }
    return best;
  }

  function findQuotaAnchor(card) {
    var labels = [
      "gemini 3.1 flash image",
      "gemini 3 flash",
      "gemini 2.5 flash lite",
      "gemini 2.5 flash",
      "gemini 3.1 pro series",
      "claude/gpt"
    ];
    var matches = [];
    var nodes = card.querySelectorAll("div, p, span");
    for (var i = 0; i < nodes.length; i++) {
      var text = lower(nodes[i].textContent);
      if (text.length > 220) {
        continue;
      }
      if (labels.some(function (label) { return text.indexOf(label) !== -1; })) {
        matches.push(nodes[i]);
      }
    }
    var anchor = matches[matches.length - 1] || null;
    while (anchor && anchor.parentElement && anchor.parentElement !== card) {
      var rect = anchor.getBoundingClientRect();
      var parentRect = anchor.parentElement.getBoundingClientRect();
      if (parentRect.width < rect.width || parentRect.width < 160 || parentRect.height > 180) {
        break;
      }
      anchor = anchor.parentElement;
    }
    return anchor;
  }

  function upsertCredits(card, file, credits) {
    var box = card.querySelector(".cliproxy-ai-credits");
    if (!box) {
      box = document.createElement("div");
      box.className = "cliproxy-ai-credits";
      var anchor = findQuotaAnchor(card);
      if (anchor && anchor.parentElement) {
        anchor.parentElement.insertBefore(box, anchor.nextSibling);
      } else {
        card.appendChild(box);
      }
    }

    var amount = credits.credit_amount !== undefined ? credits.credit_amount : credits.creditAmount;
    var min = credits.min_credit_amount !== undefined ? credits.min_credit_amount :
      (credits.minimum_credit_amount_for_usage !== undefined ? credits.minimum_credit_amount_for_usage : credits.minimumCreditAmountForUsage);
    var tier = credits.paid_tier_id || credits.paidTierID || credits.tier_id || "";
    var available = credits.available !== false;

    box.textContent = "";
    var first = document.createElement("div");
    first.className = "cliproxy-ai-credits-line";
    var label = document.createElement("span");
    label.className = "cliproxy-ai-credits-label";
    label.textContent = "AI Credits: " + formatNumber(amount) + " / min " + formatNumber(min);
    var badge = document.createElement("span");
    badge.className = "cliproxy-ai-credits-badge" + (available ? "" : " is-unavailable");
    badge.textContent = available ? "available" : "unavailable";
    first.appendChild(label);
    first.appendChild(badge);
    box.appendChild(first);

    var meta = [file.email || file.account || "", tier].filter(Boolean).join(" | ");
    if (meta) {
      var second = document.createElement("div");
      second.className = "cliproxy-ai-credits-meta";
      second.textContent = meta;
      box.appendChild(second);
    }
  }

  function render() {
    if (!files.length) {
      return;
    }
    injectStyle();
    files.forEach(function (file) {
      var credits = creditsFor(file);
      if (!credits || credits.known === false) {
        return;
      }
      var card = findCard(file);
      if (card) {
        upsertCredits(card, file, credits);
      }
    });
  }

  function injectStyle() {
    if (document.getElementById("cliproxy-ai-credits-style")) {
      return;
    }
    var style = document.createElement("style");
    style.id = "cliproxy-ai-credits-style";
    style.textContent = [
      ".cliproxy-ai-credits{margin:14px 0 10px;padding:12px 14px;border-radius:7px;background:#eef4ff;border:1px solid rgba(94,142,222,.28);color:#29466f;font-size:14px;line-height:1.45;box-sizing:border-box;width:100%;}",
      ".cliproxy-ai-credits-line{display:flex;align-items:center;gap:10px;flex-wrap:wrap;font-weight:700;}",
      ".cliproxy-ai-credits-label{color:#2f46ff;}",
      ".cliproxy-ai-credits-badge{display:inline-flex;align-items:center;border-radius:999px;background:#d7f8df;color:#04702c;padding:2px 8px;font-weight:700;font-size:13px;}",
      ".cliproxy-ai-credits-badge.is-unavailable{background:#ffe1dc;color:#9b2c1d;}",
      ".cliproxy-ai-credits-meta{margin-top:6px;color:#4b6287;font-size:13px;font-weight:500;word-break:break-word;}"
    ].join("");
    document.head.appendChild(style);
  }

  function startObserver() {
    if (observer) {
      return;
    }
    observer = new MutationObserver(scheduleRender);
    observer.observe(document.documentElement, { childList: true, subtree: true });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", startObserver, { once: true });
  } else {
    startObserver();
  }
})();
