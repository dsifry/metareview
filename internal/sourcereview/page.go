package sourcereview

import (
	"fmt"
	"html"
	"path"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/lensoutput"
)

// Page is the input to the self-contained HTML file.
type Page struct {
	Repo     string
	Commit   string
	ModelID  string
	Findings []lensoutput.TypedFinding
	Files    map[string][]byte
}

var severities = []string{"P0", "P1", "P2", "P3"}

func severityCounts(fs []lensoutput.TypedFinding) (p0, p1, p2, p3 int) {
	for _, f := range fs {
		switch f.Severity {
		case "P0":
			p0++
		case "P1":
			p1++
		case "P2":
			p2++
		case "P3":
			p3++
		}
	}
	return p0, p1, p2, p3
}

func findingDirs(fs []lensoutput.TypedFinding) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, f := range fs {
		d := path.Dir(f.File)
		if !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs)
	return dirs
}

func excerpt(body []byte, start, end int) string {
	lines := splitLines(body)
	if start < 1 || end > len(lines) || end < start {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}

// area is the part of the tree a finding belongs to on the map: the first two
// segments of its directory ("app/models" for app/models/concerns/x.rb).
func area(file string) string {
	parts := strings.Split(path.Dir(file), "/")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return strings.Join(parts, "/")
}

// summary is the first sentence of an issue, for the collapsed row. The full
// issue is shown when the finding is opened.
func summary(issue string) string {
	s := strings.TrimSpace(issue)
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	return s
}

func severityRank(s string) int {
	for i, v := range severities {
		if v == s {
			return i
		}
	}
	return len(severities)
}

// fileGroup is every finding in one file, in the order the page shows them.
type fileGroup struct {
	file     string
	findings []lensoutput.TypedFinding
	worst    int
}

// groupByFile puts the files with the most severe findings first, then the
// files with the most findings. Inside a file, findings go by severity, then line.
func groupByFile(fs []lensoutput.TypedFinding) []fileGroup {
	index := map[string]int{}
	var groups []fileGroup
	for _, f := range fs {
		i, ok := index[f.File]
		if !ok {
			i = len(groups)
			index[f.File] = i
			groups = append(groups, fileGroup{file: f.File, worst: len(severities)})
		}
		groups[i].findings = append(groups[i].findings, f)
		if r := severityRank(f.Severity); r < groups[i].worst {
			groups[i].worst = r
		}
	}
	for _, g := range groups {
		sort.SliceStable(g.findings, func(a, b int) bool {
			ra, rb := severityRank(g.findings[a].Severity), severityRank(g.findings[b].Severity)
			if ra != rb {
				return ra < rb
			}
			return g.findings[a].StartLine < g.findings[b].StartLine
		})
	}
	sort.SliceStable(groups, func(a, b int) bool {
		if groups[a].worst != groups[b].worst {
			return groups[a].worst < groups[b].worst
		}
		if len(groups[a].findings) != len(groups[b].findings) {
			return len(groups[a].findings) > len(groups[b].findings)
		}
		return groups[a].file < groups[b].file
	})
	return groups
}

// areaRow is one line of the map: an area and its findings' severities, worst first.
type areaRow struct {
	name       string
	severities []string
}

func areaRows(fs []lensoutput.TypedFinding) []areaRow {
	index := map[string]int{}
	var rows []areaRow
	for _, f := range fs {
		a := area(f.File)
		i, ok := index[a]
		if !ok {
			i = len(rows)
			index[a] = i
			rows = append(rows, areaRow{name: a})
		}
		rows[i].severities = append(rows[i].severities, f.Severity)
	}
	for _, r := range rows {
		sort.SliceStable(r.severities, func(a, b int) bool {
			return severityRank(r.severities[a]) < severityRank(r.severities[b])
		})
	}
	sort.SliceStable(rows, func(a, b int) bool {
		if len(rows[a].severities) != len(rows[b].severities) {
			return len(rows[a].severities) > len(rows[b].severities)
		}
		return rows[a].name < rows[b].name
	})
	return rows
}

// thousands writes n with comma separators: 1346 -> "1,346".
func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// lede is the one sentence at the top of the page: what ran, what it found,
// and where to start.
func lede(model string, reviewed int, counts []int) string {
	total := 0
	for _, c := range counts {
		total += c
	}
	s := fmt.Sprintf("Reviewed %s with %s", plural(reviewed, "file", "files"), model)
	if total == 0 {
		return s + " and found nothing to report."
	}
	s += "."
	worst := 0
	for counts[worst] == 0 {
		worst++
	}
	switch c := counts[worst]; {
	case total == 1:
		return s + fmt.Sprintf(" The one finding is a %s.", severities[worst])
	case c == total:
		return s + fmt.Sprintf(" All %s findings are %s.", thousands(c), severities[worst])
	default:
		return s + fmt.Sprintf(" Start with the %s at %s.", thousands(c), severities[worst])
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return thousands(n) + " " + many
}

const pageScript = `
(function () {
  var severity = document.getElementById("severity");
  var directory = document.getElementById("directory");
  var search = document.getElementById("search");
  var shown = document.getElementById("shown");
  var findings = document.querySelectorAll("#findings .finding");
  var groups = document.querySelectorAll("#findings .group");
  var areas = document.querySelectorAll("#map .area");
  var segments = document.querySelectorAll("#bar .seg");
  var area = "";
  function press(list, attr, value) {
    for (var i = 0; i < list.length; i++) {
      list[i].setAttribute("aria-pressed", list[i].getAttribute(attr) === value ? "true" : "false");
    }
  }
  function apply() {
    var s = severity.value;
    var d = directory.value;
    var q = search ? search.value.toLowerCase() : "";
    var n = 0;
    for (var i = 0; i < findings.length; i++) {
      var el = findings[i];
      var okS = s === "" || el.getAttribute("data-severity") === s;
      var okD = d === "" || el.getAttribute("data-dir") === d;
      var okA = area === "" || el.getAttribute("data-area") === area;
      var file = el.getAttribute("data-file") || "";
      var okQ = q === "" || file.toLowerCase().indexOf(q) >= 0 || el.textContent.toLowerCase().indexOf(q) >= 0;
      var ok = okS && okD && okA && okQ;
      el.style.display = ok ? "" : "none";
      if (ok) n++;
    }
    for (var g = 0; g < groups.length; g++) {
      var items = groups[g].querySelectorAll(".finding");
      var any = false;
      for (var k = 0; k < items.length; k++) {
        if (items[k].style.display !== "none") any = true;
      }
      groups[g].style.display = any ? "" : "none";
    }
    press(segments, "data-severity", s);
    press(areas, "data-area", area);
    if (shown) {
      shown.textContent = n === findings.length
        ? "Showing all " + n
        : "Showing " + n + " of " + findings.length;
    }
  }
  severity.addEventListener("change", apply);
  directory.addEventListener("change", apply);
  if (search) search.addEventListener("input", apply);
  for (var a = 0; a < segments.length; a++) {
    segments[a].addEventListener("click", function () {
      var v = this.getAttribute("data-severity");
      severity.value = severity.value === v ? "" : v;
      apply();
    });
  }
  for (var b = 0; b < areas.length; b++) {
    areas[b].addEventListener("click", function () {
      var v = this.getAttribute("data-area");
      area = area === v ? "" : v;
      apply();
    });
  }
  var buttons = document.querySelectorAll("#findings .finding > button");
  for (var j = 0; j < buttons.length; j++) {
    buttons[j].addEventListener("click", function () {
      var detail = this.parentNode.querySelector(".detail");
      detail.hidden = !detail.hidden;
      this.setAttribute("aria-expanded", detail.hidden ? "false" : "true");
    });
  }
})();
`

const pageCSS = `
:root {
  color-scheme: light;
  --ground: #ffffff;
  --ink: #1f232b;
  --soft: #5b6472;
  --faint: #8b93a0;
  --rule: #e3e6eb;
  --well: #f4f5f7;
  --hover: #f7f8fa;
  --p0: #b3122e;
  --p1: #d9480f;
  --p2: #c08a1e;
  --p3: #4a7fa3;
  --empty: #e3e6eb;
  --focus: #1f232b;
  --display: "Avenir Next Condensed", "Avenir Next", "Helvetica Neue", Arial, sans-serif;
  --ui: "Avenir Next", "Helvetica Neue", "Segoe UI", Arial, sans-serif;
  --prose: Charter, "Iowan Old Style", "Sitka Text", Georgia, serif;
  --code: "SF Mono", ui-monospace, Menlo, Consolas, monospace;
}
@media (prefers-color-scheme: dark) {
  :root {
    color-scheme: dark;
    --ground: #16191e;
    --ink: #e8eaee;
    --soft: #a9b0bb;
    --faint: #737b87;
    --rule: #2b3038;
    --well: #1e2228;
    --hover: #1c2026;
    --p0: #f0506b;
    --p1: #f57c3a;
    --p2: #e0b14a;
    --p3: #74a9cf;
    --empty: #2b3038;
    --focus: #e8eaee;
  }
}
* { box-sizing: border-box; }
html { background: var(--ground); }
body {
  margin: 0;
  padding: 3rem clamp(1rem, 3vw, 3.5rem) 5rem;
  color: var(--ink);
  font: 16px/1.5 var(--ui);
  -webkit-font-smoothing: antialiased;
}
button, input, select { font: inherit; color: inherit; }
:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }

/* Masthead: the repo, what reviewed it, and how much it found. */
.masthead {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0.5rem 2rem;
  align-items: end;
}
.masthead h1 {
  margin: 0;
  font-family: var(--display);
  font-size: clamp(2.6rem, 6vw, 4.5rem);
  font-weight: 700;
  line-height: 0.95;
  letter-spacing: -0.02em;
}
#repo { margin: 0.5rem 0 0; color: var(--soft); overflow-wrap: anywhere; }
.lede {
  max-width: 34rem;
  margin: 1.4rem 0 0;
  font-family: var(--prose);
  font-size: 1.3rem;
  line-height: 1.4;
}
.facts {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 0.15rem 1rem;
  margin: 0;
  font-size: 0.9rem;
}
.facts dt { color: var(--faint); }
.facts dd { margin: 0; font-variant-numeric: tabular-nums; }
#commit { font-family: var(--code); font-size: 0.82rem; overflow-wrap: anywhere; }

/* The tally: one bar split by severity, with the four counts under it. */
.tally { margin: 2.5rem 0 0; }
.total {
  margin: 0 0 0.6rem;
  font-family: var(--display);
  font-size: 1.5rem;
  font-weight: 600;
}
.total span { color: var(--soft); font-weight: 500; }
#bar {
  display: flex;
  gap: 3px;
  height: 0.9rem;
}
#bar .seg {
  min-width: 0.5rem;
  padding: 0;
  border: 0;
  border-radius: 2px;
  cursor: pointer;
}
#bar .seg[aria-pressed="true"] { box-shadow: 0 0 0 2px var(--ground), 0 0 0 4px var(--ink); }
#bar .seg:hover { filter: brightness(1.1); }
.counts {
  display: flex;
  flex-wrap: wrap;
  gap: 0.3rem 1.8rem;
  margin: 0.7rem 0 0;
  padding: 0;
  list-style: none;
  font-size: 0.95rem;
  font-variant-numeric: tabular-nums;
}
.counts li::before {
  content: "";
  display: inline-block;
  width: 0.65rem;
  height: 0.65rem;
  margin-right: 0.45rem;
  border-radius: 2px;
  background: var(--empty);
}
#count-p0::before, .sev-P0 { background: var(--p0); }
#count-p1::before, .sev-P1 { background: var(--p1); }
#count-p2::before, .sev-P2 { background: var(--p2); }
#count-p3::before, .sev-P3 { background: var(--p3); }

/* The map: where in the tree the findings sit, one tick per finding. */
.map-head {
  margin: 3rem 0 0.25rem;
  font-family: var(--display);
  font-size: 1.5rem;
  font-weight: 600;
}
.hint { margin: 0 0 1rem; color: var(--soft); font-size: 0.9rem; }
#map {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(30rem, 100%), 1fr));
  gap: 0 2.5rem;
  margin: 0;
  padding: 0;
  list-style: none;
}
#map .area {
  display: grid;
  grid-template-columns: 9.5rem minmax(0, 1fr) 2.2rem;
  gap: 0.9rem;
  align-items: center;
  width: 100%;
  padding: 0.3rem 0.4rem;
  background: transparent;
  border: 0;
  border-radius: 3px;
  cursor: pointer;
  text-align: left;
}
#map .area:hover { background: var(--hover); }
#map .area[aria-pressed="true"] { background: var(--well); box-shadow: inset 3px 0 0 var(--ink); }
.area-name {
  overflow: hidden;
  font-family: var(--code);
  font-size: 0.8rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.track { display: block; }
.ticks { display: flex; gap: 1px; min-width: 4px; }
.tick { flex: 1 1 0; min-width: 1px; height: 1rem; border-radius: 1px; }
.area-count { color: var(--soft); font-size: 0.85rem; text-align: right; font-variant-numeric: tabular-nums; }

/* Controls stay on screen while the list scrolls. */
.controls {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  flex-wrap: wrap;
  gap: 0.6rem 1.2rem;
  align-items: center;
  margin: 3rem 0 0;
  padding: 0.9rem 0;
  background: var(--ground);
  border-bottom: 1px solid var(--rule);
}
.controls label { display: flex; gap: 0.45rem; align-items: center; color: var(--soft); font-size: 0.9rem; }
select, input[type="search"] {
  padding: 0.35rem 0.55rem;
  color: var(--ink);
  background: var(--ground);
  border: 1px solid var(--rule);
  border-radius: 4px;
}
select { max-width: 17rem; }
input[type="search"] { width: 16rem; }
#shown { margin-left: auto; color: var(--soft); font-size: 0.9rem; font-variant-numeric: tabular-nums; }

/* Findings, grouped by file. */
#findings { margin-top: 0.5rem; }
.group { padding: 1.4rem 0 0.4rem; border-bottom: 1px solid var(--rule); }
.group-head {
  display: flex;
  gap: 1rem;
  align-items: baseline;
  justify-content: space-between;
  margin: 0 0 0.35rem;
  font-family: var(--code);
  font-size: 0.86rem;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.group-head .dir { color: var(--faint); font-weight: 400; }
.group-head .n { flex: none; color: var(--faint); font-family: var(--ui); font-weight: 400; }
article.finding { margin: 0; }
article.finding > button {
  display: grid;
  grid-template-columns: 2.6rem 5.2rem minmax(0, 1fr) auto;
  gap: 0.9rem;
  align-items: baseline;
  width: 100%;
  padding: 0.55rem 0.5rem;
  background: transparent;
  border: 0;
  border-radius: 4px;
  cursor: pointer;
  text-align: left;
}
article.finding > button:hover { background: var(--hover); }
.badge {
  justify-self: start;
  padding: 0.05rem 0.4rem;
  border-radius: 3px;
  color: #fff;
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.02em;
}
@media (prefers-color-scheme: dark) { .badge { color: #16191e; } }
.lines { color: var(--faint); font-family: var(--code); font-size: 0.8rem; font-variant-numeric: tabular-nums; }
.summary { font-family: var(--prose); font-size: 1rem; line-height: 1.45; overflow-wrap: anywhere; }
.consequence { overflow-wrap: anywhere; }
.tag { color: var(--faint); font-size: 0.8rem; }
.tag-bug { color: var(--soft); }
/* Open: the rest of the issue appears in place, and the impact and code sit under it. */
.rest { display: none; }
article.finding > button[aria-expanded="true"] { background: var(--well); border-radius: 4px 4px 0 0; }
article.finding > button[aria-expanded="true"] .rest { display: inline; }
article.finding > button[aria-expanded="true"] .lead { font-weight: 600; }
.detail { display: none; }
.detail:not([hidden]) {
  display: grid;
  grid-template-columns: minmax(0, 36rem) minmax(0, 1fr);
  gap: 2rem;
  margin: 0 0 1rem;
  padding: 0.6rem 0.5rem 1.2rem 10.1rem;
  background: var(--well);
  border-radius: 0 0 4px 4px;
}
.prose { font-family: var(--prose); max-width: 38rem; }
.consequence { margin: 0; color: var(--ink); line-height: 1.55; }
.consequence::before { content: "Impact: "; color: var(--ink); font-family: var(--ui); font-size: 0.85rem; font-weight: 600; }
.confidence { margin: 0.9rem 0 0; color: var(--faint); font-family: var(--ui); font-size: 0.82rem; }
.code {
  display: flex;
  max-height: 24rem;
  overflow: auto;
  background: var(--ground);
  border: 1px solid var(--rule);
  border-radius: 5px;
  align-self: start;
}
.gutter, .quote {
  margin: 0;
  padding: 0.8rem 0.9rem;
  font-family: var(--code);
  font-size: 0.78rem;
  line-height: 1.55;
  white-space: pre;
}
.gutter {
  position: sticky;
  left: 0;
  flex: none;
  padding-right: 0.7rem;
  color: var(--faint);
  background: var(--ground);
  border-right: 1px solid var(--rule);
  text-align: right;
  user-select: none;
}
.quote { flex: 1; color: var(--ink); }
.empty {
  margin: 3rem 0;
  font-family: var(--prose);
  font-size: 1.15rem;
  color: var(--soft);
}
@media (max-width: 760px) {
  body { padding: 1.5rem 1rem 3rem; }
  .masthead { grid-template-columns: 1fr; }
  #map { grid-template-columns: 1fr; }
  #map .area { grid-template-columns: 7rem minmax(0, 1fr) 2rem; }
  article.finding > button { grid-template-columns: 2.6rem minmax(0, 1fr); }
  .lines { grid-column: 2; grid-row: 2; }
  .summary { grid-column: 2; grid-row: 1; }
  .tag { display: none; }
  .detail:not([hidden]) { grid-template-columns: 1fr; padding-left: 0.5rem; }
  input[type="search"] { width: 100%; }
  #shown { margin-left: 0; }
}
@media (prefers-reduced-motion: no-preference) {
  article.finding > button, #map .area { transition: background-color 120ms ease; }
}
`

// Render returns one HTML document. It has no external assets. The quote is the
// excerpt of the cited lines; it is not a field of the findings file.
func Render(p Page) []byte {
	p0, p1, p2, p3 := severityCounts(p.Findings)
	counts := []int{p0, p1, p2, p3}
	total := len(p.Findings)
	groups := groupByFile(p.Findings)
	name := path.Base(p.Repo)
	var b strings.Builder
	fmt.Fprintf(&b, "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<title>%s source review</title>\n<style>", html.EscapeString(name))
	b.WriteString(pageCSS)
	b.WriteString("</style>\n</head>\n<body>\n<header class=\"masthead\">\n<div>\n")
	fmt.Fprintf(&b, "<h1>%s</h1>\n", html.EscapeString(name))
	fmt.Fprintf(&b, "<p class=\"meta\" id=\"repo\">%s</p>\n", html.EscapeString(p.Repo))
	fmt.Fprintf(&b, "<p class=\"lede\">%s</p>\n", html.EscapeString(lede(p.ModelID, len(p.Files), counts)))
	b.WriteString("</div>\n<dl class=\"facts\">\n")
	fmt.Fprintf(&b, "<dt>Model</dt><dd class=\"meta\" id=\"model\">%s</dd>\n", html.EscapeString(p.ModelID))
	fmt.Fprintf(&b, "<dt>Commit</dt><dd class=\"meta\" id=\"commit\">%s</dd>\n", html.EscapeString(p.Commit))
	fmt.Fprintf(&b, "<dt>Reviewed</dt><dd>%s</dd>\n", plural(len(p.Files), "file", "files"))
	b.WriteString("</dl>\n</header>\n")

	b.WriteString("<section class=\"tally\">\n")
	fmt.Fprintf(&b, "<p class=\"total\">%s <span>in %s</span></p>\n", plural(total, "finding", "findings"), plural(len(groups), "file", "files"))
	b.WriteString("<div id=\"bar\">\n")
	for i, s := range severities {
		if counts[i] == 0 {
			continue
		}
		fmt.Fprintf(&b, "<button type=\"button\" class=\"seg sev-%s\" data-severity=\"%s\" style=\"flex-grow:%d\" aria-pressed=\"false\" title=\"Show only %s\"><span hidden>%s</span></button>\n", s, s, counts[i], s, s)
	}
	b.WriteString("</div>\n<ul class=\"counts\">\n")
	for i, s := range severities {
		fmt.Fprintf(&b, "<li id=\"count-%s\">%s %d</li>\n", strings.ToLower(s), s, counts[i])
	}
	b.WriteString("</ul>\n</section>\n")

	if total > 0 {
		b.WriteString("<section>\n<h2 class=\"map-head\">Where the findings are</h2>\n")
		b.WriteString("<p class=\"hint\">One mark per finding, worst first. Select an area to show only its findings.</p>\n<ul id=\"map\">\n")
		rows := areaRows(p.Findings)
		most := len(rows[0].severities)
		for _, r := range rows {
			fmt.Fprintf(&b, "<li><button type=\"button\" class=\"area\" data-area=\"%s\" aria-pressed=\"false\"><span class=\"area-name\">%s</span><span class=\"track\"><span class=\"ticks\" style=\"width:%.2f%%\">", html.EscapeString(r.name), html.EscapeString(r.name), 100*float64(len(r.severities))/float64(most))
			for _, s := range r.severities {
				fmt.Fprintf(&b, "<i class=\"tick sev-%s\"></i>", html.EscapeString(s))
			}
			fmt.Fprintf(&b, "</span></span><span class=\"area-count\">%d</span></button></li>\n", len(r.severities))
		}
		b.WriteString("</ul>\n</section>\n")
	}

	b.WriteString("<div class=\"controls\">\n")
	b.WriteString("<label>Severity <select id=\"severity\">\n")
	b.WriteString("<option value=\"\">all</option>\n<option value=\"P0\">P0</option>\n<option value=\"P1\">P1</option>\n<option value=\"P2\">P2</option>\n<option value=\"P3\">P3</option>\n")
	b.WriteString("</select></label>\n")
	b.WriteString("<label>Directory <select id=\"directory\">\n<option value=\"\">all</option>\n")
	for _, d := range findingDirs(p.Findings) {
		fmt.Fprintf(&b, "<option value=\"%s\">%s</option>\n", html.EscapeString(d), html.EscapeString(d))
	}
	b.WriteString("</select></label>\n")
	b.WriteString("<label>Search <input type=\"search\" id=\"search\" placeholder=\"Words in the issue, file, or code\"></label>\n")
	fmt.Fprintf(&b, "<p id=\"shown\">Showing all %d</p>\n", total)
	b.WriteString("</div>\n<div id=\"findings\">\n")
	if total == 0 {
		fmt.Fprintf(&b, "<p class=\"empty\">No findings. %s reviewed %s and reported nothing to fix.</p>\n", html.EscapeString(p.ModelID), plural(len(p.Files), "file", "files"))
	}
	for _, g := range groups {
		b.WriteString("<section class=\"group\">\n")
		dirPart, base := path.Split(g.file)
		fmt.Fprintf(&b, "<h3 class=\"group-head\"><span><span class=\"dir\">%s</span>%s</span><span class=\"n\">%s</span></h3>\n", html.EscapeString(dirPart), html.EscapeString(base), plural(len(g.findings), "finding", "findings"))
		for _, f := range g.findings {
			dir := path.Dir(f.File)
			quote := ""
			if p.Files != nil {
				quote = excerpt(p.Files[f.File], f.StartLine, f.EndLine)
			}
			lines := fmt.Sprintf("L%d", f.StartLine)
			if f.EndLine != f.StartLine {
				lines = fmt.Sprintf("L%d–%d", f.StartLine, f.EndLine)
			}
			tag := string(f.Tag)
			fmt.Fprintf(&b, "<article class=\"finding\" data-file=\"%s\" data-severity=\"%s\" data-dir=\"%s\" data-area=\"%s\">\n", html.EscapeString(f.File), html.EscapeString(f.Severity), html.EscapeString(dir), html.EscapeString(area(f.File)))
			// The row shows the issue's first sentence; opening it reveals the rest in place.
			issue := strings.TrimSpace(f.Issue)
			lead := summary(issue)
			fmt.Fprintf(&b, "<button type=\"button\" aria-expanded=\"false\"><span class=\"badge sev-%s\">%s</span><span class=\"lines\">%s</span><span class=\"summary issue\"><span class=\"lead\">%s</span><span class=\"rest\">%s</span></span><span class=\"tag tag-%s\">%s</span></button>\n",
				html.EscapeString(f.Severity), html.EscapeString(f.Severity), lines, html.EscapeString(lead), html.EscapeString(issue[len(lead):]), html.EscapeString(tag), html.EscapeString(tag))
			b.WriteString("<div class=\"detail\" hidden>\n<div class=\"prose\">\n")
			fmt.Fprintf(&b, "<p class=\"consequence\">%s</p>\n", html.EscapeString(f.Consequence))
			fmt.Fprintf(&b, "<p class=\"confidence\">Model confidence %d of 100</p>\n", f.Confidence)
			b.WriteString("</div>\n<div class=\"code\">")
			var gutter []string
			for n := f.StartLine; quote != "" && n <= f.EndLine; n++ {
				gutter = append(gutter, fmt.Sprint(n))
			}
			fmt.Fprintf(&b, "<pre class=\"gutter\" aria-hidden=\"true\">%s</pre>", strings.Join(gutter, "\n"))
			fmt.Fprintf(&b, "<pre class=\"quote\">%s</pre>", html.EscapeString(quote))
			b.WriteString("</div>\n</div>\n</article>\n")
		}
		b.WriteString("</section>\n")
	}
	b.WriteString("</div>\n<script id=\"page\">")
	b.WriteString(pageScript)
	b.WriteString("</script>\n</body>\n</html>\n")
	return []byte(b.String())
}
