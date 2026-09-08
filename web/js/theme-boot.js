(function () {
  try {
    var allowed = { night: 1, deco: 1, cyber: 1, clean: 1 };
    var t = localStorage.getItem("speakeasy-theme") || "deco";
    document.documentElement.setAttribute("data-theme", allowed[t] ? t : "deco");
  } catch (e) {
    document.documentElement.setAttribute("data-theme", "deco");
  }
})();
