package token

type Message struct {
	Role    string
	Content string
}

func Estimate(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + 3) / 4
}

func EstimateMessages(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += Estimate(m.Content) + 4
	}
	return total
}
