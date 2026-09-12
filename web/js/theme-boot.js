(function () {
  var allowed = { night: 1, deco: 1, cyber: 1, clean: 1 };
  function apply(t) {
    t = allowed[t] ? t : "deco";
    document.documentElement.setAttribute("data-theme", t);
    try {
      localStorage.setItem("speakeasy-theme", t);
    } catch (e) {}
    return t;
  }
  try {
    apply(localStorage.getItem("speakeasy-theme") || "deco");
  } catch (e) {
    apply("deco");
  }
  if (typeof fetch !== "function") {
    window.__speakeasyThemeReady = Promise.resolve();
    return;
  }
  window.__speakeasyThemeReady = fetch("/api/mesh/theme", { headers: { Accept: "application/json" } })
    .then(function (r) {
      return r.ok ? r.json() : null;
    })
    .then(function (data) {
      var t = data && (data.theme || data.Theme);
      if (t) apply(t);
    })
    .catch(function () {});
})();
