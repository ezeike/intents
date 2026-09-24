# Functionality tree: block-builder (API) + block-designer (frontend)

Reference inventory of what a user could plausibly want to *do*, split into the two surfaces
that would each need their own intent resolution: the raw API (`block-builder`, an agent/API
caller's surface — MCP tools, direct RPC, or an LLM acting on a user's behalf) and the frontend
app (`block-designer` / "Loom", a human clicking around a UI). Compiled to ground future
`intents` package use-case work — this pairing (chat-driven spec authoring backend +
document-editing frontend) is close to the "product design app" example already used in
`docs/superpowers/specs/2026-09-24-intents-package-design.md`.

Format: a tree of `Domain > Category > Action`, phrased as user intents where the underlying
call isn't self-explanatory.

---

## A. API surface (block-builder) — 9 Connect-RPC services + MCP wrapper + stubbed A2A

```
block-builder
├─ Spec Authoring
│  ├─ Propose a change to this plugin's spec, in natural language (streaming, repeatable)
│  ├─ Accept the most recently proposed spec (commit it)
│  ├─ Update/replace the committed spec directly (bypass the LLM conversation)
│  ├─ Validate a spec (committed or draft) and get back its built IR
│  ├─ Fetch a plugin's current committed spec
│  ├─ List the known-audited rule primitives available to reference in a spec
│  └─ List every plugin in a project
├─ Codegen / Runs
│  ├─ Dry-run: see what a codegen run would do, without running it
│  ├─ Start a codegen run against a validated spec
│  ├─ Check a run's current status
│  ├─ Watch a run's node-by-node progress live (streaming)
│  ├─ Retry a failed/cancelled run (reusing cached completed nodes)
│  ├─ Cancel an in-progress run
│  ├─ Inspect one generated node's output before it's materialized to source
│  ├─ Inspect the current manifest (prefix allocations, generated-node hashes)
│  └─ Subscribe to every event for a plugin (specs committed, runs advancing) — session-scoped, forward-only
├─ Plugin Chat
│  └─ Have a general-purpose conversation about this plugin (separate from spec drafting/proposals)
├─ API Key Management  (operator-only; no MCP tool)
│  ├─ Create an API key for a user (only time the plaintext is ever shown)
│  ├─ List a user's keys (active + revoked)
│  └─ Revoke a key
├─ User Management
│  ├─ Get-or-create a user by email
│  ├─ Fetch a user
│  ├─ List every user in the org
│  ├─ Rename a user (display name)
│  └─ Delete a user (cascades: projects, plugins, conversations, messages)
├─ Project Management
│  ├─ Create a project
│  ├─ Fetch / list a user's projects
│  ├─ Rename a project
│  ├─ Delete a project (cascades: plugins, conversations, messages)
│  ├─ Fetch a project's settings
│  └─ Update project settings (merge-patch: set keys / delete keys)
├─ Conversation / Plugin-Metadata Management
│  ├─ Register a plugin (metadata row, prerequisite for spec-drafting)
│  ├─ List a project's plugins
│  ├─ Create / list / fetch / rename / delete a conversation (delete cascades: messages, FTS rows)
│  ├─ List a conversation's messages (paginated, oldest-first)
│  └─ Full-text search message content (scoped to a project or a conversation)
├─ Asset Catalog Management
│  ├─ Add a reusable catalog asset (project template, chain template, watcher, tx set, etc.)
│  ├─ List assets (optionally by kind)
│  ├─ Full-text / substring search assets
│  ├─ Delete a catalog asset
│  ├─ Select an asset into a project (with optional per-selection config)
│  ├─ List a project's selected assets
│  └─ Deselect an asset from a project
├─ Node Deployment & Operations
│  ├─ Fetch / list deployed nodes (per environment)
│  ├─ Fetch / list deployments (per node)
│  ├─ Deploy a node (build materialized plugin → image → push → run against existing chain)
│  ├─ Watch a deployment's live container logs (streaming, follow or drain-once)
│  ├─ Check a deployment's live status (polled, not last-written)
│  └─ Remove a node's deployment (tears down containers, keeps node identity for redeploy)
└─ Agent-to-Agent (A2A) — stubbed, wire surface only, all RPCs currently UNIMPLEMENTED
   ├─ Send a message / streaming message to the agent
   ├─ Get / list / cancel a task
   ├─ Subscribe to task updates
   └─ Discover the agent's capabilities (public, unauthenticated agent-card document)

Not an intent surface: tx-catalog protos (~70 pre-made chain-agnostic transaction message
types) — reference data the codegen pipeline draws from, surfaced only indirectly via the Asset
Catalog, never called directly.
```

## B. Frontend surface (block-designer / "Loom") — what a human does by clicking around

```
block-designer
├─ Identity
│  ├─ Sign in (enter a username, creates/resolves a user)
│  ├─ Switch identity (change username)
│  └─ Log out
├─ Projects
│  ├─ Create a project (from a template picker — no blank-project path)
│  ├─ Switch the active project
│  ├─ Rename a project
│  ├─ Delete a project (with confirm)
│  └─ Edit project settings (key/value editor)
├─ Chains (UI name for "plugin")
│  ├─ List / open a chain
│  ├─ Create a blank chain (validated name)
│  ├─ Create a chain from a template
│  └─ Start something new — Plugin (live), Extension/Contract/Token (stubbed, "coming soon")
├─ Conversations
│  ├─ Start a new conversation under a chain
│  ├─ Select / switch conversations
│  ├─ Rename a conversation
│  ├─ Delete a conversation (with confirm)
│  └─ Download a conversation as Markdown
├─ Spec-drafting Chat
│  ├─ Describe a plugin in natural language
│  ├─ Answer a clarifying question (via suggested-answer chips, or dismiss)
│  ├─ Copy a message to clipboard
│  ├─ Retry a failed turn (view raw error, retry without retyping)
│  ├─ Stop an in-flight streaming response
│  └─ Open a reference link the assistant surfaced
├─ Spec Review & Commit
│  ├─ View the current draft/proposal (read-only, dirty-line highlighted)
│  ├─ Search within the spec preview
│  ├─ Accept / discard a staged proposal
│  ├─ Commit the accepted draft to the backend
│  ├─ Build (kick off + live-watch a codegen run, node by node)
│  ├─ Deploy (UI present, disabled — backend not wired yet)
│  ├─ Edit the plugin's Chain ID directly (manual-edit form)
│  └─ View known-primitives reference list
├─ Assets Library
│  ├─ Browse assets, grouped by kind, collapsible
│  ├─ Preview an asset (description, tags, transactions, recursive dependency chain)
│  ├─ Toggle an asset into/out of the active project
│  └─ Select an asset (when picking a project/chain template)
├─ Nodes / Deployments
│  ├─ List deployment nodes (current environment)
│  ├─ View a node's deployments (target, deployed-at, live status)
│  ├─ View logs (toggle, switch between canopy/plugin container, live streaming)
│  └─ Remove a node (destructive, with confirm)
├─ Events Feed
│  ├─ Toggle a docked or floating live-events panel
│  ├─ View an LLM call, formatted (model, prompts, response, token/duration metadata)
│  └─ View an LLM call, raw JSON (exact wire payload)
├─ Global Search  (⌘K — wired to sample data, not yet live)
│  └─ Jump to a transaction / state object / module / event / error in the current spec
├─ Notifications  (sample data — placeholder)
│  └─ View recent notifications (validation failures, suggestions, collaborator edits, deploy results)
├─ Settings
│  ├─ General (autosave interval, default project)
│  ├─ Diagram (layout direction, node spacing)
│  ├─ Appearance (theme — live-wired)
│  ├─ Environment (backend target: dev/sandbox/prod)
│  └─ Shortcuts (reference list, read-only)
├─ Landing / Onboarding
│  └─ "Create your first project" CTA for a brand-new user
└─ API Keys
   └─ Button present, no dialog wired yet (stub)
```

---

## Notes for future intents-package work

- **The API and frontend surfaces are not 1:1.** The frontend wraps most of the API (spec
  authoring, chains/plugins, conversations, assets, nodes) behind guided UI flows, but several
  API capabilities have no frontend exposure at all (API key management, user/org management,
  `Plan` dry-run, `RetryRun`, `GetRunNodeOutput`/`GetManifest` introspection, the A2A surface).
  An API-facing intent router and a frontend-facing one would need different option sets, even
  though they're driving the same backend.
- **`Spec Authoring`'s `ProposeSpecChange`/`AcceptSpec` is effectively the real-world version of
  this doc's "new_proposal" example intent** — natural language in, a drafted/streamed proposal
  out, an explicit accept step before it's committed. `PluginRunService.StartRun` (the "Build"
  button) is the equivalent of the "functional_update" pattern once a spec is committed and
  needs to be materialized into code.
- **A large fraction of the frontend's stubbed features** (Deploy button, Global Search, Notifications,
  most Settings fields, API Keys dialog, Extension/Contract/Token creation) are not yet real —
  worth excluding from any intent spec until they're backed by working functionality, to avoid
  building resolution logic for actions that don't do anything yet.
- **Both surfaces are already too large for one flat classify** (9 API service groups × several
  actions each, 14 frontend functional areas) — this is exactly the case architecture.md §5
  describes: a hierarchical tree grouped by domain (Spec Authoring, Codegen, Projects, Assets,
  Deployment, ...) is the right shape, not a single wide option list.
