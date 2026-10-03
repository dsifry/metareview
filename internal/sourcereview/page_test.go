package sourcereview

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/lensoutput"
)

func TestRenderFiltersAndQuote(t *testing.T) {
	files := map[string][]byte{
		"main.go":    []byte("package main\n"),
		"pkg/app.go": []byte("alpha\nbeta\n"),
		"a.go":       []byte("a\n"),
		"b/c.go":     []byte("c\n"),
	}
	findings := []lensoutput.TypedFinding{
		{Tag: lensoutput.TagBug, File: "main.go", StartLine: 1, EndLine: 1, Issue: "root broke", Consequence: "no entry", Confidence: 90, Severity: "P0"},
		{Tag: lensoutput.TagAdvisory, File: "pkg/app.go", StartLine: 1, EndLine: 2, Issue: "alpha broke", Consequence: "wrong answer", Confidence: 70, Severity: "P1"},
		{Tag: lensoutput.TagBug, File: "a.go", StartLine: 1, EndLine: 1, Issue: "a", Consequence: "a", Confidence: 60, Severity: "P2"},
		{Tag: lensoutput.TagBug, File: "b/c.go", StartLine: 1, EndLine: 1, Issue: "c", Consequence: "c", Confidence: 50, Severity: "P3"},
	}
	page := Render(Page{Repo: "/repo", Commit: "abc123", ModelID: "opus", Findings: findings, Files: files})
	html := string(page)
	for _, want := range []string{
		`id="repo">/repo`, `id="commit">abc123`, `id="model">opus`,
		`id="count-p0">P0 1`, `id="count-p1">P1 1`, `id="count-p2">P2 1`, `id="count-p3">P3 1`,
		`id="severity"`, `id="directory"`, `value="."`, `value="pkg"`,
		"root broke", "no entry", "package main", "alpha broke", "wrong answer", "alpha\nbeta",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("page missing %q", want)
		}
	}
	if strings.Contains(html, "http://") || strings.Contains(html, "https://") || strings.Contains(html, "fetch(") || strings.Contains(html, "<form") || strings.Contains(html, "src=") {
		t.Fatal("page is not self-contained")
	}
	if !strings.Contains(html, `<script id="page">`) {
		t.Fatal("shipped script missing")
	}
	report := runPage(t, page)
	if report.Counts["p0"] != "P0 1" || report.Counts["p3"] != "P3 1" {
		t.Fatalf("counts %+v", report.Counts)
	}
	// The two findings the filter drives are the first two, which differ in
	// severity and directory. The harness records what the shipped script did.
	if len(report.Steps) < 2 {
		t.Fatalf("steps %+v", report)
	}
	sev := report.Steps[0]
	if sev.Displays[0] != "" || sev.Displays[1] != "none" {
		t.Fatalf("severity filter displays %v", sev.Displays)
	}
	dir := report.Steps[1]
	if dir.Displays[0] != "" || dir.Displays[1] != "none" {
		t.Fatalf("directory filter displays %v", dir.Displays)
	}
	if report.Opened.Hidden || report.Opened.Expanded != "true" || report.Opened.Issue != "root broke" || report.Opened.Consequence != "no entry" || report.Opened.Quote != "package main" {
		t.Fatalf("opened %+v", report.Opened)
	}
	if !report.Closed.Hidden || report.Closed.Expanded != "false" {
		t.Fatalf("closed %+v", report.Closed)
	}
	all := []string{"", "", "", ""}
	if report.Bar.Severity != "P0" || !reflectArgs(report.Bar.Displays, []string{"", "none", "none", "none"}) || report.Bar.Pressed != "true" || report.Bar.Shown != "Showing 1 of 4" {
		t.Fatalf("bar %+v", report.Bar)
	}
	if report.BarCleared.Severity != "" || !reflectArgs(report.BarCleared.Displays, all) || report.BarCleared.Shown != "Showing all 4" {
		t.Fatalf("bar cleared %+v", report.BarCleared)
	}
	// The biggest area is the repo root: main.go and a.go.
	if report.Area.Name != "." || !reflectArgs(report.Area.Displays, []string{"", "none", "", "none"}) || !reflectArgs(report.Area.Groups, []string{"", "none", "", "none"}) || report.Area.Pressed != "true" {
		t.Fatalf("area %+v", report.Area)
	}
	if !reflectArgs(report.AreaCleared.Displays, all) || report.AreaCleared.Pressed != "false" {
		t.Fatalf("area cleared %+v", report.AreaCleared)
	}
	if !reflectArgs(report.Search.Displays, []string{"none", "", "none", "none"}) || report.Search.Shown != "Showing 1 of 4" {
		t.Fatalf("search %+v", report.Search)
	}
	if !reflectArgs(report.FileSearch.Displays, []string{"none", "", "none", "none"}) || report.FileSearch.Shown != "Showing 1 of 4" {
		t.Fatalf("filename search %+v", report.FileSearch)
	}
}

func TestPageHelpers(t *testing.T) {
	for n, want := range map[int]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 1346: "1,346", 1234567: "1,234,567"} {
		if got := thousands(n); got != want {
			t.Fatalf("thousands(%d) = %q", n, got)
		}
	}
	if plural(1, "file", "files") != "1 file" || plural(1346, "file", "files") != "1,346 files" {
		t.Fatal("plural")
	}
	for _, c := range []struct {
		counts []int
		want   string
	}{
		{[]int{0, 0, 0, 0}, "Reviewed 3 files with opus and found nothing to report."},
		{[]int{0, 37, 136, 123}, "Reviewed 3 files with opus. Start with the 37 at P1."},
		{[]int{0, 0, 1, 0}, "Reviewed 3 files with opus. The one finding is a P2."},
		{[]int{0, 0, 0, 2}, "Reviewed 3 files with opus. All 2 findings are P3."},
	} {
		if got := lede("opus", 3, c.counts); got != c.want {
			t.Fatalf("lede %v = %q", c.counts, got)
		}
	}
	if summary(" One. Two. ") != "One." || summary("No period") != "No period" || summary("v1.2 is out") != "v1.2 is out" {
		t.Fatal("summary")
	}
	if area("main.go") != "." || area("app/x.rb") != "app" || area("app/models/concerns/x.rb") != "app/models" {
		t.Fatal("area")
	}
	if severityRank("P0") != 0 || severityRank("P3") != 3 || severityRank("bogus") != 4 {
		t.Fatal("severityRank")
	}
	f := func(file, sev string, line int) lensoutput.TypedFinding {
		return lensoutput.TypedFinding{File: file, Severity: sev, StartLine: line, EndLine: line}
	}
	groups := groupByFile([]lensoutput.TypedFinding{
		f("z.go", "P2", 9), f("z.go", "P2", 3), f("z.go", "P1", 5),
		f("b.go", "P1", 1),
		f("a.go", "P1", 1),
		f("c.go", "P3", 1),
	})
	var order []string
	for _, g := range groups {
		order = append(order, g.file)
	}
	// Worst severity first; among equals, more findings first; then by path.
	if !reflectArgs(order, []string{"z.go", "a.go", "b.go", "c.go"}) {
		t.Fatalf("group order %v", order)
	}
	z := groups[0].findings
	if z[0].StartLine != 5 || z[1].StartLine != 3 || z[2].StartLine != 9 {
		t.Fatalf("findings in z.go %+v", z)
	}
	rows := areaRows([]lensoutput.TypedFinding{f("b/x.go", "P3", 1), f("a/x.go", "P3", 1), f("c/x.go", "P2", 1), f("c/y.go", "P1", 1)})
	if rows[0].name != "c" || !reflectArgs(rows[0].severities, []string{"P1", "P2"}) || rows[1].name != "a" || rows[2].name != "b" {
		t.Fatalf("area rows %+v", rows)
	}
}

func TestRenderRangesAndGutter(t *testing.T) {
	files := map[string][]byte{"pkg/app.go": []byte("alpha\nbeta\ngamma\n")}
	page := string(Render(Page{Repo: "/src/app", Commit: "c", ModelID: "grok-4.7", Files: files, Findings: []lensoutput.TypedFinding{
		{Tag: lensoutput.TagBug, File: "pkg/app.go", StartLine: 2, EndLine: 3, Issue: "First sentence. Second sentence.", Consequence: "bad", Confidence: 80, Severity: "P2"},
	}}))
	for _, want := range []string{
		"<h1>app</h1>", "L2–3", `<pre class="gutter" aria-hidden="true">2` + "\n3</pre>", `<pre class="quote">beta` + "\ngamma</pre>",
		`<span class="lead">First sentence.</span><span class="rest"> Second sentence.</span>`,
		`<span class="dir">pkg/</span>app.go`, "Model confidence 80 of 100", "The one finding is a P2.", `data-area="pkg"`, `data-file="pkg/app.go"`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("page missing %q", want)
		}
	}
}

func TestRenderEmpty(t *testing.T) {
	page := Render(Page{Repo: "r", Commit: "c", ModelID: "gpt-6-astra", Findings: nil, Files: nil})
	html := string(page)
	for _, id := range []string{"count-p0", "count-p1", "count-p2", "count-p3"} {
		if !strings.Contains(html, `id="`+id+`">`) || !strings.Contains(html, "0") {
			t.Fatalf("missing zero %s", id)
		}
	}
	if strings.Contains(html, `class="finding"`) {
		t.Fatal("empty page has a finding")
	}
	if !strings.Contains(html, `id="severity"`) || !strings.Contains(html, `id="directory"`) {
		t.Fatal("filters missing")
	}
	if !strings.Contains(html, "No findings. gpt-6-astra reviewed 0 files and reported nothing to fix.") || strings.Contains(html, `id="map"`) || strings.Contains(html, `class="seg`) {
		t.Fatal("empty page should explain itself and draw no map or bar segments")
	}
	report := runPage(t, page)
	if report.FindingCount != 0 {
		t.Fatalf("findings %d", report.FindingCount)
	}
	for _, k := range []string{"p0", "p1", "p2", "p3"} {
		if !strings.HasSuffix(report.Counts[k], " 0") {
			t.Fatalf("%s = %s", k, report.Counts[k])
		}
	}
}

type pageReport struct {
	FindingCount int               `json:"findingCount"`
	Counts       map[string]string `json:"counts"`
	Steps        []struct {
		Displays []string `json:"displays"`
	} `json:"steps"`
	Opened struct {
		Hidden      bool   `json:"hidden"`
		Expanded    string `json:"expanded"`
		Issue       string `json:"issue"`
		Consequence string `json:"consequence"`
		Quote       string `json:"quote"`
	} `json:"opened"`
	Closed struct {
		Hidden   bool   `json:"hidden"`
		Expanded string `json:"expanded"`
	} `json:"closed"`
	Bar struct {
		Severity string   `json:"severity"`
		Displays []string `json:"displays"`
		Pressed  string   `json:"pressed"`
		Shown    string   `json:"shown"`
	} `json:"bar"`
	BarCleared struct {
		Severity string   `json:"severity"`
		Displays []string `json:"displays"`
		Shown    string   `json:"shown"`
	} `json:"barCleared"`
	Area struct {
		Name     string   `json:"name"`
		Displays []string `json:"displays"`
		Groups   []string `json:"groups"`
		Pressed  string   `json:"pressed"`
	} `json:"area"`
	AreaCleared struct {
		Displays []string `json:"displays"`
		Pressed  string   `json:"pressed"`
	} `json:"areaCleared"`
	Search struct {
		Displays []string `json:"displays"`
		Shown    string   `json:"shown"`
	} `json:"search"`
	FileSearch struct {
		Displays []string `json:"displays"`
		Shown    string   `json:"shown"`
	} `json:"fileSearch"`
}

func runPage(t *testing.T, page []byte) pageReport {
	t.Helper()
	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "page.html")
	scriptPath := filepath.Join(dir, "dom.js")
	if err := os.WriteFile(htmlPath, page, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte(pageHarness), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", scriptPath, htmlPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	var report pageReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("report: %v\n%s", err, out)
	}
	return report
}

const pageHarness = `
const fs = require("fs");
const html = fs.readFileSync(process.argv[2], "utf8");
const m = html.match(/<script id="page">([\s\S]*)<\/script>/);
if (!m) { console.error("no shipped script"); process.exit(1); }
const script = m[1];

function decode(s) {
  return s.replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&amp;/g, "&");
}
function parseAttrs(s) {
  const attrs = {};
  const re = /([A-Za-z0-9:-]+)(?:\s*=\s*"([^"]*)")?/g;
  let mm;
  while ((mm = re.exec(s))) attrs[mm[1]] = decode(mm[2] ?? "");
  return attrs;
}
class El {
  constructor(name, attrs) {
    this.name = name;
    this.attrs = attrs || {};
    this.children = [];
    this.parentNode = null;
    this.style = { display: "" };
    this.hidden = Object.prototype.hasOwnProperty.call(this.attrs, "hidden");
    this.value = this.attrs.value || "";
    this._listeners = {};
  }
  getAttribute(k) { return Object.prototype.hasOwnProperty.call(this.attrs, k) ? this.attrs[k] : null; }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  addEventListener(type, fn) { (this._listeners[type] ||= []).push(fn); }
  dispatchEvent(type) { for (const fn of this._listeners[type] || []) fn.call(this); }
  get textContent() { return this.children.map(c => c.textContent || "").join(""); }
  set textContent(v) { this.children = [{ textContent: String(v), name: "" }]; }
  querySelector(sel) { const a = queryAll(this, sel); return a[0] || null; }
  querySelectorAll(sel) { return queryAll(this, sel); }
}
function classHas(n, c) { return (" " + (n.attrs.class || "") + " ").includes(" " + c + " "); }
function matchesSimple(n, sel) {
  if (!n || !n.name) return false;
  if (sel.startsWith("#")) return n.attrs.id === sel.slice(1);
  if (sel.startsWith(".")) return classHas(n, sel.slice(1));
  return n.name === sel;
}
function descendants(n) {
  const out = [];
  for (const c of n.children || []) {
    if (!c.name) continue;
    out.push(c);
    out.push(...descendants(c));
  }
  return out;
}
function queryAll(root, sel) {
  sel = sel.trim();
  const gt = sel.indexOf(">");
  if (gt >= 0) {
    const left = sel.slice(0, gt).trim();
    const right = sel.slice(gt + 1).trim();
    const out = [];
    for (const p of queryAll(root, left)) {
      for (const c of p.children) if (c.name && matchesSimple(c, right)) out.push(c);
    }
    return out;
  }
  const parts = sel.split(/\s+/).filter(Boolean);
  let cur = [root];
  for (const part of parts) {
    const next = [];
    for (const n of cur) for (const d of descendants(n)) if (matchesSimple(d, part)) next.push(d);
    cur = next;
  }
  return cur;
}
const voidTags = new Set(["meta", "link", "br", "img", "input"]);
function parseHTML(src) {
  const root = new El("#document", {});
  const stack = [root];
  const re = /<!DOCTYPE[^>]*>|<!--[\s\S]*?-->|<\/([A-Za-z0-9]+)>|<([A-Za-z0-9]+)([^>]*)>|([^<]+)/g;
  let mm;
  while ((mm = re.exec(src))) {
    if (mm[1]) {
      const name = mm[1].toLowerCase();
      while (stack.length > 1 && stack[stack.length - 1].name !== name) stack.pop();
      if (stack.length > 1) stack.pop();
    } else if (mm[2]) {
      const name = mm[2].toLowerCase();
      const el = new El(name, parseAttrs(mm[3] || ""));
      const parent = stack[stack.length - 1];
      parent.children.push(el);
      el.parentNode = parent;
      const selfClose = /\/\s*$/.test(mm[3] || "") || voidTags.has(name);
      if (!selfClose) stack.push(el);
    } else if (mm[4]) {
      stack[stack.length - 1].children.push({ textContent: decode(mm[4]), name: "" });
    }
  }
  return root;
}
const root = parseHTML(html);
const document = {
  getElementById(id) {
    const hit = descendants(root).filter(n => n.attrs && n.attrs.id === id);
    return hit[0] || null;
  },
  querySelectorAll(sel) { return queryAll(root, sel); },
};
try { new Function("document", script)(document); }
catch (e) { console.error(String(e)); process.exit(1); }
const findings = document.querySelectorAll("#findings .finding");
const counts = {};
for (const id of ["p0", "p1", "p2", "p3"]) counts[id] = document.getElementById("count-" + id).textContent.trim();
const report = { findingCount: findings.length, counts, steps: [], opened: {} };
if (findings.length >= 2) {
  const sev = document.getElementById("severity");
  sev.value = findings[0].getAttribute("data-severity");
  sev.dispatchEvent("change");
  report.steps.push({ displays: findings.map(f => f.style.display) });
  sev.value = "";
  const dir = document.getElementById("directory");
  dir.value = findings[0].getAttribute("data-dir");
  dir.dispatchEvent("change");
  report.steps.push({ displays: findings.map(f => f.style.display) });
  dir.value = "";
  dir.dispatchEvent("change");
  const btn = findings[0].querySelector("button");
  btn.dispatchEvent("click");
  const detail = findings[0].querySelector(".detail");
  report.opened = {
    hidden: detail.hidden,
    expanded: btn.getAttribute("aria-expanded"),
    issue: findings[0].querySelector(".issue").textContent.trim(),
    consequence: findings[0].querySelector(".consequence").textContent.trim(),
    quote: findings[0].querySelector(".quote").textContent.trim(),
  };
  btn.dispatchEvent("click");
  report.closed = { hidden: detail.hidden, expanded: btn.getAttribute("aria-expanded") };
  const displays = () => findings.map(f => f.style.display);
  const groupDisplays = () => document.querySelectorAll("#findings .group").map(g => g.style.display);
  const shown = () => document.getElementById("shown").textContent;
  // The severity bar: one click selects that severity, a second click clears it.
  const seg = document.querySelectorAll("#bar .seg")[0];
  seg.dispatchEvent("click");
  report.bar = { severity: sev.value, displays: displays(), pressed: seg.getAttribute("aria-pressed"), shown: shown() };
  seg.dispatchEvent("click");
  report.barCleared = { severity: sev.value, displays: displays(), shown: shown() };
  // The map: one click shows only that area and hides the other files' groups.
  const areaBtn = document.querySelectorAll("#map .area")[0];
  areaBtn.dispatchEvent("click");
  report.area = { name: areaBtn.getAttribute("data-area"), displays: displays(), groups: groupDisplays(), pressed: areaBtn.getAttribute("aria-pressed") };
  areaBtn.dispatchEvent("click");
  report.areaCleared = { displays: displays(), pressed: areaBtn.getAttribute("aria-pressed") };
  // Search matches the finding's text, case-insensitively.
  const search = document.getElementById("search");
  search.value = findings[1].querySelector(".issue").textContent.trim().toUpperCase();
  search.dispatchEvent("input");
  report.search = { displays: displays(), shown: shown() };
  // Filename matching is separate from issue/snippet text and ignores case.
  search.value = findings[1].getAttribute("data-file").split("/").pop().toUpperCase();
  search.dispatchEvent("input");
  report.fileSearch = { displays: displays(), shown: shown() };
}
process.stdout.write(JSON.stringify(report));
`
