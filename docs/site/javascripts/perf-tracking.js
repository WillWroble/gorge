// Renders perf/history.jsonl (copied from the perf-data branch at build
// time) as one Chart.js chart per scenario/metric on the tracking page.
// Loaded on every page via extra_javascript, so it does nothing unless the
// page has #perf-charts. Chart.js is fetched lazily, pinned, from cdnjs.
(function () {
  var CHART_JS = "https://cdnjs.cloudflare.com/ajax/libs/Chart.js/4.4.1/chart.umd.min.js";
  var chartLoad = null;

  function loadChart() {
    if (window.Chart) return Promise.resolve();
    if (!chartLoad) {
      chartLoad = new Promise(function (resolve, reject) {
        var s = document.createElement("script");
        s.src = CHART_JS;
        s.onload = resolve;
        s.onerror = function () { reject(new Error("could not load Chart.js")); };
        document.head.appendChild(s);
      });
    }
    return chartLoad;
  }

  function historyURL() {
    var base = ".";
    try { base = JSON.parse(document.getElementById("__config").textContent).base; } catch (e) {}
    return new URL(base.replace(/\/?$/, "/") + "perf/history.jsonl", location.href).href;
  }

  function parse(text) {
    var rows = [];
    text.split("\n").forEach(function (line) {
      line = line.trim();
      if (!line) return;
      try {
        var r = JSON.parse(line);
        if (r && r.scenario && r.metric && typeof r.ratio_to_anchor === "number") rows.push(r);
      } catch (e) { /* skip malformed lines */ }
    });
    return rows;
  }

  function cssVar(name, fallback) {
    var v = getComputedStyle(document.body).getPropertyValue(name).trim();
    return v || fallback;
  }

  function render(root, status, rows) {
    var groups = {};
    var order = [];
    rows.forEach(function (r) {
      var key = r.scenario + " · " + r.metric;
      if (!groups[key]) { groups[key] = []; order.push(key); }
      groups[key].push(r);
    });
    order.sort();
    var fg = cssVar("--md-default-fg-color--light", "#666");
    var grid = cssVar("--md-default-fg-color--lightest", "rgba(127,127,127,0.2)");
    var accent = cssVar("--md-accent-fg-color", "#e65100");
    status.textContent = rows.length + " measurements across " + order.length +
      " series. Values are ratio to anchor; 1.0 = anchor speed.";
    order.forEach(function (key) {
      var series = groups[key].slice().sort(function (a, b) {
        return String(a.run_date).localeCompare(String(b.run_date));
      });
      var last = series[series.length - 1];
      var fig = document.createElement("figure");
      fig.style.margin = "1.5em 0";
      var cap = document.createElement("figcaption");
      cap.textContent = key + (last.unit ? " (" + last.unit + ", " +
        (last.higher_is_better === false ? "lower" : "higher") + " is better)" : "");
      var canvas = document.createElement("canvas");
      canvas.setAttribute("aria-label", "ratio to anchor over time for " + key);
      fig.appendChild(canvas);
      fig.appendChild(cap);
      root.appendChild(fig);
      var labels = series.map(function (r) {
        return String(r.run_date).slice(0, 10) + " " + String(r.commit || "").slice(0, 9);
      });
      var lo = series.map(function (r) { return r.ratios && r.ratios.length ? Math.min.apply(null, r.ratios) : r.ratio_to_anchor; });
      var hi = series.map(function (r) { return r.ratios && r.ratios.length ? Math.max.apply(null, r.ratios) : r.ratio_to_anchor; });
      new window.Chart(canvas, {
        type: "line",
        data: {
          labels: labels,
          datasets: [
            { label: "min", data: lo, borderWidth: 0, pointRadius: 0, fill: false },
            { label: "max", data: hi, borderWidth: 0, pointRadius: 0, fill: "-1",
              backgroundColor: "rgba(127,127,127,0.18)" },
            { label: "ratio to anchor", data: series.map(function (r) { return r.ratio_to_anchor; }),
              borderColor: accent, backgroundColor: accent, borderWidth: 2, pointRadius: 3, tension: 0 }
          ]
        },
        options: {
          animation: false,
          plugins: {
            legend: { display: false },
            tooltip: {
              filter: function (item) { return item.datasetIndex === 2; },
              callbacks: {
                afterLabel: function (ctx) {
                  var r = series[ctx.dataIndex];
                  return "anchor " + (r.anchor || "?") + ", value " + r.value + (r.unit ? " " + r.unit : "");
                }
              }
            }
          },
          scales: {
            x: { ticks: { color: fg, maxRotation: 60 }, grid: { color: grid } },
            y: { ticks: { color: fg }, grid: { color: grid }, title: { display: true, text: "ratio to anchor", color: fg } }
          }
        }
      });
    });
  }

  function init() {
    var root = document.getElementById("perf-charts");
    if (!root || root.dataset.rendered) return;
    root.dataset.rendered = "1";
    var status = document.getElementById("perf-status");
    fetch(historyURL(), { cache: "no-cache" })
      .then(function (res) { return res.ok ? res.text() : ""; })
      .catch(function () { return ""; })
      .then(function (text) {
        var rows = parse(text);
        if (!rows.length) {
          status.innerHTML = "<strong>No data yet.</strong> The <code>perf-data</code> branch has not " +
            "published any measurements; the record schema below describes what will appear here.";
          return;
        }
        return loadChart().then(function () { render(root, status, rows); });
      })
      .catch(function (err) { status.textContent = "Could not render charts: " + err.message; });
  }

  if (window.document$ && typeof window.document$.subscribe === "function") {
    window.document$.subscribe(init); // Material instant navigation
  } else {
    document.addEventListener("DOMContentLoaded", init);
  }
})();
