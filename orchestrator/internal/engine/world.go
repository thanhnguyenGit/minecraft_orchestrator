package engine


func SyncWorld(events *[]string) {
	defer func() {
		clear(*events)
		*events = (*events)[:0]
	}()
	
	// for _, ev := range *events {
		
	// }

	
}

func RunEngineLoop() {
	var tick uint64 = 0
	events := make([]string, 1024)
	for {
		tick++
		SyncWorld(&events)
	}
}