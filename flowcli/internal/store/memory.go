package store

import (
	"context"
	"fmt"
	"sync"
)

// Memory is a fixture-backed Store. Same project as the Svelte prototype, so
// the two front doors show the same data.
type Memory struct {
	mu       sync.RWMutex
	projects []Project
	nodes    []Node
	deps     []Dependency
	activity []ActivityEntry
	seq      int
}

func ver(agent AgentResult, name, when string) Verification {
	return Verification{Agent: agent, AgentName: name, AgentWhen: when, Human: NoVerdict}
}

func none() Verification { return Verification{Agent: NoReport, Human: NoVerdict} }

func NewMemory() *Memory {
	wp := func(id, title string, state WPState) Node {
		return Node{ID: id, ProjectID: "prj-travel", Type: WorkPackage, Title: title, Status: Ready, State: state}
	}
	task := func(id, parent, title string, st Status, cond string, desc []string, v Verification) Node {
		return Node{ID: id, ProjectID: "prj-travel", ParentID: parent, Type: Task, Title: title,
			Description: desc, Status: st, Condition: cond, Verification: v}
	}
	step := func(id, parent, title string, st Status, cond, note string) Node {
		return Node{ID: id, ProjectID: "prj-travel", ParentID: parent, Type: Step, Title: title,
			Status: st, Condition: cond, Note: note}
	}

	m := &Memory{
		projects: []Project{
			{ID: "prj-travel", Name: "Travel Webapp", Description: "Booking flow, auth and payments."},
			{ID: "prj-beer", Name: "Beer App", Description: "Tasting notes and cellar tracking."},
			{ID: "prj-docs", Name: "Developer Docs", Description: "Public API reference."},
		},
		nodes: []Node{
			wp("WP-AUTH", "Authentication Infrastructure", Active),
			task("T-1041", "WP-AUTH", "Session store on Redis with sliding expiry", Done, "redis-cli ping",
				[]string{
					"Replaces the in-process session map so sessions survive a deploy.",
					"Sliding expiry of 30 minutes with a hard cap of 12 hours. The cap matters more than the slide — without it a tab left open for a week keeps a session alive forever.",
				}, ver(Pass, "claude-code", "3d ago")),
			step("T-1041.1", "T-1041", "Provision Redis instance", Done, "terraform apply",
				"Single node with AOF persistence. A cluster is overkill until sessions outgrow one box."),
			step("T-1041.2", "T-1041", "Session serializer", Done, "pnpm test:session",
				"MessagePack rather than JSON — roughly 40% smaller for our shape."),
			step("T-1041.3", "T-1041", "Cut over behind flag", Done, "manual √", ""),

			task("T-1042", "WP-AUTH", "OAuth2 device-code flow for the CLI", Ready, "pnpm test:auth --grep device",
				[]string{
					"The TUI and MCP server both authenticate headlessly. Device-code is the only flow that works without a browser redirect on the machine running fctrl.",
					"The provider caps polling at one request every five seconds and returns slow_down when exceeded, so the client needs real backoff rather than a fixed interval.",
					"Refresh tokens land in the OS keyring — Keychain, libsecret, or Credential Manager — never on disk in plaintext.",
				}, ver(Pass, "claude-code", "2d ago")),
			step("T-1042.1", "T-1042", "Register client credentials in provider", Done, "manual √",
				"Done in the provider console; the client id lives in 1Password under \"fctrl oauth\"."),
			step("T-1042.2", "T-1042", "Poll token endpoint with backoff", Done, "curl -sf /device/token",
				"Exponential from 5s with a 30s ceiling, honouring the slow_down hint by adding 5s each time it appears."),
			step("T-1042.3", "T-1042", "Persist refresh token to OS keyring", Ready, "fctrl auth whoami",
				"Keychain on macOS, libsecret on Linux, Credential Manager on Windows. Headless Linux has no libsecret — fall back to a mode-0600 file and warn loudly."),
			step("T-1042.4", "T-1042", "Handle expired_token + slow_down", Blocked, "pnpm test:auth --grep slowdown",
				"Blocked on the error taxonomy being settled in T-1043."),
			step("T-1042.5", "T-1042", "Docs: CLI login walkthrough", Blocked, "file exists: docs/cli-login.md", ""),

			task("T-1043", "WP-AUTH", "Refresh-token rotation and reuse detection", Blocked, "pnpm test:auth --grep rotate",
				[]string{
					"Rotate on every refresh; a replayed token invalidates the whole family.",
					"Reuse almost always means theft. Killing the family logs the legitimate user out too, which is the correct trade.",
				}, none()),
			step("T-1043.1", "T-1043", "Token family table", Blocked, "", "One row per family, not per token. Tokens are derived."),
			step("T-1043.2", "T-1043", "Rotation on refresh", Blocked, "", ""),
			step("T-1043.3", "T-1043", "Reuse alarm", Blocked, "", "Page on it. A reuse event is a live incident, not a metric."),
			step("T-1043.4", "T-1043", "Backfill existing tokens", Blocked, "", ""),

			task("T-1044", "WP-AUTH", "Rate-limit the token endpoint", Blocked, "k6 run load/token.js",
				[]string{"Needs the metrics pipeline from Observability before limits can be tuned to anything but a guess."}, none()),
			step("T-1044.1", "T-1044", "Choose limiter algorithm", Blocked, "", "Sliding window over token bucket — bursty CLI logins are legitimate."),
			step("T-1044.2", "T-1044", "Wire to metrics", Blocked, "", ""),
			step("T-1044.3", "T-1044", "Load test at 5k rps", Blocked, "", ""),

			task("T-1045", "WP-AUTH", "Migrate legacy sessions", Deferred, "manual sign-off",
				[]string{"Parked until the legacy cohort drops below 2% of DAU. Currently 6.4% and falling about half a point a month."},
				ver(Stale, "claude-code", "3w ago")),
			step("T-1045.1", "T-1045", "Cohort report", Done, "manual √", "Refreshed weekly into the Observability dashboard."),
			step("T-1045.2", "T-1045", "Dual-read shim", Deferred, "", ""),
			step("T-1045.3", "T-1045", "Backfill job", Deferred, "", ""),
			step("T-1045.4", "T-1045", "Cutover", Deferred, "", ""),

			task("T-1046", "WP-AUTH", "Audit-log every token issuance", Ready, "pnpm test:auth --grep audit",
				[]string{"Compliance wants issuance, refresh and revocation events retained for a year."},
				ver(Fail, "claude-code", "4h ago")),
			step("T-1046.1", "T-1046", "Event schema", Ready, "", ""),
			step("T-1046.2", "T-1046", "Write path", Blocked, "", ""),
			step("T-1046.3", "T-1046", "Retention policy", Blocked, "", ""),

			wp("WP-BOOK", "Booking Engine", Active),
			task("T-2010", "WP-BOOK", "Availability search across provider adapters", Ready, "pnpm test:booking",
				[]string{
					"Fan-out to every enabled adapter, merge and de-duplicate by property id.",
					"A slow adapter must not hold the whole response. Each gets a 600ms budget and anything late is dropped from this query.",
				}, ver(Pass, "claude-code", "20m ago")),
			step("T-2010.1", "T-2010", "Adapter interface", Done, "tsc --noEmit", ""),
			step("T-2010.2", "T-2010", "Fan-out with timeout budget", Done, "pnpm test:booking --grep fanout", "allSettled with a per-adapter cancel."),
			step("T-2010.3", "T-2010", "Result de-duplication", Done, "pnpm test:booking --grep dedupe", "Provider id first, then normalised name plus coordinates within 50m."),
			step("T-2010.4", "T-2010", "Currency normalisation", Done, "manual √", ""),
			step("T-2010.5", "T-2010", "Cache layer", Ready, "redis-cli ping", "Five minute TTL keyed on the normalised query."),
			step("T-2010.6", "T-2010", "Adapter failure isolation", Blocked, "", ""),
			step("T-2010.7", "T-2010", "p95 under 800ms", Blocked, "k6 run load/search.js", ""),

			task("T-2011", "WP-BOOK", "Hold-and-confirm two-phase reservation", Blocked, "pnpm test:booking --grep hold",
				[]string{"Holds expire after 10 minutes; confirm is idempotent per hold id."}, none()),
			step("T-2011.1", "T-2011", "Hold table + TTL", Blocked, "", ""),
			step("T-2011.2", "T-2011", "Confirm endpoint", Blocked, "", ""),
			step("T-2011.3", "T-2011", "Expiry sweeper", Blocked, "", "Runs every 30s. A missed sweep is harmless; a double-release is not."),

			task("T-2012", "WP-BOOK", "Idempotency keys on the confirm endpoint", Blocked, "file exists: docs/idempotency.md",
				[]string{"Keys are scoped per authenticated principal, so this waits on the CLI auth flow."},
				ver(Fail, "claude-code", "3h ago")),
			step("T-2012.1", "T-2012", "Key storage + TTL", Blocked, "", ""),
			step("T-2012.2", "T-2012", "Replay response cache", Blocked, "", ""),
			step("T-2012.3", "T-2012", "Document the contract", Blocked, "", ""),

			wp("WP-PAY", "Payments", Active),
			task("T-3007", "WP-PAY", "Stripe webhook signature verification", Ready, "pnpm test:pay --grep webhook",
				[]string{"Reject unsigned or replayed webhook deliveries."}, none()),
			step("T-3007.1", "T-3007", "Verify signature header", Ready, "", ""),
			step("T-3007.2", "T-3007", "Replay window of 5m", Blocked, "", ""),
			task("T-3001", "WP-PAY", "Currency rounding rules", Done, "pnpm test:pay --grep round",
				[]string{"Banker's rounding at the line level, not the total."}, ver(Pass, "claude-code", "1w ago")),
			task("T-3011", "WP-PAY", "Refund reconciliation job", Blocked, "pnpm test:pay --grep refund",
				[]string{"Cannot reconcile refunds until reservations have a stable lifecycle."}, none()),

			wp("WP-OBS", "Observability", Planned),
			task("T-4002", "WP-OBS", "Structured log schema for the Rust core", Ready, "cargo test --package fctrl-core log",
				[]string{"One event shape for the engine, the web app and the TUI."}, none()),
			task("T-4000", "WP-OBS", "OTel collector bootstrap", Done, "kubectl get pods -l otel", nil, ver(Pass, "claude-code", "2w ago")),

			wp("WP-UI", "UI Redesign", Planned),
			task("T-5001", "WP-UI", "Dark-mode token audit", Deferred, "manual", nil, none()),

			wp("WP-LEGACY", "Legacy Import", WPDone),
			task("T-9001", "WP-LEGACY", "One-off CSV import", Done, "manual", nil,
				Verification{Agent: Pass, AgentName: "you", AgentWhen: "3w ago", Human: Accepted, HumanWhen: "3w ago"}),

			{ID: "WP-BEER", ProjectID: "prj-beer", Type: WorkPackage, Title: "Cellar tracking", Status: Ready, State: Active},
			{ID: "T-8001", ProjectID: "prj-beer", ParentID: "WP-BEER", Type: Task, Title: "Bottle inventory model",
				Description: []string{"Track what is in the cellar and when it should be drunk.",
					"One row per bottle: brew, vintage, purchase date, drink-by window and cellar position."},
				Status: Ready, Condition: "pnpm test:cellar", Verification: none()},
			{ID: "T-8001.1", ProjectID: "prj-beer", ParentID: "T-8001", Type: Step, Title: "Bottle schema", Status: Done,
				Condition: "sqlite3 cellar.db \".schema bottles\"", Note: "Single table with a partial index on drink_by."},
			{ID: "T-8001.2", ProjectID: "prj-beer", ParentID: "T-8001", Type: Step, Title: "Add/update bottle", Status: Done,
				Condition: "pnpm test:cellar --grep upsert", Note: "Upsert keyed on (brew, vintage, position)."},
			{ID: "T-8001.3", ProjectID: "prj-beer", ParentID: "T-8001", Type: Step, Title: "Drink-by window query", Status: Ready,
				Condition: "pnpm test:cellar --grep window", Note: ""},
			{ID: "T-8002", ProjectID: "prj-beer", ParentID: "WP-BEER", Type: Task, Title: "Tasting-note capture",
				Description: []string{"Free-text notes per bottle with a 0-5 rating."}, Status: Deferred,
				Condition: "pnpm test:cellar --grep tasting", Verification: none()},
			{ID: "T-8002.1", ProjectID: "prj-beer", ParentID: "T-8002", Type: Step, Title: "Note field on bottle", Status: Deferred,
				Condition: "", Note: ""},
			{ID: "T-8002.2", ProjectID: "prj-beer", ParentID: "T-8002", Type: Step, Title: "Rating breakdown view", Status: Deferred,
				Condition: "", Note: ""},
			{ID: "WP-DOCS", ProjectID: "prj-docs", Type: WorkPackage, Title: "API reference", Status: Ready, State: Active},
			{ID: "T-7001", ProjectID: "prj-docs", ParentID: "WP-DOCS", Type: Task, Title: "Generate from OpenAPI",
				Status: Blocked, Condition: "make docs", Verification: none()},
			{ID: "T-7001.1", ProjectID: "prj-docs", ParentID: "T-7001", Type: Step, Title: "Redoc template", Status: Done,
				Condition: "make docs && test -f out/index.html", Note: ""},
			{ID: "T-7001.2", ProjectID: "prj-docs", ParentID: "T-7001", Type: Step, Title: "Hosting bucket", Status: Done,
				Condition: "aws s3 ls s3://fctrl-docs", Note: "Static site on CloudFront with a 10-minute cache."},
			{ID: "T-7001.3", ProjectID: "prj-docs", ParentID: "T-7001", Type: Step, Title: "CI publish job", Status: Blocked,
				Condition: "gh run list --workflow=docs", Note: "Blocked on the token scope for the release bot."},
			{ID: "T-7002", ProjectID: "prj-docs", ParentID: "WP-DOCS", Type: Task, Title: "Changelog from git tags",
				Description: []string{"Derive the release notes from conventional-commit ranges."}, Status: Ready,
				Condition: "make changelog && test -s CHANGELOG.md", Verification: none()},
		},
		deps: []Dependency{
			{"T-1042", "T-1043"},
			{"T-1043", "T-1044"},
			{"WP-OBS", "T-1044"},
			{"T-2010", "T-2011"},
			{"T-2011", "T-2012"},
			{"T-1042", "T-2012"},
			{"WP-BOOK", "T-3011"},
			{"T-3007", "T-3011"},
		},
		activity: []ActivityEntry{
			{"a1", "T-1042", ActVerify, "claude-code", "2d ago", "Reported condition passed"},
			{"a2", "T-1042", ActStatus, "you", "2d ago", "BLOCKED → READY, the keyring work can start"},
			{"a3", "T-1042", ActEdit, "claude-code", "3d ago", "Marked step \"Poll token endpoint with backoff\" done"},
			{"a4", "T-1042", ActComment, "you", "4d ago", "Split the polling work out of T-1041 — it was doing too much."},
			{"a5", "T-1046", ActVerify, "claude-code", "4h ago", "Reported condition failed: 2 assertions in audit.spec.ts"},
			{"a6", "T-1046", ActComment, "claude-code", "4h ago", "The failures are in the retention assertions, not the write path."},
			{"a7", "T-2010", ActVerify, "claude-code", "20m ago", "Reported condition passed"},
			{"a8", "T-2012", ActVerify, "claude-code", "3h ago", "Reported condition failed: docs/idempotency.md not found"},
			{"a9", "T-1041", ActStatus, "you", "3d ago", "READY → DONE"},
		},
		seq: 100,
	}
	return m
}

func (m *Memory) Projects(_ context.Context) ([]Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Project, len(m.projects))
	copy(out, m.projects)
	// Fill each project's progress from the leaf nodes it owns: a task is a
	// leaf when it has no steps, otherwise each step is a leaf.
	for i := range out {
		// count leaves per project
		var done, total int
		for _, n := range m.nodes {
			if n.ProjectID != out[i].ID {
				continue
			}
			if n.Type == Step {
				total++
				if n.Status == Done {
					done++
				}
				continue
			}
			if n.Type == Task {
				// leaf task if it has no step children
				hasStep := false
				for _, c := range m.nodes {
					if c.ParentID == n.ID && c.Type == Step {
						hasStep = true
						break
					}
				}
				if !hasStep {
					total++
					if n.Status == Done {
						done++
					}
				}
			}
		}
		out[i].Progress.Done = done
		out[i].Progress.Total = total
	}
	return out, nil
}

func (m *Memory) Nodes(_ context.Context, projectID string) ([]Node, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Node, 0, len(m.nodes))
	for _, n := range m.nodes {
		if n.ProjectID == projectID {
			out = append(out, n)
		}
	}
	return out, nil
}

func (m *Memory) Dependencies(_ context.Context, projectID string) ([]Dependency, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	in := map[string]bool{}
	for _, n := range m.nodes {
		if n.ProjectID == projectID {
			in[n.ID] = true
		}
	}
	out := make([]Dependency, 0, len(m.deps))
	for _, d := range m.deps {
		if in[d.BlockerID] || in[d.BlockedID] {
			out = append(out, d)
		}
	}
	return out, nil
}

func (m *Memory) Activity(_ context.Context, projectID string) ([]ActivityEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	in := map[string]bool{}
	for _, n := range m.nodes {
		if n.ProjectID == projectID {
			in[n.ID] = true
		}
	}
	out := make([]ActivityEntry, 0, len(m.activity))
	for _, a := range m.activity {
		if in[a.NodeID] {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *Memory) SetStatus(_ context.Context, nodeID string, status Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.nodes {
		if m.nodes[i].ID == nodeID {
			prev := m.nodes[i].Status
			m.nodes[i].Status = status
			m.push(nodeID, ActStatus, fmt.Sprintf("%s → %s", prev, status))
			return nil
		}
	}
	return fmt.Errorf("node not found: %s", nodeID)
}

func (m *Memory) SetVerdict(_ context.Context, nodeID string, verdict HumanVerdict) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.nodes {
		if m.nodes[i].ID == nodeID {
			m.nodes[i].Verification.Human = verdict
			if verdict == NoVerdict {
				m.nodes[i].Verification.HumanWhen = ""
				m.push(nodeID, ActVerify, "Cleared the verification override")
			} else {
				m.nodes[i].Verification.HumanWhen = "just now"
				if verdict == Accepted {
					m.push(nodeID, ActVerify, "Accepted the condition as verified")
				} else {
					m.push(nodeID, ActVerify, "Rejected the condition")
				}
			}
			return nil
		}
	}
	return fmt.Errorf("node not found: %s", nodeID)
}

func (m *Memory) AddComment(_ context.Context, nodeID, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.push(nodeID, ActComment, text)
	return nil
}

func (m *Memory) CreateProject(_ context.Context, name, description string, _ bool) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := fmt.Sprintf("prj-%d", m.seq)
	m.projects = append(m.projects, Project{ID: id, Name: name, Description: description})
	return id, nil
}

func (m *Memory) CreateNode(_ context.Context, n NewNode) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := fmt.Sprintf("%s.%d", n.ParentID, m.seq)
	// Work packages are identified by a WP- prefix rather than a task-style id.
	if n.Type == WorkPackage {
		id = fmt.Sprintf("WP-%d", m.seq)
	}
	m.nodes = append(m.nodes, Node{
		ID:          id,
		ProjectID:   n.ProjectID,
		ParentID:    n.ParentID,
		Type:        n.Type,
		Title:       n.Title,
		Description: n.Description,
		Status:      Ready,
		Condition:   n.Condition,
	})
	return id, nil
}

// push prepends an activity entry. Caller holds the lock.
func (m *Memory) push(nodeID string, kind ActivityKind, text string) {
	m.seq++
	e := ActivityEntry{ID: fmt.Sprintf("a%d", m.seq), NodeID: nodeID, Kind: kind, Author: "you", When: "just now", Text: text}
	m.activity = append([]ActivityEntry{e}, m.activity...)
}
