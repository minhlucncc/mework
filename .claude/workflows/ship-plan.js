export const meta = {
  name: 'ship-plan',
  description:
    'Plan the execution of an APPROVED OpenSpec change as a reviewable handoff under .handoff/<change>/. For EACH task in the change\'s tasks.md it emits TWO handoff tasks — a test/Red task (the test plan: which *_test.go, the assertions drawn from the spec scenarios) and a code/Green task (the production change) — wired so the code task depends on its test and the pair shares one commit group. Writes plan.json (the index), tasks/NN-a-test.md + NN-b-code.md, and a README.md of shared context. Honors args.local — when true, the plan.json carries localOnly=true so ship-code picks up the fully-local path (no gh, no remote push). Writes NO production code and creates NO branch. Idempotent: re-planning preserves any tasks already marked done. The output handoff is meant to be reviewed (and optionally hand-edited) before /opsx:ship-code executes it.',
  phases: [
    { title: 'Preflight', detail: 'openspec status + validate; read change artifacts' },
    { title: 'Plan', detail: 'derive test+code pairs, write .handoff/<change>/' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
const change = A.change
const date = A.date // YYYY-MM-DD — passed in; Date.now()/new Date() are unavailable in scripts
const tdd = A.tdd === undefined ? true : !!A.tdd // default test-first
const local = A.local === true // when true, downstream ship-code uses the fully-local path

if (!change || typeof change !== 'string') {
  throw new Error('ship-plan requires args { change, date, tdd?, local? }; got typeof=' + (typeof args) + ' keys=' + Object.keys(A).join(','))
}
if (!/^[a-z0-9][a-z0-9-]*$/.test(change)) throw new Error('Unsafe change name (expected kebab-case slug): ' + change)
if (date && !/^\d{4}-\d{2}-\d{2}$/.test(date)) throw new Error('Unsafe date (expected YYYY-MM-DD): ' + date)
const handoffDir = `.handoff/${change}`

// ---------------------------------------------------------------- schemas
const PREFLIGHT = {
  type: 'object', additionalProperties: false,
  required: ['ok', 'reason', 'changeRoot', 'proposalPath', 'designPath', 'tasksPath', 'specPaths', 'changeTasks'],
  properties: {
    ok: { type: 'boolean', description: 'true only if openspec status + validate succeeded' },
    reason: { type: 'string' },
    changeRoot: { type: 'string' },
    proposalPath: { type: ['string', 'null'] },
    designPath: { type: ['string', 'null'] },
    tasksPath: { type: 'string' },
    specPaths: { type: 'array', items: { type: 'string' } },
    title: { type: 'string', description: 'human-readable change title from proposal.md' },
    changeTasks: {
      type: 'array',
      description: 'the parsed checklist items from the change tasks.md, in order',
      items: {
        type: 'object', additionalProperties: false, required: ['n', 'text', 'done'],
        properties: {
          n: { type: 'string', description: 'two-digit ordinal, e.g. "01"' },
          text: { type: 'string', description: 'the task line text' },
          done: { type: 'boolean', description: 'already ticked [x]' },
        },
      },
    },
  },
}
const PLAN = {
  type: 'object', additionalProperties: false, required: ['handoffDir', 'pairs', 'taskFiles', 'notes'],
  properties: {
    handoffDir: { type: 'string' },
    pairs: { type: 'integer', description: 'number of change tasks planned (each = one test+code pair)' },
    taskFiles: { type: 'array', items: { type: 'string' }, description: 'all tasks/NN-*.md files written' },
    notes: { type: 'string' },
  },
}

// ---------------------------------------------------------------- handoff format (single source of truth for the planner)
const HANDOFF_FORMAT = [
  `HANDOFF FORMAT — write these files under "${handoffDir}/" (create dirs as needed):`,
  ``,
  `1. ${handoffDir}/plan.json — the machine-readable index:`,
  `   { "change": "${change}", "title": "<title>", "changeRoot": "<changeRoot>",`,
  `     "localOnly": ${local ? 'true' : 'false'},`,
  `     "tasks": [ <one object per handoff task> ] }`,
  `   Each task object (additionalProperties NOT allowed):`,
  `   { "id": "01a", "pair": "01", "role": "test"|"code", "slug": "<kebab>", "title": "<imperative>",`,
  `     "status": "todo", "depends_on": ["..."], "deliverable": "<repo-relative path>",`,
  `     "verify": "<one-line checkable acceptance>", "skipRed": false }`,
  `   Rules: ids are "<pair><a|b>" (a=test, b=code). For EACH change task in tasks.md emit a`,
  `   pair: an "a" test task (role:test, depends_on:[]) and a "b" code task (role:code,`,
  `   depends_on:["<pair>a"]). The code task's deliverable is the production .go file; the test`,
  `   task's deliverable is the *_test.go file. pair = the two-digit change-task ordinal.`,
  ``,
  `2. ${handoffDir}/tasks/<id>-<a-test|b-code>.md per task (filename: "01-a-test.md", "01-b-code.md"):`,
  `   --- (YAML frontmatter)`,
  `   id: "01a"`,
  `   pair: "01"`,
  `   role: test`,
  `   slug: <kebab>`,
  `   title: <imperative one-line>`,
  `   status: todo`,
  `   depends_on: []`,
  `   deliverable: <repo-relative path>`,
  `   verify: <one-line checkable acceptance>`,
  `   skipRed: false`,
  `   ---`,
  `   ## Goal`,
  `   <1-3 sentences>`,
  `   ## Context`,
  `   Read ../README.md for shared context. <pointers to the proposal/design and the exact`,
  `   delta-spec scenario(s) this task realizes.>`,
  `   ## Acceptance criteria`,
  `   - [ ] <concrete, checkable item — for a test task, the cases to assert (table rows);`,
  `         for a code task, "the NNa test passes" + behavior + make vet/test green>`,
  `   ## Output log`,
  `   <!-- appended by ship-code; leave empty -->`,
  ``,
  `3. ${handoffDir}/README.md — shared context:`,
  `   # ${change} — <title>`,
  `   ## Summary  (2-5 sentences from the proposal)`,
  `   ## Artifacts  (links: proposal, design, tasks.md, delta specs)`,
  `   ## Task index  (table: pair | test deliverable | code deliverable | from change task)`,
  `   ## Conventions  (stdin-not-argv, table-driven tests, make vet/test, -p 1, evidence dir)`,
  ``,
  `IDEMPOTENCY: if ${handoffDir}/plan.json already exists, read it first and PRESERVE the`,
  `status of any task already marked "done" (do not regress it to "todo"); you may rewrite`,
  `the rest. Keep plan.json tasks[*] and the tasks/*.md frontmatter in sync.`,
].join('\n')

const SKILL = (name) => `the \`${name}\` skill (.claude/skills/${name}/SKILL.md)`

// ---------------------------------------------------------------- Phase 1: Preflight
phase('Preflight')
const pre = await agent(
  [
    `Preflight planning for OpenSpec change "${change}". Use Bash. Steps:`,
    `1. openspec status --change "${change}" --json — parse changeRoot, proposal/design/tasks artifact paths, and the delta-spec paths. Read proposal.md for a one-line title.`,
    `2. openspec validate "${change}" --strict (fall back to non-strict) — MUST pass; if not, ok=false + reason and STOP.`,
    `3. Read the change's tasks.md and return its checklist items in order as changeTasks (n = two-digit ordinal by position, text = the line, done = whether it is [x]).`,
    `Do NOT create a branch and do NOT edit files. Return the structured result.`,
  ].join('\n'),
  { schema: PREFLIGHT, label: 'preflight', phase: 'Preflight', agentType: 'general-purpose' },
)
if (!pre || !pre.ok) {
  return { stage: 'preflight', ok: false, reason: pre ? pre.reason : 'preflight agent returned null', change }
}
const title = pre.title || change
const openTasks = (pre.changeTasks || []).filter((t) => !t.done)
log(`preflight ok — ${pre.changeTasks.length} change task(s), ${openTasks.length} open; planning ${openTasks.length} test+code pair(s)`)
if (!openTasks.length) {
  return { stage: 'plan', ok: true, change, handoffDir, pairs: 0, taskFiles: [], notes: 'no open change tasks — nothing to plan', nextStep: `No open tasks in ${change}'s tasks.md — nothing to plan.` }
}

// ---------------------------------------------------------------- Phase 2: Plan (write the handoff)
phase('Plan')
const CONTEXT = [
  `Change "${change}" — "${title}". Ground the plan in these artifacts (read them):`,
  pre.proposalPath ? `- proposal (what & why): ${pre.proposalPath}` : '',
  pre.designPath ? `- design (how): ${pre.designPath}` : '',
  `- tasks (the checklist to expand): ${pre.tasksPath}`,
  pre.specPaths && pre.specPaths.length ? `- delta specs (the scenarios the tests must assert): ${pre.specPaths.join(', ')}` : '- delta specs: (none)',
].join('\n')

const plan = await agent(
  [
    `Write the execution handoff for OpenSpec change "${change}" into "${handoffDir}/". Apply ${SKILL('planning-and-task-breakdown')} and ${SKILL('test-driven-development')} (the test tasks are the Red plan).`,
    CONTEXT,
    `For EACH OPEN task in the change's tasks.md (${openTasks.map((t) => t.n + '. ' + t.text).join(' | ')}), produce a TEST task and a CODE task:`,
    `- The TEST task captures the failing test to write first: which *_test.go file, and the concrete assertions/table cases derived from the relevant delta-spec scenarios + acceptance criteria. Its verify line asserts the test fails before implementation (Red).`,
    `- The CODE task captures the minimal production change to make that test pass (which .go file, the behavior). It depends_on the test task. Its verify line: the test passes (Green) + make vet/test green.`,
    `If a change task is doc-only / pure-config with no testable behavior, still emit the pair but set the test task's skipRed=true with a one-line reason in its Goal (never skip silently).`,
    `changeRoot is ${pre.changeRoot}. Write every file per the format below, then return the handoff dir, the pair count, and the list of task files.`,
    ``,
    HANDOFF_FORMAT,
  ].join('\n'),
  { schema: PLAN, label: 'write-handoff', phase: 'Plan', agentType: 'general-purpose' },
)
if (!plan) return { stage: 'plan', ok: false, reason: 'plan agent returned null', change, handoffDir }
log(`plan: wrote ${plan.pairs} pair(s), ${plan.taskFiles.length} task file(s) under ${handoffDir}`)

// ---------------------------------------------------------------- Report
return {
  stage: 'done',
  ok: true,
  change,
  title,
  handoffDir,
  pairs: plan.pairs,
  taskFiles: plan.taskFiles,
  localOnly: local,
  notes: plan.notes,
  nextStep: `Handoff written to ${handoffDir}/ (${plan.pairs} test+code pair(s))${local ? ' — localOnly=true' : ''}. Review/edit the tasks, then run /opsx:ship-code ${change}${local ? ' --local' : ''} to implement them (one red+green commit per pair).`,
}
