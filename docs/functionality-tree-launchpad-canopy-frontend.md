# Functionality tree: launchpad (API) + canopy-frontend (frontend)

Same exercise as `functionality-tree-block-builder-designer.md`, applied to Canopy's
chain-launch platform: `launchpad` (Go backend — REST API, WebSocket push, background workers)
and `canopy-frontend` (Next.js "Canopy Superapp" — launchpad UI, trading/AMM/orderbook,
multi-chain wallet, explorer, staking, governance, moderation).

Format: a tree of `Domain > Category > Action`, phrased as user intents where the underlying
call isn't self-explanatory.

---

## A. API surface (launchpad)

Only `api` (REST) and `websocket` are externally callable. `deposit-processor`, `graduator`,
`analytics-worker`, `volume-gen` are background workers (metrics port only, no business API).
`bridge-safe`, `manual-bridge-topup`, `kms-sign`, `ensure-bridge-keys`, `wcnpy-deploy`,
`reconcile-balance-history` are operator CLI tools, not services.

```
launchpad
├─ Auth & Sessions
│  ├─ Sign in with a wallet (SIWE: request nonce → verify signed message)
│  ├─ Log out / list active sessions / revoke a session
│  ├─ Link an additional wallet to an existing account
│  ├─ Verify and link an email address
│  └─ Resolve a bearer token to a wallet address (trusted first-party callers)
├─ User Accounts & Profile
│  ├─ Update profile
│  ├─ Export account data / delete account (GDPR-style)
│  ├─ Upload a profile asset (presigned upload + completion)
│  ├─ Link / unlink a GitHub account
│  ├─ Look up a public profile by wallet address
│  └─ Follow / unfollow a user, view followers/following/follow-counts
├─ Token Launches ("Chains")
│  ├─ List / search chains; validate a name+symbol before creating
│  ├─ Create a chain (launch), activate a draft, delete, archive
│  ├─ Edit chain metadata: description, template, socials, media assets
│  ├─ Link/update a source-code repository; validate a fork before linking
│  ├─ Get a live bonding-curve price quote
│  ├─ View a chain's transaction history, price/market-cap/metrics history, holders, accolades, stats
│  └─ View recent trades across all chains
├─ Comments / Social Engagement
│  ├─ Read / post / vote / delete comments on a chain
│  ├─ Attach an image to a comment
│  ├─ Favorite / follow a chain
│  └─ Watch a wallet address for activity
├─ Virtual Pool Trading / Bonding Curve
│  ├─ List pools / get pool state
│  ├─ Get a trade quote
│  ├─ Sell tokens against the bonding curve (buy route present but disabled)
│  └─ Transfer virtual-pool token balance
├─ Order Book / Instant Swap
│  ├─ View the order book / a specific order
│  ├─ Quote and create an instant swap
│  ├─ Submit deposit proof for an instant swap
│  └─ Submit intent to close an order
├─ Bridge (cross-chain, e.g. CNPY↔EVM)
│  ├─ View public bridge config / public transfer feed
│  ├─ View the caller's own bridge transfers
│  └─ Submit a bridge redeem request
├─ Wallet Management
│  ├─ Import / export a wallet
│  └─ Get / update / delete a managed wallet
├─ Portfolio
│  └─ Snapshot, overview, balances, performance, allocation, multi-chain summary
├─ Transactions
│  ├─ Submit a client-signed raw transaction (primary write path — backend never holds keys)
│  ├─ Estimate fee
│  └─ View history / pending / batch status / a transaction's detail+status
├─ Events / Rewards / Slashing
│  ├─ View wallet event/reward/slash history
│  └─ View public validator reward/slash history, public staking reward events
├─ Staking
│  └─ View caller's staking positions, rewards, unstaking queue
├─ Validators
│  └─ List / view detail / export validator data
├─ Address Book
│  └─ Add / list / search / favorite / update / delete a saved address
├─ Notifications
│  └─ List, unread count, mark one read, mark all read
├─ Faucet / Airdrop
│  └─ Check eligibility, claim testnet/faucet funds
├─ Explorer (read-only)
│  ├─ Browse blocks / transactions / swaps; universal search
│  ├─ Top accounts / trending chains / account detail
│  ├─ Network overview / telemetry / historical stats
│  ├─ Canopy network dashboard: audit, methodology, total/circulating supply
│  └─ Browse liquidity pools and LP positions
├─ Analytics / Metrics (public, read-only)
│  ├─ Liquidity: TVL growth/distribution, LP counts, fee APY, utilization
│  ├─ Trading: volume, unique traders, trade-size stats, cross-chain %, trader/builder leaderboards
│  └─ Staking: validator yield per chain
├─ Chain Parameters / Governance / Config
│  ├─ View protocol params (fee, governance, consensus, validator)
│  ├─ List governance proposals
│  └─ View public platform config (launchpad, CNPY price, bonding curve, moderation)
├─ Templates
│  └─ List / view a chain-launch template
├─ Moderation (moderator-only)
│  ├─ Delete / archive / restore a chain; list moderated chains
│  ├─ Remove a comment
│  ├─ Ban / unban an address; view banned-address list
│  └─ View a user-metrics moderator report
├─ Terminal Admin (separate service-to-service auth)
│  ├─ Read/update global launch-economics config (write needs dual approval)
│  ├─ Pool solvency report; fund a pool (dual approval)
│  ├─ Freeze/unfreeze a pool; toggle deposits/withdrawals
│  ├─ Block/unblock/force graduation; view graduation status
│  └─ Missed deposits, stuck payouts, failed refunds (bulk-reset ops, dual approval)
├─ Agent-to-Agent (A2A) — partially stubbed
│  ├─ Discover agent capabilities (public agent-card document)
│  └─ `tasks/get` only (demo task store; real domain-action wiring is future work per code comments)
└─ WebSocket (push)
   ├─ Subscribe / unsubscribe to a channel; report current page/tab being viewed
   ├─ Chain/block: block.finalized, block.indexed
   ├─ Trading: pool.update, pool.trade, candle.update
   ├─ Order book: oracle.orders, root.orders
   └─ Per-connection: portfolio.update, balances.update, user.notification, tx.notification
```

## B. Frontend surface (canopy-frontend — "Canopy Superapp")

Feature availability is gated by a `FEATURES_HIDE_MAP` flag list — gated-off features still show
their nav/tab with a "Coming Soon" overlay rather than disappearing.

```
canopy-frontend
├─ Home / Landing
│  └─ View trending-chains ticker, click through to a chain, navigate to any major section
├─ Explorer
│  ├─ View network overview/dashboard charts, telemetry
│  ├─ Search chains / addresses / blocks / tx hashes
│  ├─ Browse "New Launches", full chain directory, top accounts, top validators
│  ├─ Browse recent blocks / recent transactions
│  └─ Toggle currency display (USD/CNPY)
├─ Chains (browse + detail)
│  ├─ Browse the full chain list
│  ├─ View a chain's Overview / Holders / Transactions / Comments / Milestones / Code / Explorer tabs
│  ├─ Post / view comments; view achievements/milestones
│  ├─ View bonding-curve price chart, tokenomics, media gallery
│  ├─ Connect a GitHub repo to a chain; pick/edit a plugin language
│  ├─ Trade directly from the chain page (embedded Buy/Sell/Swap/Convert/Liquidity)
│  ├─ Edit a chain (creator-only) / archive a chain (creator/mod)
│  └─ View graduation progress tracker
├─ Launchpad (chain creation)
│  ├─ Browse listings: All / Trending / Favorites / My Chains / Graduated
│  └─ Create a chain via wizard: Main Info → Branding & Media → Links & Documentation →
│     Launch Settings → Review & Payment (save-as-draft and resume supported; validation,
│     duplicate-name/profanity checks, insufficient-balance guard, creator initial purchase)
├─ Creator Profile
│  └─ View a creator's bio, chains, ranking, revenue stats
├─ Graduation
│  └─ View graduation dashboard/queue, deployment status, liquidity migration, validator bootstrap
├─ Trade
│  ├─ Swap (buy/sell) with pair/pool selection and confirmation
│  ├─ Place / edit / cancel an order; view own open orders and fulfillment status
│  ├─ Convert between assets
│  ├─ Provide/withdraw liquidity from the trade UI
│  ├─ Bridge tokens (wrap/cross-chain)
│  ├─ Adjust slippage tolerance; select destination currency
│  └─ View AMM pool analytics (metric cards, charts, top pools table)
├─ AMM
│  └─ View dashboard/pools list, provide liquidity, swap, view pool analytics
├─ Liquidity
│  └─ Browse top pools, add/withdraw liquidity, view own positions, per-pool detail chart
├─ Wallet
│  ├─ Connect / create / select / switch / unlock a wallet
│  ├─ Send (with address-book suggestions) / Receive (address/QR)
│  ├─ Assets tab: search/sort holdings, portfolio overview
│  ├─ Activity tab: history, live pending-transaction badge
│  ├─ Staking tab: stake, unstake, cancel pending unstake, multi-committee select, claim rewards
│  ├─ Governance tab: view/interact with proposals
│  ├─ My Chains tab: chains this wallet created
│  ├─ Bridge tab: bridge in/out, view history
│  └─ Quick Buy/Sell/Convert/Swap embedded in wallet content
├─ Address Book
│  └─ Save/view a contact; recipient auto-suggestions when sending
├─ Faucet
│  └─ Request testnet tokens; view cooldown/status
├─ Governance
│  └─ View proposal summary; vote
├─ Validators
│  └─ Browse searchable validator list; view a validator's detail page
├─ Blocks / Transactions (explorer detail pages)
│  └─ Browse blocks/transactions lists; view a single block/transaction detail
├─ Accounts (address pages)
│  └─ View identity/header, portfolio + chart, open positions, transaction history
├─ Dashboard
│  └─ Aggregate metric cards, timeseries charts (export/configure), committee distribution
├─ Settings
│  ├─ Public profile edit
│  ├─ Wallet backup: import/export
│  ├─ Wallet linking: link an external wallet
│  ├─ Address book management
│  └─ Privacy & data preferences
├─ Moderation (moderator-only)
│  ├─ View/ban/unban addresses
│  ├─ Moderate chains (per-chain dropdown of actions)
│  └─ Export moderation data; refresh
├─ Compliance / Jurisdiction
│  └─ Automatic region gate; "ineligible jurisdiction" / "restricted" explanation pages
├─ Authentication
│  ├─ Log in (wallet-based)
│  ├─ Session guard for unauthenticated users; ban-check gate
│  └─ "Require auth" prompt surfaced mid-action when needed
├─ Developer Access
│  └─ Request/enter developer access; connect GitHub for chain-repo features
├─ Airdrop / Questing Claim Portal
│  └─ View claim banner with countdown, claim airdrop with destination-wallet selection,
│     create a wallet from within the claim flow (mobile)
├─ Legal
│  └─ View Privacy Policy / Terms of Service
└─ Cross-cutting
   ├─ Wallet-gated actions app-wide (connect+unlock required before trade/stake/send/create)
   ├─ Real-time updates via WebSocket with automatic polling fallback on disconnect
   ├─ Notification bell: list, mark-read, mark-all-read
   ├─ Profanity/content moderation check on user-submitted text
   └─ Dark/light theme toggle
```

---

## Notes for future intents-package work

- **This surface is an order of magnitude larger than block-builder/block-designer** — ~25 frontend
  functional areas and ~20+ API domains, several of which (Trade, Wallet, Launchpad) are
  themselves multi-step wizards or multi-tab hubs with 10+ actions apiece. A single top-level
  `classify` here would badly exceed the head-budget ceiling (`docs/context-budgets.md`) — this
  needs at least 3 levels of hierarchical descent (e.g. `domain → area → action`) before hitting
  leaves, not the 2 levels used in the block-builder/designer example.
- **Money-moving actions are dense here** — buy/sell/swap/bridge/stake/send/withdraw-liquidity
  all appear repeatedly across Trade, Wallet, AMM, and Liquidity. This is the strongest real-world
  case yet for `RiskTier` inheritance (architecture.md §3/§5): a whole subtree (`wallet →
  * anything that moves funds`) should inherit at least `RiskHigh`, rather than tagging each
  leaf action individually and risking one being missed.
- **Multi-step wizards (chain creation, especially) don't map cleanly onto a single intent
  resolution.** "Create a chain" isn't one classify+slot-fill pass — it's 5 sequential steps
  (Main Info, Branding, Links, Launch Settings, Review) with draft-save/resume and per-step
  validation already built into the frontend. This is a candidate for the "resumable session"
  extension flagged as an open question in `2026-09-24-intents-package-design.md`, rather than
  the one-shot `Resolve[T]` — a single intent call is a better fit for something like "buy 50
  CNPY of chain X" than for the whole launch wizard.
- **A meaningful amount of API surface has no frontend equivalent**: Terminal Admin (dual-approval
  pool operations), most Moderation API routes beyond what the `/mod` page exposes, A2A, and
  several CLI-only operator tools. Same pattern as block-builder/block-designer — an intent router
  scoped to "what the frontend user can ask for" is meaningfully narrower than "everything the
  API can do."
- **The WebSocket push surface (block/trade/portfolio updates) is not intent-resolution material**
  — it's server-initiated push, not something a user "intends" via free text. Worth excluding
  explicitly from any intent spec built from this inventory, same as excluding background workers.
- **Read-only browsing/viewing actions vastly outnumber state-changing ones** (Explorer, Dashboard,
  most of Chains/Accounts/Blocks/Transactions/Validators are pure viewing). These are good
  candidates for a fast, low-risk-tier classify path — the interesting design problems (slot-filling,
  risk gating, lookup/vector-search fallback) cluster almost entirely in Trade/Wallet/Launchpad.
