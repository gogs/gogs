// wwads.cn: above table of contents. Carbon Ads: below table of contents.
(function () {
  "use strict";

  var CARBON_ID = "gogs-carbon-ad";
  var WWADS_ID = "gogs-wwads";

  // Load the wwads.cn global script (skipped on localhost — ad unit
  // 97 is registered for gogs.io and the API rejects other origins).
  var isProduction = location.hostname !== "localhost";
  if (isProduction) {
    var wwScript = document.createElement("script");
    wwScript.src = "https://cdn.wwads.cn/js/makemoney.js";
    wwScript.async = true;
    document.head.appendChild(wwScript);
  }

  function injectCarbonAd() {
    if (document.getElementById(CARBON_ID)) return;

    var toc = document.getElementById("table-of-contents");
    if (!toc) return;

    var container = document.createElement("div");
    container.id = CARBON_ID;

    var carbon = document.createElement("script");
    carbon.async = true;
    carbon.type = "text/javascript";
    carbon.src =
      "//cdn.carbonads.com/carbon.js?serve=CKYILK3U&placement=gogsio";
    carbon.id = "_carbonads_js";
    container.appendChild(carbon);

    toc.appendChild(container);
  }

  function injectWwads() {
    if (document.getElementById(WWADS_ID)) return;

    var toc = document.getElementById("table-of-contents");
    if (!toc) return;

    var container = document.createElement("div");
    container.id = WWADS_ID;

    var wwads = document.createElement("div");
    wwads.className = "wwads-cn wwads-horizontal";
    wwads.setAttribute("data-id", "97");
    container.appendChild(wwads);

    toc.insertBefore(container, toc.firstChild);
  }

  function injectAds() {
    injectCarbonAd();
    injectWwads();
  }

  // Re-inject when the content is replaced during SPA navigation.
  var debounce;
  new MutationObserver(function () {
    clearTimeout(debounce);
    debounce = setTimeout(injectAds, 200);
  }).observe(document.documentElement, { childList: true, subtree: true });

  injectAds();
})();
