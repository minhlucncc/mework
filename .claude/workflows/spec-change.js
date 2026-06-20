export const meta = {
  name: 'spec-change',
  description:
    'Author and quality-assure an OpenSpec change spec: draft (if missing) → cross-validate → revise → report. Loads an existing change (or scaffolds one with the cNNNN- numbering convention and drafts its artifacts), then fans out one read-only critic agent per spec-review axis IN PARALLEL (Structure/validity, Clarity/KISS, Testability, Minimality/YAGNI, Consistency/DRY, Completeness), collects severity-labelled findings, and runs a revise loop that fixes Blocker/Required items and re-runs openspec validate --strict until clean (or maxRevisions). Writes openspec/changes/<change>/review/REVIEW.md and returns the verdict. Honors dryRun (review-only, no edits), reserveTokens, and maxRevisions. Mirrors the ship workflow idiom; the spec-layer counterpart of code review.',
  phases: [
    { title: 'Preflight', detail: 'openspec status + validate; load or scaffold the change' },
    { title: 'Cross-validate', detail: 'one read-only critic per axis, in parallel' },
    { title: 'Revise', detail: 'fix Blocker/Required findings, re-validate (skipped on dryRun)' },
    { title: 'Report', detail: 'write review/REVIEW.md + verdict' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
const change = A.change
const slug = A.slug // optional kebab slug to draft a NEW change when `change` does not exist yet
const date = A.date // YYYY-MM-DD — passed in; Date.now()/new Date() are unavailable in scripts
const dryRun = !!A.dryRun // review-only: produce findings + report, make NO edits
const reserve = A.reserveTokens || 50000
const maxRevisions = A.maxRevisions ?? 2

if (!change || typeof change !== 'string') {
  throw new Error('spec-change requires args { change, date, dryRun?, slug?, maxRevisions? }; got typeof=' + (typeof args) + ' keys=' + Object.keys(A).join(','))
}
if (!/^[a-z][a-z0-9-]*$/.test(change)) throw new Error('Unsafe change name (must start with a letter, kebab-case): ' + change)
if (slug && !/^[a-z0-9][a-z0-9-]*$/.test(slug)) throw new Error('Unsafe slug (kebab-case): ' + slug)
if (date && !/^\d{4}-\d{2}-\d{2}$/.test(date)) throw new Error('Unsafe date (expected YYYY-MM-DD): ' + date)

const SKILL = (name) => `the \`${name}\` skill (.claude/skills/${name}/SKILL.md)`
const reviewSkill = SKILL('spec-review-and-quality')

// ---------------------------------------------------------------- the six review axes (single source of truth)
const AXES = [
  { key: 'structure', title: 'Structure & validity', brief: 'openspec validate --strict passes; Purpose + Requirements; SHALL/MUST in the body line; >=1 Scenario each; delta uses ADDED/MODIFIED/REMOVED/RENAMED with the FULL requirement on MODIFY; MODIFIED/REMOVED names exist in the baseline spec. Structural failures are Blockers.' },
  { key: 'clarity', title: 'Clarity & KISS', brief: 'one requirement = one behavior; no "and...and..." packing across distinct concerns; plain-language bodies; specific requirement names.' },
  { key: 'testability', title: 'Testability', brief: 'every scenario decidable with concrete literals and a definite WHEN/THEN; NO soft MAY / "to the extent of" in an intended-behavior THEN (isolate true optionality); edge/negative scenarios where they matter.' },
  { key: 'minimality', title: 'Minimality & YAGNI', brief: 'normative requirements describe only in-scope/built behavior; future options (extra backends, multi-module, unused forms) belong in design.md, not requirements; spec behavior, not process ("tests MUST pass" is a task); no requirement that changes no test.' },
  { key: 'consistency', title: 'Consistency & DRY', brief: 'each behavior defined once (reference, do not restate); canonical glossary terms (runner/dispatch/hub/session/grant/sandbox/agent), no mixed vocabulary; consistent with the CLAUDE.md invariants (a contradiction is a Blocker).' },
  { key: 'completeness', title: 'Completeness (not partials)', brief: 'every proposal claim -> a requirement; every requirement -> >=1 scenario AND covering task(s) in tasks.md; every task -> a requirement; design.md records the non-trivial decisions.' },
]

// ---------------------------------------------------------------- schemas
const PREFLIGHT = {
  type: 'object', additionalProperties: false,
  required: ['exists', 'ready', 'reason', 'changeRoot', 'tasksPath', 'specPaths'],
  properties: {
    exists: { type: 'boolean', description: 'true if the change directory already exists' },
    ready: { type: 'boolean', description: 'true if the change exists, openspec validate ran, and proposal/tasks + >=1 delta spec are present' },
    reason: { type: 'string' },
    validatePass: { type: 'boolean', description: 'whether openspec validate --strict passed (false if it did not run)' },
    changeRoot: { type: 'string' },
    proposalPath: { type: ['string', 'null'] },
    designPath: { type: ['string', 'null'] },
    tasksPath: { type: ['string', 'null'] },
    specPaths: { type: 'array', items: { type: 'string' }, description: 'the change delta spec.md paths' },
    title: { type: 'string', description: 'human-readable change title from proposal.md' },
  },
}
const FINDING = {
  type: 'object', additionalProperties: false,
  required: ['severity', 'location', 'problem', 'suggestion'],
  properties: {
    axis: { type: 'string', description: 'the review axis this finding belongs to (carry it through on remaining items)' },
    severity: { type: 'string', enum: ['Blocker', 'Required', 'Nit', 'FYI'], description: 'Blocker=invalid/untestable/contradicts an invariant; Required=fix before apply; Nit=optional; FYI=info' },
    location: { type: 'string', description: 'file + requirement/scenario name the finding is about' },
    problem: { type: 'string', description: 'what is wrong, concretely' },
    suggestion: { type: 'string', description: 'the concrete fix' },
  },
}
const REVIEW = {
  type: 'object', additionalProperties: false, required: ['axis', 'summary', 'findings'],
  properties: {
    axis: { type: 'string' },
    summary: { type: 'string', description: 'one-line verdict for this axis' },
    findings: { type: 'array', items: FINDING },
  },
}
const REVISE = {
  type: 'object', additionalProperties: false, required: ['applied', 'validatePass', 'remaining', 'notes'],
  properties: {
    applied: { type: 'integer', description: 'number of Blocker/Required findings actually fixed in the artifacts' },
    validatePass: { type: 'boolean', description: 'openspec validate --strict result AFTER edits' },
    remaining: { type: 'array', items: FINDING, description: 'Blocker/Required findings NOT resolved (with reason in problem)' },
    notes: { type: 'string' },
  },
}
const REPORTW = {
  type: 'object', additionalProperties: false, required: ['written', 'path', 'verdict'],
  properties: {
    written: { type: 'boolean' },
    path: { type: 'string', description: 'review/REVIEW.md path' },
    verdict: { type: 'string', enum: ['approve', 'revise'], description: 'approve if no Blocker/Required remains' },
  },
}

const isOpen = (f) => f && (f.severity === 'Blocker' || f.severity === 'Required')

// ---------------------------------------------------------------- Phase 1: Preflight (load or scaffold)
phase('Preflight')
let pre = await agent(
  [
    `Preflight for OpenSpec change "${change}". Use Bash. Read-only except the scaffold step below.`,
    `1. Check if openspec/changes/${change}/ exists. Run: openspec status --change "${change}" --json (it errors if absent).`,
    `2. If it EXISTS: parse changeRoot, proposal/design/tasks artifact paths, and the delta spec.md paths; read proposal.md for a one-line title; run openspec validate "${change}" --strict and report validatePass. Set exists=true, ready=(proposal+tasks+>=1 delta spec present).`,
    !dryRun && slug
      ? `3. If it does NOT exist: scaffold it now. The change name "${change}" is the target; create it via "openspec new change "${change}"" (names MUST start with a letter — the cNNNN- convention satisfies this), then draft proposal.md, the delta spec(s) under specs/<capability>/, design.md, and tasks.md following ${SKILL('openspec-propose')} and ${SKILL('spec-driven-development')} (use openspec instructions <artifact> --change "${change}" --json templates). Then set exists=true and return the resolved paths.`
      : `3. If it does NOT exist: set exists=false, ready=false, reason="change not found"${dryRun ? ' (dryRun: not scaffolding)' : ' (no slug given to draft)'} and STOP without creating anything.`,
    `Return the structured result. The actionContext.mode must be spec-driven/repo-local; if it is workspace-planning, set ready=false and say so.`,
  ].filter(Boolean).join('\n'),
  { schema: PREFLIGHT, label: 'preflight', phase: 'Preflight', agentType: 'general-purpose' },
)
if (!pre || !pre.exists || !pre.ready) {
  return { stage: 'preflight', ok: false, reason: pre ? pre.reason : 'preflight agent returned null', change }
}
const title = pre.title || change
const CONTEXT = [
  `Change "${change}" — "${title}". Artifacts (read them):`,
  pre.proposalPath ? `- proposal (what & why): ${pre.proposalPath}` : '',
  pre.designPath ? `- design (how): ${pre.designPath}` : '',
  pre.tasksPath ? `- tasks (the checklist): ${pre.tasksPath}` : '',
  pre.specPaths && pre.specPaths.length ? `- delta specs: ${pre.specPaths.join(', ')}` : '- delta specs: (none)',
].filter(Boolean).join('\n')
log(`preflight ok — ${change}: ${pre.specPaths.length} delta spec(s); validate ${pre.validatePass ? 'passed' : 'NOT passing'}`)

// ---------------------------------------------------------------- Phase 2: Cross-validate (parallel critics)
phase('Cross-validate')
const reviews = (await parallel(
  AXES.map((ax) => () =>
    agent(
      [
        `Review OpenSpec change "${change}" on ONE axis only: ${ax.title}.`,
        `Apply ${reviewSkill}. This is a READ-ONLY review — do NOT edit any file.`,
        CONTEXT,
        `Axis focus: ${ax.brief}`,
        ax.key === 'structure' ? `Run: openspec validate "${change}" --strict --no-interactive — a failure is a Blocker; quote the error.` : '',
        ax.key === 'consistency' || ax.key === 'completeness' ? `You MAY read the baseline openspec/specs/<capability>/spec.md and CLAUDE.md to cross-check.` : '',
        `Return findings strictly scoped to THIS axis. Each finding: severity (Blocker/Required/Nit/FYI), location (file + requirement/scenario), problem, suggestion. If the axis is clean, return an empty findings array with a one-line summary.`,
      ].filter(Boolean).join('\n'),
      { schema: REVIEW, label: `critic:${ax.key}`, phase: 'Cross-validate', agentType: 'general-purpose' },
    ),
  ),
)).filter(Boolean)

let findings = reviews.flatMap((r) => (r.findings || []).map((f) => ({ ...f, axis: r.axis })))
let openCount = findings.filter(isOpen).length
log(`cross-validate: ${reviews.length}/${AXES.length} axes reviewed, ${findings.length} finding(s), ${openCount} Blocker/Required`)

// ---------------------------------------------------------------- Phase 3: Revise (skipped on dryRun)
let revisions = 0
let lastValidate = pre.validatePass
if (dryRun) {
  log('dryRun: skipping revise — review-only')
} else {
  phase('Revise')
  while (openCount > 0 && revisions < maxRevisions) {
    if (budget && budget.total && budget.remaining() < reserve) {
      log(`revise: stopping — token budget reserve (${reserve}) reached with ${openCount} open finding(s)`)
      break
    }
    revisions++
    const open = findings.filter(isOpen)
    const rev = await agent(
      [
        `Revise OpenSpec change "${change}" to resolve these ${open.length} Blocker/Required finding(s). Apply ${reviewSkill}. Use Bash + edit the change's artifacts only.`,
        CONTEXT,
        `Findings to fix:`,
        ...open.map((f, i) => `${i + 1}. [${f.severity}] (${f.axis}) ${f.location} — ${f.problem} → ${f.suggestion}`),
        `Make the minimal edits that resolve each finding without introducing new requirements beyond scope. Keep deltas in ADDED/MODIFIED/REMOVED/RENAMED form. Then run: openspec validate "${change}" --strict --no-interactive and report validatePass.`,
        `Return applied (count fixed), validatePass, and remaining (any Blocker/Required you could NOT resolve, with the reason in problem and the original axis preserved).`,
      ].join('\n'),
      { schema: REVISE, label: `revise:${revisions}`, phase: 'Revise', agentType: 'general-purpose' },
    )
    if (!rev) { log(`revise:${revisions} returned null — stopping`); break }
    lastValidate = rev.validatePass
    findings = (rev.remaining || []).map((f) => ({ ...f, axis: f.axis || 'revise' }))
    openCount = findings.filter(isOpen).length
    log(`revise:${revisions}: applied ${rev.applied}, validate ${rev.validatePass ? 'pass' : 'FAIL'}, ${openCount} open finding(s) left`)
  }
}

// ---------------------------------------------------------------- Phase 4: Report
phase('Report')
const verdict = openCount === 0 ? 'approve' : 'revise'
const reportLines = findings.map((f) => `- **${f.severity}** _(${f.axis})_ ${f.location} — ${f.problem}${f.suggestion ? ` → ${f.suggestion}` : ''}`)
const rep = await agent(
  [
    `Write the spec-review report for OpenSpec change "${change}" to "${pre.changeRoot}/review/REVIEW.md" (create the dir). Use Bash/Write.`,
    `Header: "# Spec review — ${change}${date ? ` (${date})` : ''}" then "Verdict: ${verdict.toUpperCase()}", the axes reviewed (${AXES.map((a) => a.title).join('; ')}), revisions run (${revisions}), and openspec validate: ${lastValidate ? 'pass' : 'NOT passing'}.`,
    findings.length ? `Then a "## Findings" section listing each item verbatim:\n${reportLines.join('\n')}` : `Then "## Findings\\n_No Blocker/Required findings; spec is clean._"`,
    `Set verdict="approve" only if there are no Blocker/Required findings (open count is ${openCount}). Return written, the path, and the verdict.`,
  ].join('\n'),
  { schema: REPORTW, label: 'report', phase: 'Report', agentType: 'general-purpose' },
)

// ---------------------------------------------------------------- Report
return {
  stage: 'done',
  ok: verdict === 'approve',
  change,
  title,
  dryRun,
  verdict,
  revisions,
  validatePass: lastValidate,
  openFindings: openCount,
  totalFindings: findings.length,
  reviewReport: rep ? rep.path : `${pre.changeRoot}/review/REVIEW.md`,
  nextStep:
    verdict === 'approve'
      ? `Spec "${change}" is clean (no Blocker/Required). Review ${rep ? rep.path : 'review/REVIEW.md'}, then run /opsx:ship ${change} (or /opsx:apply).`
      : `Spec "${change}" still has ${openCount} Blocker/Required finding(s)${dryRun ? ' (dryRun — none auto-fixed)' : ` after ${revisions} revision(s)`}. See ${rep ? rep.path : 'review/REVIEW.md'} and fix them (or re-run with a higher maxRevisions).`,
}
