// Flattens a mutation-testing-report so the planner can query mutants by file and by test id.
// Ids are only meaningful inside this one report (spec F7).
export function indexReport(report) {
  const mutants = [];
  for (const [file, entry] of Object.entries(report.files ?? {})) {
    for (const m of entry.mutants ?? []) {
      mutants.push({
        file,
        id: String(m.id),
        status: m.status,
        killedBy: (m.killedBy ?? []).map(String),
        coveredBy: (m.coveredBy ?? []).map(String),
        startLine: m.location.start.line,
        endLine: m.location.end.line,
      });
    }
  }
  const testIds = {};
  for (const [file, entry] of Object.entries(report.testFiles ?? {})) {
    testIds[file] = (entry.tests ?? []).map((t) => String(t.id));
  }
  return { mutants, testIds };
}
