(function () {
  var p = "system";
  try { p = localStorage.getItem("bv.theme") || "system"; } catch (e) {}
  var d = p === "dark" || (p !== "light" && matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.dataset.theme = d ? "dark" : "light";
})();