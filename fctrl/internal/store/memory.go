package store

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Memory is the fixture store. Everything lives in two slices, exactly the
// shape the real tables have.
type Memory struct {
	mu       sync.RWMutex
	projects []Project
	nodes    []Node
	deps     []Dependency
	canned   map[string]VerifyResult
}

var _ Store = (*Memory)(nil)

func NewMemory() *Memory {
	m := &Memory{
		canned: map[string]VerifyResult{
			"T-1042": VerifyPass,
			"T-2012": VerifyFail,
			"T-3007": VerifyPass,
		},
	}
	m.seed()
	return m
}

func (m *Memory) Projects(_ context.Context) ([]Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Project, len(m.projects))
	copy(out, m.projects)
	return out, nil
}

func (m *Memory) Nodes(_ context.Context, projectID string) ([]Node, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []Node{}
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
	inProject := map[string]bool{}
	for _, n := range m.nodes {
		if n.ProjectID == projectID {
			inProject[n.ID] = true
		}
	}
	out := []Dependency{}
	for _, d := range m.deps {
		if inProject[d.BlockerID] || inProject[d.BlockedID] {
			out = append(out, d)
		}
	}
	return out, nil
}

// SetStatus writes the one node. No cascade: the Rust core owns that, and
// faking it here would teach the prototype the wrong lesson.
func (m *Memory) SetStatus(_ context.Context, nodeID string, s Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.nodes {
		if m.nodes[i].ID == nodeID {
			m.nodes[i].Status = s
			return nil
		}
	}
	return errors.New("node not found: " + nodeID)
}

// Verify fakes a condition run: a short pause, then a canned result.
func (m *Memory) Verify(_ context.Context, nodeID string) (VerifyResult, error) {
	time.Sleep(900 * time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	res, ok := m.canned[nodeID]
	if !ok {
		res = VerifyPass
	}
	for i := range m.nodes {
		if m.nodes[i].ID == nodeID {
			m.nodes[i].LastResult = res
			m.nodes[i].LastRun = "just now"
			break
		}
	}
	return res, nil
}

// Dependents returns the nodes this one blocks — used by the confirm bar to
// show what the core *would* re-evaluate.
func (m *Memory) Dependents(nodeID string) []Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	byID := map[string]Node{}
	for _, n := range m.nodes {
		byID[n.ID] = n
	}
	out := []Node{}
	for _, d := range m.deps {
		if d.BlockerID == nodeID {
			if n, ok := byID[d.BlockedID]; ok {
				out = append(out, n)
			}
		}
	}
	return out
}

func (m *Memory) seed() {
	now := time.Now().Unix()
	m.projects = []Project{
		{ID: "prj-travel", Name: "Travel Webapp", Description: "Booking flow, auth and payments.", CreatedAt: now},
		{ID: "prj-beer", Name: "Beer App", Description: "Tasting notes and cellar tracking.", CreatedAt: now},
		{ID: "prj-docs", Name: "Developer Docs", Description: "Public API reference.", CreatedAt: now},
	}

	wp := func(id, name string, state WPState) Node {
		return Node{ID: id, ProjectID: "prj-travel", Type: TypeWorkPackage, Title: name, Status: StatusReady, State: state, LastResult: VerifyNone}
	}
	task := func(id, parent, title string, st Status, cond, desc string, res VerifyResult, run string) Node {
		return Node{ID: id, ProjectID: "prj-travel", ParentID: parent, Type: TypeTask, Title: title, Status: st, Condition: cond, Description: desc, LastResult: res, LastRun: run}
	}
	step := func(id, parent, title string, st Status, cond string) Node {
		return Node{ID: id, ProjectID: "prj-travel", ParentID: parent, Type: TypeStep, Title: title, Status: st, Condition: cond, LastResult: VerifyNone}
	}

	m.nodes = []Node{
		wp("WP-AUTH", "Authentication Infrastructure", StateActive),
		task("T-1041", "WP-AUTH", "Session store on Redis with sliding expiry", StatusDone,
			"redis-cli ping", "Replaces the in-process session map. Sliding expiry of 30m, hard cap 12h.", VerifyPass, "3d ago"),
		step("T-1041.1", "T-1041", "Provision Redis instance", StatusDone, "terraform apply"),
		step("T-1041.2", "T-1041", "Session serializer", StatusDone, "go test ./session"),
		step("T-1041.3", "T-1041", "Cut over behind flag", StatusDone, "manual"),

		task("T-1042", "WP-AUTH", "OAuth2 device-code flow for the CLI", StatusReady,
			"pnpm test:auth --grep device",
			"The TUI and MCP server both authenticate headlessly. Device-code is the only flow that works without a browser redirect on the machine running fctrl.",
			VerifyStale, "2d ago"),
		step("T-1042.1", "T-1042", "Register client credentials in provider", StatusDone, "manual"),
		step("T-1042.2", "T-1042", "Poll token endpoint with backoff", StatusDone, "curl -sf /device/token"),
		step("T-1042.3", "T-1042", "Persist refresh token to OS keyring", StatusReady, "fctrl auth whoami"),
		step("T-1042.4", "T-1042", "Handle expired_token + slow_down", StatusBlocked, "pnpm test:auth --grep slowdown"),
		step("T-1042.5", "T-1042", "Docs: CLI login walkthrough", StatusBlocked, "file exists: docs/cli-login.md"),

		task("T-1043", "WP-AUTH", "Refresh-token rotation + reuse detection", StatusBlocked,
			"pnpm test:auth --grep rotate", "Rotate on every refresh; a replayed token invalidates the whole family.", VerifyNone, ""),
		step("T-1043.1", "T-1043", "Token family table", StatusBlocked, ""),
		step("T-1043.2", "T-1043", "Rotation on refresh", StatusBlocked, ""),
		step("T-1043.3", "T-1043", "Reuse alarm", StatusBlocked, ""),

		task("T-1044", "WP-AUTH", "Rate-limit the token endpoint", StatusBlocked,
			"k6 run load/token.js", "Needs the metrics pipeline from Observability before limits can be tuned.", VerifyNone, ""),
		task("T-1045", "WP-AUTH", "Migrate legacy sessions", StatusDeferred,
			"manual sign-off", "Parked until the legacy cohort drops below 2% of DAU.", VerifyNone, ""),

		wp("WP-BOOK", "Booking Engine", StateActive),
		task("T-2010", "WP-BOOK", "Availability search across provider adapters", StatusReady,
			"pnpm test:booking", "Fan-out to every enabled adapter, merge and de-duplicate by property id.", VerifyPass, "20m ago"),
		step("T-2010.1", "T-2010", "Adapter interface", StatusDone, "tsc --noEmit"),
		step("T-2010.2", "T-2010", "Fan-out with timeout budget", StatusDone, "pnpm test:booking --grep fanout"),
		step("T-2010.3", "T-2010", "Result de-duplication", StatusDone, "pnpm test:booking --grep dedupe"),
		step("T-2010.4", "T-2010", "Cache layer", StatusReady, "redis-cli ping"),
		step("T-2010.5", "T-2010", "p95 under 800ms", StatusBlocked, "k6 run load/search.js"),

		task("T-2011", "WP-BOOK", "Hold-and-confirm two-phase reservation", StatusBlocked,
			"pnpm test:booking --grep hold", "Holds expire after 10 minutes; confirm is idempotent per hold id.", VerifyNone, ""),
		task("T-2012", "WP-BOOK", "Idempotency keys on the confirm endpoint", StatusBlocked,
			"file exists: docs/idempotency.md", "Keys are scoped per authenticated principal, so this waits on the CLI auth flow.", VerifyFail, "3h ago"),

		wp("WP-PAY", "Payments", StateActive),
		task("T-3007", "WP-PAY", "Stripe webhook signature verification", StatusReady,
			"pnpm test:pay --grep webhook", "Reject unsigned or replayed webhook deliveries.", VerifyNone, ""),
		task("T-3011", "WP-PAY", "Refund reconciliation job", StatusBlocked,
			"pnpm test:pay --grep refund", "Cannot reconcile refunds until reservations have a stable lifecycle.", VerifyNone, ""),

		wp("WP-OBS", "Observability", StatePlanned),
		task("T-4002", "WP-OBS", "Structured log schema for the Rust core", StatusReady,
			"cargo test --package fctrl-core log", "One event shape for the engine, the web app and the TUI.", VerifyNone, ""),

		wp("WP-UI", "UI Redesign", StatePlanned),
		task("T-5001", "WP-UI", "Dark-mode token audit", StatusDeferred, "manual", "", VerifyNone, ""),

		wp("WP-LEGACY", "Legacy Import", StateDone),
		task("T-9001", "WP-LEGACY", "One-off CSV import", StatusDone, "manual", "", VerifyPass, "3w ago"),

		{ID: "WP-BEER", ProjectID: "prj-beer", Type: TypeWorkPackage, Title: "Cellar tracking", Status: StatusReady, State: StateActive},
		{ID: "T-8001", ProjectID: "prj-beer", ParentID: "WP-BEER", Type: TypeTask, Title: "Bottle inventory model", Status: StatusReady, Condition: "go test ./cellar"},
		{ID: "WP-DOCS", ProjectID: "prj-docs", Type: TypeWorkPackage, Title: "API reference", Status: StatusReady, State: StateActive},
		{ID: "T-7001", ProjectID: "prj-docs", ParentID: "WP-DOCS", Type: TypeTask, Title: "Generate from OpenAPI", Status: StatusBlocked, Condition: "make docs"},
	}

	m.deps = []Dependency{
		{BlockerID: "T-1042", BlockedID: "T-1043"},
		{BlockerID: "T-1043", BlockedID: "T-1044"},
		{BlockerID: "WP-OBS", BlockedID: "T-1044"},
		{BlockerID: "T-2010", BlockedID: "T-2011"},
		{BlockerID: "T-2011", BlockedID: "T-2012"},
		{BlockerID: "T-1042", BlockedID: "T-2012"},
		{BlockerID: "WP-BOOK", BlockedID: "T-3011"},
	}
}
