package scheduler

import (
	"slices"
	"strings"
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

type testSystem struct {
	id     SystemID
	access AccessSpec
}

func (s testSystem) ID() SystemID             { return s.id }
func (s testSystem) Access() AccessSpec        { return s.access }
func (s testSystem) Run(_ *RunContext) error   { return nil }

func testAccess(reads, writes model.Mask, queries, structural []model.Mask) AccessSpec {
	return AccessSpec{
		Reads:      reads,
		Writes:     writes,
		Queries:    queries,
		Structural: structural,
	}
}

func mustCompile(t testing.TB, b *Builder) *ExecutionPlan {
	t.Helper()
	plan, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	return plan
}

var (
	mPos   = model.Components(model.CPosition)
	mVel   = model.Components(model.CVelocity)
	mHealth = model.Components(model.CHealth)
	mBot   = model.Components(model.CBot)
)

func TestSortPhase_Empty(t *testing.T) {
	b := NewBuilder()
	result, err := b.sortdPhase()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("len = %d, want 0", len(result))
	}
}

func TestSortPhase_Single(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("A")

	result, err := b.sortdPhase()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].id != "A" {
		t.Fatalf("result = %v, want [A]", phaseIDs(result))
	}
}

func TestSortPhase_Chain(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("A")
	b.AddPhase("B", "A")
	b.AddPhase("C", "B")

	result, err := b.sortdPhase()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids := phaseIDs(result); !slices.Equal(ids, []string{"A", "B", "C"}) {
		t.Fatalf("result = %v, want [A B C]", ids)
	}
}

func TestSortPhase_Diamond(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("A")
	b.AddPhase("B")
	b.AddPhase("C", "A", "B")

	result, err := b.sortdPhase()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ids := phaseIDs(result)
	idxA := slices.Index(ids, "A")
	idxB := slices.Index(ids, "B")
	idxC := slices.Index(ids, "C")
	if idxC < idxA || idxC < idxB {
		t.Fatalf("result = %v, C must be after both A and B", ids)
	}
}

func TestSortPhase_Cycle(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("A", "C")
	b.AddPhase("B", "A")
	b.AddPhase("C", "B")

	_, err := b.sortdPhase()
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSortPhase_UnknownDependency(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("A", "X")

	_, err := b.sortdPhase()
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
	if !strings.Contains(err.Error(), "unknown phase") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompilePhase_SingleSystem(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	b.AddSystem("main", testSystem{id: "S1", access: testAccess(0, 0, nil, nil)})

	waves, err := b.compilePhase(b.phases["main"])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(waves) != 1 || len(waves[0]) != 1 {
		t.Fatalf("waves = %v, want [[S1]]", waveIDs(waves))
	}
}

func TestCompilePhase_NoConflictSameWave(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	acc := testAccess(0, 0, nil, nil)
	b.AddSystem("main", testSystem{id: "S1", access: acc})
	b.AddSystem("main", testSystem{id: "S2", access: acc})

	waves, err := b.compilePhase(b.phases["main"])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(waves) != 1 || len(waves[0]) != 2 {
		t.Fatalf("waves = %v, want [[S1 S2]]", waveIDs(waves))
	}
}

func TestCompilePhase_AfterDependency(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	acc := testAccess(0, 0, nil, nil)
	b.AddSystem("main", testSystem{id: "S1", access: acc})
	b.AddSystem("main", testSystem{id: "S2", access: acc}, After("S1"))

	waves, err := b.compilePhase(b.phases["main"])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids := waveIDs(waves); !waveIDsEqual(ids, [][]string{{"S1"}, {"S2"}}) {
		t.Fatalf("waves = %v, want [[S1] [S2]]", ids)
	}
}

func TestCompilePhase_BeforeDependency(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	acc := testAccess(0, 0, nil, nil)
	b.AddSystem("main", testSystem{id: "S1", access: acc}, Before("S2"))
	b.AddSystem("main", testSystem{id: "S2", access: acc})

	waves, err := b.compilePhase(b.phases["main"])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids := waveIDs(waves); !waveIDsEqual(ids, [][]string{{"S1"}, {"S2"}}) {
		t.Fatalf("waves = %v, want [[S1] [S2]]", ids)
	}
}

func TestCompilePhase_Cycle(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	acc := testAccess(0, 0, nil, nil)
	b.AddSystem("main", testSystem{id: "S1", access: acc}, After("S2"))
	b.AddSystem("main", testSystem{id: "S2", access: acc}, After("S1"))

	_, err := b.compilePhase(b.phases["main"])
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompilePhase_ExternalDependency(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	acc := testAccess(0, 0, nil, nil)
	b.AddSystem("main", testSystem{id: "S1", access: acc}, After("S2"))

	_, err := b.compilePhase(b.phases["main"])
	if err == nil {
		t.Fatal("expected error for external dependency")
	}
	if !strings.Contains(err.Error(), "outside this phase") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompilePhase_ConflictSeparatesWaves(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	b.AddSystem("main", testSystem{id: "S1", access: testAccess(0, mPos, nil, nil)})
	b.AddSystem("main", testSystem{id: "S2", access: testAccess(mPos, 0, nil, nil)})

	waves, err := b.compilePhase(b.phases["main"])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids := waveIDs(waves); len(ids) != 2 {
		t.Fatalf("waves = %v, want 2 separate waves", ids)
	}
}

func TestSystemsConflict_None(t *testing.T) {
	a := testAccess(mPos, 0, nil, nil)
	b := testAccess(mVel, 0, nil, nil)
	if systemsConflict(a, b) {
		t.Fatal("disjoint accesses should not conflict")
	}
}

func TestSystemsConflict_WriteRead(t *testing.T) {
	a := testAccess(0, mPos, nil, nil)
	b := testAccess(mPos, 0, nil, nil)
	if !systemsConflict(a, b) {
		t.Fatal("A writes X, B reads X should conflict")
	}
}

func TestSystemsConflict_ReadWrite(t *testing.T) {
	a := testAccess(mPos, 0, nil, nil)
	b := testAccess(0, mPos, nil, nil)
	if !systemsConflict(a, b) {
		t.Fatal("A reads X, B writes X should conflict")
	}
}

func TestSystemsConflict_WriteWrite(t *testing.T) {
	a := testAccess(0, mPos, nil, nil)
	b := testAccess(0, mPos, nil, nil)
	if !systemsConflict(a, b) {
		t.Fatal("both write X should conflict")
	}
}

func TestSystemsConflict_BothRead(t *testing.T) {
	a := testAccess(mPos, mVel, nil, nil)
	b := testAccess(mPos, mHealth, nil, nil)
	if systemsConflict(a, b) {
		t.Fatal("both only reading overlapping component should not conflict")
	}
}

func TestSystemsConflict_StructuralVsQuery(t *testing.T) {
	a := testAccess(0, 0, nil, []model.Mask{mPos | mVel})
	b := testAccess(0, 0, []model.Mask{mPos}, nil)
	if !systemsConflict(a, b) {
		t.Fatal("A.Structural contains B.Queries should conflict")
	}
}

func TestSystemsConflict_StructuralNoOverlap(t *testing.T) {
	a := testAccess(0, 0, nil, []model.Mask{mPos})
	b := testAccess(0, 0, []model.Mask{mVel}, nil)
	if systemsConflict(a, b) {
		t.Fatal("A.Structural does not contain B.Queries should not conflict")
	}
}

func TestSystemsConflict_BothStructural(t *testing.T) {
	a := testAccess(0, 0, nil, []model.Mask{mPos})
	b := testAccess(0, 0, nil, []model.Mask{mVel})
	if !systemsConflict(a, b) {
		t.Fatal("both have structural effects should conflict")
	}
}

func TestWaveTouchesDirty_Empty(t *testing.T) {
	sys := &systemDef{system: testSystem{id: "S", access: testAccess(0, 0, []model.Mask{mPos}, nil)}}
	touched, masks := waveTouchesDirty([]*systemDef{sys}, map[model.Mask]struct{}{})
	if touched {
		t.Fatal("empty dirty map should not touch")
	}
	if len(masks) != 0 {
		t.Fatalf("masks = %v, want []", masks)
	}
}

func TestWaveTouchesDirty_ExactMatch(t *testing.T) {
	sys := &systemDef{system: testSystem{id: "S", access: testAccess(0, 0, []model.Mask{mPos}, nil)}}
	dirty := map[model.Mask]struct{}{mPos: {}}

	touched, masks := waveTouchesDirty([]*systemDef{sys}, dirty)
	if !touched {
		t.Fatal("dirty contains exact query mask")
	}
	if len(masks) != 1 || masks[0] != mPos {
		t.Fatalf("masks = %v, want [mPos]", masks)
	}
}

func TestWaveTouchesDirty_Superset(t *testing.T) {
	sys := &systemDef{system: testSystem{id: "S", access: testAccess(0, 0, []model.Mask{mPos}, nil)}}
	dirty := map[model.Mask]struct{}{mPos | mVel: {}}

	touched, _ := waveTouchesDirty([]*systemDef{sys}, dirty)
	if !touched {
		t.Fatal("dirty superset contains query mask")
	}
}

func TestWaveTouchesDirty_NoMatch(t *testing.T) {
	sys := &systemDef{system: testSystem{id: "S", access: testAccess(0, 0, []model.Mask{mPos}, nil)}}
	dirty := map[model.Mask]struct{}{mVel: {}}

	touched, _ := waveTouchesDirty([]*systemDef{sys}, dirty)
	if touched {
		t.Fatal("no overlap should not touch")
	}
}

func TestAppendUniqueMask_New(t *testing.T) {
	result := appendUniqueMask(nil, mPos)
	if len(result) != 1 || result[0] != mPos {
		t.Fatalf("result = %v, want [mPos]", result)
	}
}

func TestAppendUniqueMask_NewToNonEmpty(t *testing.T) {
	result := appendUniqueMask([]model.Mask{mPos}, mVel)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
}

func TestAppendUniqueMask_Duplicate(t *testing.T) {
	result := appendUniqueMask([]model.Mask{mPos}, mPos)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1 (duplicate not appended)", len(result))
	}
}

func TestCompile_Empty(t *testing.T) {
	b := NewBuilder()
	plan := mustCompile(t, b)
	if len(plan.Nodes) != 0 {
		t.Fatalf("Nodes = %d, want 0", len(plan.Nodes))
	}
}

func TestCompile_SinglePhaseOneSystem(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	b.AddSystem("main", testSystem{id: "S1", access: testAccess(0, 0, nil, nil)})

	plan := mustCompile(t, b)
	if len(plan.Nodes) != 1 {
		t.Fatalf("Nodes = %d, want 1", len(plan.Nodes))
	}
	node := plan.Nodes[0]
	if node.Kind != NodeWave {
		t.Fatal("expected NodeWave")
	}
	if node.Phase != "main" {
		t.Fatalf("Phase = %q, want \"main\"", node.Phase)
	}
	if len(node.Systems) != 1 || node.Systems[0].System.ID() != "S1" {
		t.Fatalf("Systems = %v", systemIDs(node.Systems))
	}
}

func TestCompile_TwoCompatibleSystems(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	acc := testAccess(0, 0, nil, nil)
	b.AddSystem("main", testSystem{id: "S1", access: acc})
	b.AddSystem("main", testSystem{id: "S2", access: acc})

	plan := mustCompile(t, b)
	if len(plan.Nodes) != 1 {
		t.Fatalf("Nodes = %d, want 1", len(plan.Nodes))
	}
	ids := systemIDs(plan.Nodes[0].Systems)
	if !slices.Equal(ids, []string{"S1", "S2"}) {
		t.Fatalf("Systems = %v, want [S1 S2]", ids)
	}
}

func TestCompile_TwoConflictingSystems(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	b.AddSystem("main", testSystem{id: "S1", access: testAccess(0, mPos, nil, nil)})
	b.AddSystem("main", testSystem{id: "S2", access: testAccess(mPos, 0, nil, nil)})

	plan := mustCompile(t, b)
	if len(plan.Nodes) != 2 {
		t.Fatalf("Nodes = %d, want 2", len(plan.Nodes))
	}
	for _, n := range plan.Nodes {
		if n.Kind != NodeWave {
			t.Fatal("all nodes should be NodeWave")
		}
	}
}

func TestCompile_SyncOnDirtyQuery(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	b.AddSystem("main", testSystem{
		id:     "S1",
		access: testAccess(0, 0, nil, []model.Mask{mBot | mPos}),
	})
	b.AddSystem("main", testSystem{
		id:     "S2",
		access: testAccess(0, 0, []model.Mask{mBot}, nil),
	})

	plan := mustCompile(t, b)
	if len(plan.Nodes) != 3 {
		t.Fatalf("Nodes = %d, want 3 (wave, sync, wave)", len(plan.Nodes))
	}
	if plan.Nodes[0].Kind != NodeWave {
		t.Fatal("node 0 should be NodeWave")
	}
	if plan.Nodes[1].Kind != NodeSync {
		t.Fatal("node 1 should be NodeSync")
	}
	if plan.Nodes[2].Kind != NodeWave {
		t.Fatal("node 2 should be NodeWave")
	}
}

func TestCompile_EndOfFrameSync(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	b.AddSystem("main", testSystem{
		id:     "S1",
		access: testAccess(0, 0, nil, []model.Mask{mPos}),
	})

	plan := mustCompile(t, b)
	if len(plan.Nodes) != 2 {
		t.Fatalf("Nodes = %d, want 2 (wave, sync)", len(plan.Nodes))
	}
	last := plan.Nodes[len(plan.Nodes)-1]
	if last.Kind != NodeSync {
		t.Fatal("last node should be NodeSync (end-of-frame)")
	}
	if !strings.Contains(last.Reason, "end-of-frame commit") {
		t.Fatalf("Reason = %q, should contain 'end-of-frame commit'", last.Reason)
	}
}

func TestCompile_GlobalOrderIncrements(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("main")
	acc := testAccess(0, 0, nil, nil)
	b.AddSystem("main", testSystem{id: "S1", access: acc})
	b.AddSystem("main", testSystem{id: "S2", access: acc})

	plan := mustCompile(t, b)
	orders := make([]int, 0)
	for _, n := range plan.Nodes {
		for _, cs := range n.Systems {
			orders = append(orders, cs.Order)
		}
	}
	for i := 1; i < len(orders); i++ {
		if orders[i] <= orders[i-1] {
			t.Fatalf("Orders not strictly increasing: %v", orders)
		}
	}
}

func TestCompile_TwoPhases_Ordered(t *testing.T) {
	b := NewBuilder()
	b.AddPhase("first")
	b.AddPhase("second", "first")
	acc := testAccess(0, 0, nil, nil)
	b.AddSystem("first", testSystem{id: "S1", access: acc})
	b.AddSystem("second", testSystem{id: "S2", access: acc})

	plan := mustCompile(t, b)
	if len(plan.Nodes) != 2 {
		t.Fatalf("Nodes = %d, want 2", len(plan.Nodes))
	}
	if plan.Nodes[0].Phase != "first" {
		t.Fatalf("first node Phase = %q, want \"first\"", plan.Nodes[0].Phase)
	}
	if plan.Nodes[1].Phase != "second" {
		t.Fatalf("second node Phase = %q, want \"second\"", plan.Nodes[1].Phase)
	}
}

func phaseIDs(phases []*phaseDef) []string {
	ids := make([]string, len(phases))
	for i, p := range phases {
		ids[i] = string(p.id)
	}
	return ids
}

func waveIDs(waves [][]*systemDef) [][]string {
	ids := make([][]string, len(waves))
	for i, w := range waves {
		ids[i] = make([]string, len(w))
		for j, s := range w {
			ids[i][j] = string(s.system.ID())
		}
	}
	return ids
}

func systemIDs(sys []CompiledSystem) []string {
	ids := make([]string, len(sys))
	for i, s := range sys {
		ids[i] = string(s.System.ID())
	}
	return ids
}

func waveIDsEqual(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !slices.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}
