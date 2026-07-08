package bus

import "fmt"

const eventStream = "mc:events"

func CommandStream(botID string) string {
	return fmt.Sprintf("mc:bot:%s:commands", botID)
}

func EventStream() string {
	return eventStream
}
