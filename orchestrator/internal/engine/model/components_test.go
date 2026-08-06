package model

import "testing"

func TestHostMetadataComponentsAreTheOnlyComponents(t *testing.T) {
	const wantComponentCount = 10
	if ComponentCount != wantComponentCount {
		t.Fatalf("ComponentCount = %d, want %d host metadata components", ComponentCount, wantComponentCount)
	}

	want := Components(CBot, CSession, CPosition, CRotation, CVelocity, CHealth, CHunger, CGameMode, CInventory, CEffects)
	if !MirroredBotMask.Equals(want) {
		t.Fatalf("MirroredBotMask = %s, want %s", MirroredBotMask, want)
	}
}
