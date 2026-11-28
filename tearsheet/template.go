package tearsheet

import "html/template"

// pageTemplate is the tear sheet.
//
// The styles and the script are inline: the page is one file that opens
// anywhere, with nothing to fetch and nothing to serve.
var pageTemplate = template.Must(template.New("tearsheet").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root {
  color-scheme: light;
  --page: #f9f9f7;
  --surface-1: #fcfcfb;
  --text-primary: #0b0b0b;
  --text-secondary: #52514e;
  --text-muted: #898781;
  --grid: #e1e0d9;
  --base: #c3c2b7;
  --border: rgba(11, 11, 11, 0.10);
  --series-1: #2a78d6;
  --neg: #e34948;
  --delta-up: #006300;
  --delta-down: #d03b3b;
  --shadow: 0 1px 2px rgba(11, 11, 11, 0.04);
}
@media (prefers-color-scheme: dark) {
  :root:where(:not([data-theme="light"])) {
    color-scheme: dark;
    --page: #0d0d0d;
    --surface-1: #1a1a19;
    --text-primary: #f0efec;
    --text-secondary: #c3c2b7;
    --text-muted: #898781;
    --grid: #2c2c2a;
    --base: #383835;
    --border: rgba(255, 255, 255, 0.10);
    --series-1: #3987e5;
    --neg: #e66767;
    --delta-up: #0ca30c;
    --delta-down: #e66767;
    --shadow: none;
  }
}
:root[data-theme="dark"] {
  color-scheme: dark;
  --page: #0d0d0d;
  --surface-1: #1a1a19;
  --text-primary: #f0efec;
  --text-secondary: #c3c2b7;
  --text-muted: #898781;
  --grid: #2c2c2a;
  --base: #383835;
  --border: rgba(255, 255, 255, 0.10);
  --series-1: #3987e5;
  --neg: #e66767;
  --delta-up: #0ca30c;
  --delta-down: #e66767;
  --shadow: none;
}

* { box-sizing: border-box; }

body {
  margin: 0;
  padding: 32px 20px 72px;
  background: var(--page);
  color: var(--text-primary);
  font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
  font-size: 14px;
  line-height: 1.5;
}

.viz-root { max-width: 1040px; margin: 0 auto; }

header { margin-bottom: 28px; }
.eyebrow {
  font-size: 12px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-muted);
}
h1 {
  margin: 6px 0 4px;
  font-size: 24px;
  font-weight: 600;
  letter-spacing: -0.01em;
}
.sub { margin: 0; color: var(--text-secondary); font-size: 13px; }

.hero { margin: 0 0 24px; }
.hero .label { color: var(--text-secondary); font-size: 13px; }
.hero .value {
  font-size: 52px;
  font-weight: 600;
  letter-spacing: -0.02em;
  line-height: 1.1;
}
.hero .note { color: var(--text-muted); font-size: 13px; }

.tiles {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 1px;
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: 10px;
  overflow: hidden;
  margin-bottom: 24px;
}
.tile {
  background: var(--surface-1);
  box-shadow: 0 0 0 1px var(--border);
  padding: 14px 16px;
}
.tile .label { color: var(--text-secondary); font-size: 12px; }
.tile .value {
  font-size: 18px;
  font-weight: 600;
  margin-top: 2px;
  letter-spacing: -0.01em;
}
.tile .note { color: var(--text-muted); font-size: 12px; }

.card {
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 20px 20px 16px;
  margin-bottom: 20px;
  box-shadow: var(--shadow);
}
.card h2 {
  margin: 0 0 2px;
  font-size: 15px;
  font-weight: 600;
}
.card .caption {
  margin: 0 0 14px;
  color: var(--text-secondary);
  font-size: 12.5px;
}
.card h3 {
  margin: 22px 0 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
}

.chart { position: relative; }
.chart svg { display: block; width: 100%; height: auto; }

.grid { stroke: var(--grid); stroke-width: 1; }
.base { stroke: var(--base); stroke-width: 1; }
.tick { fill: var(--text-muted); font-size: 11px; font-variant-numeric: tabular-nums; }
.hair { stroke: var(--base); stroke-width: 1; }
.hair-dot { fill: var(--series-1); stroke: var(--surface-1); stroke-width: 2; }
.dd-dot { fill: var(--neg); }

.eq-line, .dd-line {
  fill: none;
  stroke: var(--series-1);
  stroke-width: 2;
  stroke-linejoin: round;
  stroke-linecap: round;
}
.dd-line { stroke: var(--neg); }
.dd-area { fill: var(--neg); fill-opacity: 0.10; stroke: none; }
.eq-dot { fill: var(--series-1); stroke: var(--surface-1); stroke-width: 2; }
.eq-end { fill: var(--text-primary); font-size: 12px; font-weight: 600; }

.bar.pos { fill: var(--series-1); }
.bar.neg { fill: var(--neg); }
.bar { cursor: pointer; stroke: none; }
.bar:hover { opacity: 0.75; }
.bar:focus-visible { stroke: var(--text-primary); stroke-width: 2; }

.tip {
  position: absolute;
  transform: translate(-50%, -120%);
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 7px 10px;
  font-size: 12px;
  pointer-events: none;
  white-space: nowrap;
  box-shadow: 0 2px 8px rgba(11, 11, 11, 0.12);
  z-index: 2;
}
.tip .tipv { font-weight: 600; color: var(--text-primary); }
.tip .tipl { color: var(--text-secondary); }

table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12.5px;
  font-variant-numeric: tabular-nums;
}
th {
  text-align: left;
  font-weight: 500;
  color: var(--text-muted);
  border-bottom: 1px solid var(--base);
  padding: 6px 10px 6px 0;
  white-space: nowrap;
}
td {
  padding: 6px 10px 6px 0;
  border-bottom: 1px solid var(--grid);
  color: var(--text-secondary);
  white-space: nowrap;
}
td.num { text-align: right; padding-right: 0; }
th.num { text-align: right; padding-right: 0; }
td.sym { color: var(--text-primary); font-weight: 500; }
.up { color: var(--delta-up); }
.down { color: var(--delta-down); }
.scroll { max-height: 460px; overflow-y: auto; }
table.compact { width: auto; }

.empty {
  color: var(--text-muted);
  font-size: 13px;
  padding: 18px 0;
}
footer {
  color: var(--text-muted);
  font-size: 12px;
  margin-top: 28px;
}
</style>
</head>
<body>
<main class="viz-root">

<header>
  <div class="eyebrow">{{.Mode}}</div>
  <h1>{{.Title}}</h1>
  <p class="sub">{{.Subtitle}}</p>
</header>

<section class="hero">
  <div class="label">{{.HeroLabel}}</div>
  <div class="value">{{.HeroValue}}</div>
  <div class="note">{{.HeroNote}}</div>
</section>

<section class="tiles">
  {{range .Tiles}}<div class="tile">
    <div class="label">{{.Label}}</div>
    <div class="value">{{.Value}}</div>
    {{if .Note}}<div class="note">{{.Note}}</div>{{end}}
  </div>{{end}}
</section>

{{if .HasEquity}}
<section class="card">
  <h2>Portfolio value</h2>
  <p class="caption">The account's value at the close of each trading day.</p>
  <div class="chart" id="c-equity">{{.EquitySVG}}<div class="tip" hidden></div></div>
</section>

<section class="card">
  <h2>Drawdown</h2>
  <p class="caption">The fall from the running peak of the value above.</p>
  <div class="chart" id="c-drawdown">{{.DrawdownSVG}}<div class="tip" hidden></div></div>
</section>
{{end}}

{{if .HasMonthly}}
<section class="card">
  <h2>Monthly returns</h2>
  <p class="caption">Each month's return. Months above the line gained; months below it lost.</p>
  <div class="chart" id="c-monthly">{{.MonthlySVG}}<div class="tip" hidden></div></div>
  <h3>Monthly returns, in full</h3>
  <div class="scroll">
    <table class="compact">
      <thead><tr>{{range .MonthlyHead}}<th>{{.}}</th>{{end}}</tr></thead>
      <tbody>{{range .MonthlyRows}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</tbody>
    </table>
  </div>
</section>
{{end}}

{{if .HasTrades}}
<section class="card">
  <h2>Trades</h2>
  <p class="caption">Every closed trade, matched first in, first out.</p>
  <div class="scroll">
    <table>
      <thead><tr>{{range .TradeHead}}<th>{{.}}</th>{{end}}</tr></thead>
      <tbody>{{range .TradeRows}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</tbody>
    </table>
  </div>
</section>
{{end}}

<footer>Generated {{.Generated}} · {{.Mode}} · figures are net of the commission the broker charged.</footer>

</main>

<script type="application/json" id="data-equity">{{.EquityData}}</script>
<script type="application/json" id="data-drawdown">{{.DrawData}}</script>
<script>
(function () {
  function curve(chartID, dataID) {
    var chart = document.getElementById(chartID);
    if (!chart) { return; }
    var svg = chart.querySelector('svg');
    var raw = document.getElementById(dataID);
    if (!svg || !raw || !raw.textContent.trim()) { return; }

    var data = JSON.parse(raw.textContent);
    if (!data.length) { return; }

    var tip = chart.querySelector('.tip');
    var hair = svg.querySelector('.hair');
    var dot = svg.querySelector('.hair-dot');
    if (!tip || !hair || !dot) { return; }

    function show(p, scale) {
      hair.setAttribute('x1', p.x);
      hair.setAttribute('x2', p.x);
      dot.setAttribute('cx', p.x);
      dot.setAttribute('cy', p.y);
      hair.style.display = '';
      dot.style.display = '';

      tip.textContent = '';
      var value = document.createElement('div');
      value.className = 'tipv';
      value.textContent = p.value;
      var date = document.createElement('div');
      date.className = 'tipl';
      date.textContent = p.date;
      tip.appendChild(value);
      tip.appendChild(date);

      tip.style.left = (p.x * scale) + 'px';
      tip.style.top = (p.y * scale) + 'px';
      tip.hidden = false;
    }

    svg.addEventListener('pointermove', function (ev) {
      var rect = svg.getBoundingClientRect();
      var vb = svg.viewBox.baseVal;
      var scale = rect.width / vb.width;
      var x = (ev.clientX - rect.left) / scale;

      var best = data[0];
      var bestDistance = Math.abs(best.x - x);
      for (var i = 1; i < data.length; i++) {
        var d = Math.abs(data[i].x - x);
        if (d < bestDistance) { bestDistance = d; best = data[i]; }
      }
      show(best, scale);
    });

    svg.addEventListener('pointerleave', function () {
      hair.style.display = 'none';
      dot.style.display = 'none';
      tip.hidden = true;
    });
  }

  function bars(chartID) {
    var chart = document.getElementById(chartID);
    if (!chart) { return; }
    var svg = chart.querySelector('svg');
    var tip = chart.querySelector('.tip');
    if (!svg || !tip) { return; }
    var vb = svg.viewBox.baseVal;

    Array.prototype.forEach.call(svg.querySelectorAll('path.bar'), function (r) {
      function show() {
        tip.textContent = '';
        var value = document.createElement('div');
        value.className = 'tipv';
        value.textContent = r.getAttribute('data-value');
        var label = document.createElement('div');
        label.className = 'tipl';
        label.textContent = r.getAttribute('data-label');
        tip.appendChild(value);
        tip.appendChild(label);

        var rect = svg.getBoundingClientRect();
        var scale = rect.width / vb.width;
        var x = parseFloat(r.getAttribute('data-x')) + parseFloat(r.getAttribute('data-w')) / 2;
        var y = parseFloat(r.getAttribute('data-y'));
        tip.style.left = (x * scale) + 'px';
        tip.style.top = (y * scale) + 'px';
        tip.hidden = false;
      }
      function hide() { tip.hidden = true; }

      r.addEventListener('pointerenter', show);
      r.addEventListener('focus', show);
      r.addEventListener('pointerleave', hide);
      r.addEventListener('blur', hide);
    });
  }

  curve('c-equity', 'data-equity');
  curve('c-drawdown', 'data-drawdown');
  bars('c-monthly');
})();
</script>
</body>
</html>
`))
