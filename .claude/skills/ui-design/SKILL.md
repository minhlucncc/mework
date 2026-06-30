---
name: ui-design
description: Guides the AI through UI/visual design for user-facing OpenSpec changes. Covers wireframes, component trees, UI states, user flows, visual decisions, responsive/accessible design, and connecting UI design to implementation tasks. Use when a change has a visible surface (new screens, component changes, UX improvements) and you need to produce ui.md.
---

# UI Design

## Overview

UI design lives in `openspec/changes/<cNNNN-slug>/ui.md` — a sibling of `design.md` (architecture/logic) and `tasks.md`. While `design.md` captures architecture decisions, dependency graphs, and technical risks, `ui.md` captures the visual and interaction design: wireframes, component hierarchy, UI states (loading/empty/error/populated/edge cases), user flows, visual decisions, responsive behavior, and accessibility.

A good `ui.md` makes implementation tasks concrete: each UI state becomes an acceptance criterion, each component maps to a task file, and the wireframes give the implementer an unambiguous target.

## When to Use

- Your OpenSpec change introduces or modifies user-facing screens
- You are adding or changing interactive components (forms, lists, modals, navigation, buttons)
- The change affects layout, responsive behavior, or accessibility
- You need to document UI states (loading, empty, error, edge cases)
- The human explicitly requested UI design work

**When NOT to use:** Purely backend/data-layer changes, spec-only changes, infrastructure work, CLI-only changes. If the change has no visible surface, state "no UI surface — ui.md not created" in the propose output.

## The Design Process

### Step 1: Explore Existing Patterns

Before proposing any UI, explore the codebase to understand what already exists:

1. **Read the frontend package** — e.g., `apps/portal/src/`, `apps/web/src/` — to understand component naming conventions, file organization, and pattern libraries
2. **Check for design tokens** — `theme.ts`, `tokens.css`, `tailwind.config.*`, design system files in `components/ui/` or `lib/`
3. **Study existing components** — look for patterns: how do they handle error states? Loading states? Empty states? Form validation? What's the import style?
4. **Check capability specs** — read `openspec/specs/<capability>/spec.md` to understand the intended behavior the UI must support
5. **Note the tech stack** — React/Vue/Svelte? Tailwind/CSS Modules/Styled Components/vanilla CSS? Responsive or mobile-first? shadcn/ui, Radix, MUI, or custom?

Document findings as "Existing patterns observed:" in `ui.md` so the implementer knows what conventions to follow.

### Step 2: Create Wireframes / ASCII Art

Use ASCII art in fenced code blocks. Keep diagrams 72–80 characters wide. Each screen or significant state gets its own diagram.

```
+----------------------------------------------------------------------+
|  [Logo]  [Nav: Home | Search | Settings]               [Avatar v]   |
+----------------------------------------------------------------------+
|                                                                      |
|  +-----------+  +--------------------------------------------------+ |
|  | Filters   |  |  Search Results                                  | |
|  |           |  |                                                  | |
|  | [v Type]  |  |  +----------------------------------------------+ | |
|  | [ ] Active|  |  | ResultCard (title, excerpt, timestamp)      | | |
|  | [ ] Draft |  |  | [View] [Edit]                               | | |
|  |           |  |  +----------------------------------------------+ | |
|  | [Apply]   |  |  +----------------------------------------------+ | |
|  +-----------+  |  | ResultCard (title, excerpt, timestamp)      | | |
|                 |  | [View] [Edit]                               | | |
|                 |  +----------------------------------------------+ | |
|                 |                                                  | |
|                 |  [  <  Page 2 of 8  >  ]                        | |
|                 +--------------------------------------------------+ |
+----------------------------------------------------------------------+
```

Use the following conventions:
- `[Button]` or `[Label]` for interactive elements
- `[v Dropdown]` for dropdowns (v = chevron)
- `[x]` / `[ ]` for checkboxes
- `(...)` for radio buttons
- `+--...--+` for bordered containers
- `|` for vertical edges

### Step 3: Document Component Tree

Show the hierarchy of UI components using indentation. Annotate each leaf component with its possible states in brackets.

```
SearchPage
  SearchHeader
    SearchInput    [focused / blurred / filled / validation-error]
    SearchButton   [enabled / disabled / loading]
  SearchFilters
    FilterDropdown [unselected / selected / no-results]
    ToggleGroup    [all / active / archived]
  ResultList
    LoadingSpinner
    ResultCard[]   [populated / hovered / selected]
      CardTitle
      CardExcerpt
      CardMeta
      ActionMenu   [open / closed]
    EmptyState
      Illustration
      CallToAction
    ErrorState
      ErrorMessage
      RetryButton
  Pagination
    PageLink[]     [active / inactive / disabled]
```

Mark repeated elements with `[]` (e.g., `ResultCard[]`).

### Step 4: Enumerate UI States

For each interactive component, enumerate all relevant states:

| State | What the user sees |
|-------|-------------------|
| **Loading** | Skeleton, spinner, shimmer, or progressive placeholder while data fetches |
| **Empty** | Message + illustration + call-to-action when there's nothing to display |
| **Error** | Error message + retry button + fallback content on fetch/operation failure |
| **Populated** | The normal data-present state |
| **Disabled** | Greyed-out/inactive state while conditions aren't met |
| **Focused** | Keyboard/touch focus indicator for inputs and interactive elements |
| **Hovered** | Hover state for clickable elements |
| **Edge cases** | Long text truncation, rapid clicks, slow network, expired session, permissions denied, zero results |

### Step 5: Design User Flows

Write step-by-step flows for key scenarios. Cover the happy path, then branch for empty, error, and edge cases.

```
## Flow: Search and view a result

**Happy path:**
1. User lands on SearchPage
2. User types query in SearchInput (state: filled)
3. User clicks SearchButton (state: loading)
4. ResultList shows LoadingSpinner while fetching
5. Results arrive: ResultList populates with ResultCard[] items
6. User clicks "View" on a ResultCard → navigates to detail view

**Empty:**
- Search returns no results → ResultList shows EmptyState with "No results" message + suggestion to broaden query

**Error:**
- Network failure → ResultList shows ErrorState with error description + RetryButton
- User clicks Retry → re-fetch, back to Loading → Populated or Error again

**Edge cases:**
- 1000+ results → paginated; PageLinks show page 1 of N
- Very long query text → SearchInput truncates with ellipsis
- Rapid successive searches → debounce or cancel previous request
```

### Step 6: Record Visual Decisions

Record decisions that deviate from project defaults. If the project has a design system / theme tokens, reference the tokens rather than inventing new values.

- Color choices for new accent states (reference theme tokens where possible)
- Typography: heading vs body sizes, font weights
- Spacing: padding, margin, gap values (use the project's spacing scale)
- Border-radius: sharp, rounded, or pill shapes
- Icons: which icon set, specific icon names
- Animation: transitions, hover effects, loading animation styles
- Shadows: card elevation, modal backdrop

### Step 7: Responsive + Accessibility

**Responsive behavior:**
- Breakpoints: at what widths does the layout change?
- Collapsing: which sections stack, hide, or become toggles on mobile?
- Navigation: hamburger menu vs sidebar vs top nav at each breakpoint
- Tables/grids: horizontal scroll, card layout on small screens, column reordering

**Accessibility:**
- Keyboard navigation: tab order, focus indicators, escape-to-close modals, arrow-key navigation in lists
- ARIA: roles (`navigation`, `main`, `dialog`, `alert`), labels (`aria-label`, `aria-labelledby`), live regions (`aria-live="polite"` for dynamic content)
- Focus management: where focus lands when a modal opens/closes, when a page navigates
- Color contrast: verify against WCAG AA (4.5:1 for normal text, 3:1 for large text)
- Screen reader flow: linear reading order, skip-to-content link, heading hierarchy

## Template: ui.md

```markdown
# UI Design — <Feature Name>

## Overview
[One paragraph — what user-facing capability this delivers, which screens or
components it affects, who the primary user is.]

## Existing Patterns Observed
[Optional — what conventions, components, and design tokens were found in the
codebase that this design builds on or diverges from.]

## Wireframes / Layout
[ASCII-art diagram(s) of each screen or major component state.]

## Component Tree
[Indented hierarchy of components with state annotations.]

## User Flows
[Step-by-step walkthroughs: happy path, empty, error, edge cases.]

## UI States
| Component | Loading | Empty | Error | Populated | Edge Cases |
|-----------|---------|-------|-------|-----------|------------|
| SearchInput | skeleton | — | — | filled | long query |
| ResultList | spinner | no-results msg | error+retry | cards | 1000+ results |

## Visual Decisions
[List any visual choices that deviate from project defaults.]

## Responsive Behavior
[Breakpoints, layout changes, mobile navigation pattern.]

## Accessibility Considerations
[Keyboard nav, ARIA, focus management, contrast, screen reader flow.]

## Open UI Questions
[Anything needing human input before implementation.]
```

## Connecting to Implementation Tasks

UI design flows into implementation tasks in `tasks.md`. Each unit of work should reference `ui.md` sections:

```markdown
## Task [3]: SearchResultList with all states (toolchain: ts)

**Description:** Build the SearchResultList component with loading skeleton,
empty-state message, error state with retry, and populated card list.
Follow ui.md → Component Tree for the hierarchy and ui.md → UI States for
the behavior of each state.

**Acceptance criteria:**
- [ ] Loading: shimmer/skeleton renders while data is being fetched
- [ ] Empty: "No results found" message with suggestion text
- [ ] Error: error message + RetryButton on fetch failure
- [ ] Populated: renders ResultCard for each result item
- [ ] Edge: handles 0, 1, and 1000+ results without layout breakage
- [ ] 1000+ results correctly paginated per ui.md → User Flows
```

## Verification

Before concluding UI design, confirm:

- [ ] Wireframes cover all key screens and their significant states
- [ ] Component tree documents the full hierarchy with state annotations
- [ ] User flows cover happy path, empty, error, and at least one edge case
- [ ] UI states table is complete (loading / empty / error / populated / edge cases)
- [ ] Responsive breakpoints are stated
- [ ] Accessibility considerations are noted (keyboard, ARIA, contrast)
- [ ] Visual decisions reference the existing design system where applicable
- [ ] Implementation tasks in `tasks.md` map to `ui.md` sections
- [ ] The human has reviewed and approved the UI design before implementation begins
