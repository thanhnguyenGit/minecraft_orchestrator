package core

import (
	"strings"
	"testing"

	"minecraft_orchestrator/internal/engine/model"
)

func newShadowState() *shadowState {
	return &shadowState{
		entities: make(map[Entity]*shadowEntity),
		bots:     make(map[model.ProfileID]Entity),
	}
}

func mustCreateEntityInWorld(t testing.TB, w *World, b Bundle) Entity {
	t.Helper()
	e, err := w.createNow(b)
	if err != nil {
		t.Fatalf("createNow error = %v", err)
	}
	return e
}

func TestCommandBuffer_New(t *testing.T) {
	buf := NewCommandBuffer(5)
	if buf.Len() != 0 {
		t.Fatalf("Len = %d, want 0", buf.Len())
	}
	if buf.systemOrder != 5 {
		t.Fatalf("systemOrder = %d, want 5", buf.systemOrder)
	}
	if buf.next != 0 {
		t.Fatalf("next = %d, want 0", buf.next)
	}
}

func TestCommandBuffer_Stage(t *testing.T) {
	buf := NewCommandBuffer(1)
	cmd := CreateCommand{Bundle: Bundle{}}

	buf.Stage(cmd)
	buf.Stage(cmd)

	if buf.Len() != 2 {
		t.Fatalf("Len = %d, want 2", buf.Len())
	}
	if buf.next != 2 {
		t.Fatalf("next = %d, want 2", buf.next)
	}

	envelopes := buf.Envelopes()
	if envelopes[0].SystemOrder != 1 {
		t.Fatalf("envelopes[0].SystemOrder = %d, want 1", envelopes[0].SystemOrder)
	}
	if envelopes[0].Sequence != 0 {
		t.Fatalf("envelopes[0].Sequence = %d, want 0", envelopes[0].Sequence)
	}
	if envelopes[1].Sequence != 1 {
		t.Fatalf("envelopes[1].Sequence = %d, want 1", envelopes[1].Sequence)
	}
}

func TestCommandBuffer_Envelopes_ReturnsCopy(t *testing.T) {
	buf := NewCommandBuffer(0)
	buf.Stage(CreateCommand{Bundle: Bundle{}})

	result := buf.Envelopes()
	result[0].SystemOrder = 999

	if buf.Envelopes()[0].SystemOrder == 999 {
		t.Fatal("mutating returned slice should not affect buffer")
	}
}

func TestCreateCommand_Validate_InvalidBundle(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	cmd := CreateCommand{Bundle: Bundle{}}
	_, err := cmd.validate(w, shadow)
	if err == nil {
		t.Fatal("expected error for invalid bundle")
	}
}

func TestCreateCommand_Validate_Success(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	var b Bundle
	b.Set(model.CPosition, model.Position{X: 1})
	cmd := CreateCommand{Bundle: b}

	vc, err := cmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("validate error = %v", err)
	}
	if _, ok := vc.(validatedCreate); !ok {
		t.Fatalf("expected validatedCreate, got %T", vc)
	}
}

func TestCreateCommand_Validate_Success_WithBot(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	cmd := CreateCommand{Bundle: makeBundle(model.Components(model.CPosition, model.CBot), 42)}

	vc, err := cmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("validate error = %v", err)
	}

	if _, ok := vc.(validatedCreate); !ok {
		t.Fatalf("expected validatedCreate, got %T", vc)
	}
	if _, reserved := shadow.bots[profileIDForTest(42)]; !reserved {
		t.Fatal("bot profile ID should be reserved in shadow.bots")
	}
}

func TestCreateCommand_Validate_DuplicateBotInWorld(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	mask := model.Components(model.CPosition, model.CBot)
	mustCreateEntityInWorld(t, w, makeBundle(mask, 42))

	cmd := CreateCommand{Bundle: makeBundle(mask, 42)}
	_, err := cmd.validate(w, shadow)
	if err == nil {
		t.Fatal("expected error for duplicate bot in world")
	}
	if !strings.Contains(err.Error(), "already mapped") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateCommand_Validate_DuplicateBotInShadow(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()
	shadow.bots[profileIDForTest(42)] = Entity{}

	cmd := CreateCommand{Bundle: makeBundle(model.Components(model.CPosition, model.CBot), 42)}
	_, err := cmd.validate(w, shadow)
	if err == nil {
		t.Fatal("expected error for duplicate bot in shadow")
	}
	if !strings.Contains(err.Error(), "already exists in batch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateCommand_DeclaredAffected(t *testing.T) {
	var b Bundle
	b.Set(model.CPosition, model.Position{})
	cmd := CreateCommand{Bundle: b}

	masks := cmd.DeclaredAffected()
	if len(masks) != 1 {
		t.Fatalf("DeclaredAffected len = %d, want 1", len(masks))
	}
	if masks[0] != b.Mask {
		t.Fatalf("DeclaredAffected[0] = %v, want %v", masks[0], b.Mask)
	}
}

func TestCreateCommand_String(t *testing.T) {
	var b Bundle
	b.Set(model.CPosition, model.Position{})
	cmd := CreateCommand{Bundle: b}

	str := cmd.String()
	if !strings.HasPrefix(str, "create ") {
		t.Fatalf("String = %q, should start with 'create '", str)
	}
}

func TestDestroyCommand_Validate_InvalidEntity(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	cmd := DestroyCommand{Entity: Entity{Index: 999}}
	_, err := cmd.validate(w, shadow)
	if err == nil {
		t.Fatal("expected error for invalid entity")
	}
	if !strings.Contains(err.Error(), "validate destroy") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDestroyCommand_Validate_DeadEntity(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	e := mustCreateEntityInWorld(t, w, makeBundle(model.Components(model.CPosition, model.CBot), 1))

	cmd1 := DestroyCommand{Entity: e}
	_, err := cmd1.validate(w, shadow)
	if err != nil {
		t.Fatalf("first destroy should succeed: %v", err)
	}

	cmd2 := DestroyCommand{Entity: e}
	_, err = cmd2.validate(w, shadow)
	if err == nil {
		t.Fatal("expected error for already destroyed entity")
	}
	if !strings.Contains(err.Error(), "already destroyed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDestroyCommand_Validate_ExpectedMaskMismatch(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	e := mustCreateEntityInWorld(t, w, makeBundle(model.Components(model.CPosition, model.CBot), 1))

	cmd := DestroyCommand{
		Entity:       e,
		ExpectedMask: model.Components(model.CHealth),
	}
	_, err := cmd.validate(w, shadow)
	if err == nil {
		t.Fatal("expected error for mask mismatch")
	}
	if !strings.Contains(err.Error(), "expected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDestroyCommand_Validate_Success(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	e := mustCreateEntityInWorld(t, w, makeBundle(model.Components(model.CPosition, model.CBot), 1))

	cmd := DestroyCommand{Entity: e}
	vc, err := cmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("validate error = %v", err)
	}

	vd, ok := vc.(validatedDestroy)
	if !ok {
		t.Fatalf("expected validatedDestroy, got %T", vc)
	}
	if vd.entity != e {
		t.Fatalf("validatedDestroy.entity = %v, want %v", vd.entity, e)
	}
}

func TestDestroyCommand_Validate_NoExpectedMask(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	e := mustCreateEntityInWorld(t, w, makeBundle(model.Components(model.CPosition, model.CBot), 1))

	cmd := DestroyCommand{Entity: e, ExpectedMask: 0}
	_, err := cmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("ExpectedMask=0 should accept any mask: %v", err)
	}
}

func TestDestroyCommand_Validate_RemovesBotFromShadow(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	e := mustCreateEntityInWorld(t, w, makeBundle(model.Components(model.CPosition, model.CBot), 1))
	shadow.bots[profileIDForTest(1)] = Entity{}

	cmd := DestroyCommand{Entity: e}
	_, err := cmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("validate error = %v", err)
	}

	if _, exists := shadow.bots[profileIDForTest(1)]; exists {
		t.Fatal("bot should be removed from shadow.bots after destroy")
	}
}

func TestDestroyCommand_Validate_NoBot(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	e := mustCreateEntityInWorld(t, w, makeBundle(model.Components(model.CPosition), 0))

	cmd := DestroyCommand{Entity: e}
	vc, err := cmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("validate error = %v, non-bot entities should be destroyable", err)
	}
	if _, ok := vc.(validatedDestroy); !ok {
		t.Fatalf("expected validatedDestroy, got %T", vc)
	}
}

func TestDestroyCommand_String(t *testing.T) {
	cmd := DestroyCommand{Entity: Entity{Index: 1, Generation: 2}}
	str := cmd.String()
	if !strings.HasPrefix(str, "destroy ") {
		t.Fatalf("String = %q, should start with 'destroy '", str)
	}
}

func TestDisconnectCommand_Validate_InvalidEntity(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	cmd := DisconnectedCommand{Entity: Entity{Index: 999}}
	_, err := cmd.validate(w, shadow)
	if err == nil {
		t.Fatal("expected error for invalid entity")
	}
}

func TestDisconnectCommand_Validate_WrongMask(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	e := mustCreateEntityInWorld(t, w, makeBundle(model.Components(model.CPosition, model.CBot), 1))

	cmd := DisconnectedCommand{Entity: e}
	_, err := cmd.validate(w, shadow)
	if err == nil {
		t.Fatal("expected error for entity not in ConnectedBotMask")
	}
	if !strings.Contains(err.Error(), "expected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDisconnectCommand_Validate_ClientMismatch(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	e := mustCreateEntityInWorld(t, w, makeBundle(model.ConnectedBotMask, 1))

	cmd := DisconnectedCommand{
		Entity:   e,
		ClientID: "wrong_client",
	}
	_, err := cmd.validate(w, shadow)
	if err == nil {
		t.Fatal("expected error for client mismatch")
	}
	if !strings.Contains(err.Error(), "client mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDisconnectCommand_Validate_EmptyClientID(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	e := mustCreateEntityInWorld(t, w, makeBundle(model.ConnectedBotMask, 1))

	cmd := DisconnectedCommand{
		Entity:   e,
		ClientID: "",
	}
	vc, err := cmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("empty ClientID should skip check, got error: %v", err)
	}

	vm, ok := vc.(validatedMigrate)
	if !ok {
		t.Fatalf("expected validatedMigrate, got %T", vc)
	}
	if !vm.destination.Mask.Has(model.CDisconnected) {
		t.Fatal("destination should have CDisconnected")
	}
	if vm.destination.Mask.Has(model.CConnection) {
		t.Fatal("destination should not have CConnection")
	}
}

func TestDisconnectCommand_Validate_Success(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	b := makeBundle(model.ConnectedBotMask, 1)
	b.Set(model.CConnection, model.Connection{ClientId: "client_abc"})
	e := mustCreateEntityInWorld(t, w, b)

	cmd := DisconnectedCommand{
		Entity:    e,
		ClientID:  "client_abc",
		SinceTick: 12345,
	}

	vc, err := cmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("validate error = %v", err)
	}

	vm, ok := vc.(validatedMigrate)
	if !ok {
		t.Fatalf("expected validatedMigrate, got %T", vc)
	}
	if vm.entity != e {
		t.Fatalf("entity = %v, want %v", vm.entity, e)
	}
	if !vm.destination.Mask.Has(model.CDisconnected) {
		t.Fatal("destination mask should have CDisconnected")
	}
	if vm.destination.Mask.Has(model.CConnection) {
		t.Fatal("destination mask should not have CConnection")
	}

	dc, ok := vm.destination.Get(model.CDisconnected).(model.Disconnected)
	if !ok {
		t.Fatal("destination should have Disconnected data")
	}
	if dc.SinceTick != 12345 {
		t.Fatalf("SinceTick = %d, want 12345", dc.SinceTick)
	}
}

func TestDisconnectCommand_DeclaredAffected(t *testing.T) {
	cmd := DisconnectedCommand{}
	masks := cmd.DeclaredAffected()
	if len(masks) != 2 {
		t.Fatalf("DeclaredAffected len = %d, want 2", len(masks))
	}
	if masks[0] != model.ConnectedBotMask {
		t.Fatalf("DeclaredAffected[0] = %v, want ConnecedBotMask", masks[0])
	}
	if masks[1] != model.DisconnectedBotMask {
		t.Fatalf("DeclaredAffected[1] = %v, want DisconnectedBotMask", masks[1])
	}
}

func TestDisconnectCommand_String(t *testing.T) {
	cmd := DisconnectedCommand{Entity: Entity{Index: 1, Generation: 2}}
	str := cmd.String()
	if !strings.HasPrefix(str, "disconnect ") {
		t.Fatalf("String = %q, should start with 'disconnect '", str)
	}
}

func TestReconnectCommand_Validate_EmptyClientID(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	cmd := ReconnectedCommand{
		Entity:   Entity{Index: 1},
		ClientID: "",
	}
	_, err := cmd.validate(w, shadow)
	if err == nil {
		t.Fatal("expected error for empty ClientID")
	}
	if !strings.Contains(err.Error(), "client id is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReconnectCommand_Validate_InvalidEntity(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	cmd := ReconnectedCommand{
		Entity:   Entity{Index: 999},
		ClientID: "client_x",
	}
	_, err := cmd.validate(w, shadow)
	if err == nil {
		t.Fatal("expected error for invalid entity")
	}
}

func TestReconnectCommand_Validate_WrongMask(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	e := mustCreateEntityInWorld(t, w, makeBundle(model.ConnectedBotMask, 1))

	cmd := ReconnectedCommand{
		Entity:   e,
		ClientID: "client_x",
	}
	_, err := cmd.validate(w, shadow)
	if err == nil {
		t.Fatal("expected error for entity not in DisconnectedBotMask")
	}
}

func TestReconnectCommand_Validate_Success(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	b := makeBundle(model.ConnectedBotMask, 1)
	b.Set(model.CConnection, model.Connection{ClientId: "old_client", SessionId: "sess_123"})
	e := mustCreateEntityInWorld(t, w, b)

	discCmd := DisconnectedCommand{
		Entity:    e,
		ClientID:  "old_client",
		SinceTick: 100,
	}
	if _, err := discCmd.validate(w, shadow); err != nil {
		t.Fatalf("disconnect validate error = %v", err)
	}

	reconnCmd := ReconnectedCommand{
		Entity:   e,
		ClientID: "new_client",
	}
	vc, err := reconnCmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("reconnect validate error = %v", err)
	}

	vm, ok := vc.(validatedMigrate)
	if !ok {
		t.Fatalf("expected validatedMigrate, got %T", vc)
	}
	if !vm.destination.Mask.Has(model.CConnection) {
		t.Fatal("destination mask should have CConnection")
	}
	if vm.destination.Mask.Has(model.CDisconnected) {
		t.Fatal("destination mask should not have CDisconnected")
	}

	conn, ok := vm.destination.Get(model.CConnection).(model.Connection)
	if !ok {
		t.Fatal("destination should have Connection data")
	}
	if conn.ClientId != "new_client" {
		t.Fatalf("ClientId = %q, want \"new_client\"", conn.ClientId)
	}
}

func TestReconnectCommand_DeclaredAffected(t *testing.T) {
	cmd := ReconnectedCommand{}
	masks := cmd.DeclaredAffected()
	if len(masks) != 2 {
		t.Fatalf("DeclaredAffected len = %d, want 2", len(masks))
	}
	if masks[0] != model.DisconnectedBotMask {
		t.Fatalf("DeclaredAffected[0] = %v, want DisconnectedBotMask", masks[0])
	}
	if masks[1] != model.ConnectedBotMask {
		t.Fatalf("DeclaredAffected[1] = %v, want ConnecedBotMask", masks[1])
	}
}

func TestReconnectCommand_String(t *testing.T) {
	cmd := ReconnectedCommand{Entity: Entity{Index: 1, Generation: 2}}
	str := cmd.String()
	if !strings.HasPrefix(str, "reconnect ") {
		t.Fatalf("String = %q, should start with 'reconnect '", str)
	}
}

func TestValidate_CreateThenDestroy_SameBatch(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	b := makeBundle(model.Components(model.CPosition, model.CBot), 1)
	createCmd := CreateCommand{Bundle: b}
	vc, err := createCmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("create validate error = %v", err)
	}

	if err := vc.apply(w); err != nil {
		t.Fatalf("create apply error = %v", err)
	}

	destroyCmd := DestroyCommand{Entity: Entity{Index: 1, Generation: 1}}
	vd, err := destroyCmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("destroy validate error = %v", err)
	}

	if err := vd.apply(w); err != nil {
		t.Fatalf("destroy apply error = %v", err)
	}

	_, err = w.resolve(Entity{Index: 1, Generation: 1})
	if err == nil {
		t.Fatal("entity should be dead after destroy")
	}
}

func TestValidate_DestroyThenCreate_SameBot(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	b := makeBundle(model.Components(model.CPosition, model.CBot), 7)
	e := mustCreateEntityInWorld(t, w, b)

	destroyCmd := DestroyCommand{Entity: e}
	vd, err := destroyCmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("destroy validate error = %v", err)
	}

	if err := vd.apply(w); err != nil {
		t.Fatalf("destroy apply error = %v", err)
	}

	createCmd := CreateCommand{Bundle: b}
	vc, err := createCmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("create same bot after destroy should succeed: %v", err)
	}

	if err := vc.apply(w); err != nil {
		t.Fatalf("create apply error = %v", err)
	}
}

func TestValidate_CreateThenDisconnect(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	b := makeBundle(model.ConnectedBotMask, 1)
	b.Set(model.CConnection, model.Connection{ClientId: "c1"})
	e := mustCreateEntityInWorld(t, w, b)

	discCmd := DisconnectedCommand{
		Entity:    e,
		ClientID:  "c1",
		SinceTick: 500,
	}
	vc, err := discCmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("disconnect validate error = %v", err)
	}

	vm := vc.(validatedMigrate)
	if !vm.destination.Mask.Has(model.CDisconnected) {
		t.Fatal("should have migrated to DisconnectedBotMask")
	}

	if err := vc.apply(w); err != nil {
		t.Fatalf("apply error = %v", err)
	}

	bundle, err := w.bundle(e)
	if err != nil {
		t.Fatalf("bundle error = %v", err)
	}
	if !bundle.Mask.Has(model.CDisconnected) {
		t.Fatal("entity should now have CDisconnected")
	}
	if bundle.Mask.Has(model.CConnection) {
		t.Fatal("entity should not have CConnection")
	}
}

func TestValidate_Create_DuplicateBot_SameBatch(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	mask := model.Components(model.CPosition, model.CBot)
	cmd1 := CreateCommand{Bundle: makeBundle(mask, 99)}
	cmd2 := CreateCommand{Bundle: makeBundle(mask, 99)}

	_, err := cmd1.validate(w, shadow)
	if err != nil {
		t.Fatalf("first create should succeed: %v", err)
	}

	_, err = cmd2.validate(w, shadow)
	if err == nil {
		t.Fatal("second create with same bot should fail")
	}
}

func TestValidate_DisconnectThenReconnect(t *testing.T) {
	w := NewWorld()
	shadow := newShadowState()

	b := makeBundle(model.ConnectedBotMask, 1)
	b.Set(model.CConnection, model.Connection{ClientId: "c1", SessionId: "sess_abc"})
	e := mustCreateEntityInWorld(t, w, b)

	discCmd := DisconnectedCommand{
		Entity:    e,
		ClientID:  "c1",
		SinceTick: 100,
	}
	vd, err := discCmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("disconnect validate error = %v", err)
	}
	if err := vd.apply(w); err != nil {
		t.Fatalf("disconnect apply error = %v", err)
	}

	reconnCmd := ReconnectedCommand{
		Entity:   e,
		ClientID: "new_c1",
	}
	vc, err := reconnCmd.validate(w, shadow)
	if err != nil {
		t.Fatalf("reconnect validate error = %v", err)
	}

	vm := vc.(validatedMigrate)
	if !vm.destination.Mask.Has(model.CConnection) {
		t.Fatal("should have migrated back to ConnectedBotMask")
	}

	if err := vc.apply(w); err != nil {
		t.Fatalf("reconnect apply error = %v", err)
	}

	bundle, err := w.bundle(e)
	if err != nil {
		t.Fatalf("bundle error = %v", err)
	}
	if !bundle.Mask.Has(model.CConnection) {
		t.Fatal("entity should now have CConnection")
	}
	if bundle.Mask.Has(model.CDisconnected) {
		t.Fatal("entity should not have CDisconnected")
	}
}
