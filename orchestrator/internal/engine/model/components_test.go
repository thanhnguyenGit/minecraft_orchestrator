package model

import "testing"

func TestHostMetadataComponentsAreTheOnlyComponents(t *testing.T) {
	const wantComponentCount = 9
	if ComponentCount != wantComponentCount {
		t.Fatalf("ComponentCount = %d, want %d host metadata components", ComponentCount, wantComponentCount)
	}

	want := Components(CBot, CSession, CPosition, CRotation, CVelocity, CHealth, CGameMode, CInventory, CEffects)
	if !MirroredBotMask.Equals(want) {
		t.Fatalf("MirroredBotMask = %s, want %s", MirroredBotMask, want)
	}
}
