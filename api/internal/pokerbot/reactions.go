package pokerbot

// ReactionMoment contains only events visible to everyone at the table.
// Bots must never react to their private cards or an estimated hand strength.
type ReactionMoment int

const (
	ReactionBotWon ReactionMoment = iota
	ReactionHumanWon
	ReactionAllIn
	ReactionUncontested
)

type ReactionChoice struct {
	ID       string
	Targeted bool
}

// ChooseReaction returns a sparse, varied response to a public event. The
// actor applies the table-wide cooldown and persists the chosen reaction.
func ChooseReaction(moment ReactionMoment, random Random) (ReactionChoice, bool) {
	chance := .10
	if moment == ReactionAllIn {
		chance = .16
	}
	if random.Float64() >= chance {
		return ReactionChoice{}, false
	}
	var choices []ReactionChoice
	switch moment {
	case ReactionBotWon:
		choices = []ReactionChoice{
			{ID: "clap"}, {ID: "laugh"}, {ID: "fire"}, {ID: "shark"}, {ID: "pokerface"},
			{ID: "chip", Targeted: true}, {ID: "tear", Targeted: true},
			{ID: "tomato", Targeted: true}, {ID: "poop", Targeted: true},
			{ID: "rofl", Targeted: true}, {ID: "duck", Targeted: true},
			{ID: "spotlight", Targeted: true}, {ID: "crown", Targeted: true},
			{ID: "flowers", Targeted: true}, {ID: "bandage", Targeted: true},
		}
	case ReactionHumanWon:
		choices = []ReactionChoice{
			{ID: "clap"}, {ID: "wow"}, {ID: "angry"}, {ID: "cry"}, {ID: "respect"},
			{ID: "clover", Targeted: true}, {ID: "horseshoe", Targeted: true},
			{ID: "knife", Targeted: true}, {ID: "boomerang", Targeted: true},
		}
	case ReactionAllIn:
		choices = []ReactionChoice{
			{ID: "wow"}, {ID: "nervous"}, {ID: "heartbeat"}, {ID: "pokerface"},
			{ID: "clover", Targeted: true}, {ID: "cucumber", Targeted: true},
		}
	case ReactionUncontested:
		choices = []ReactionChoice{
			{ID: "cold"}, {ID: "sleepy"}, {ID: "pokerface"},
			{ID: "coffee", Targeted: true}, {ID: "turtle", Targeted: true},
			{ID: "cucumber", Targeted: true},
		}
	}
	if len(choices) == 0 {
		return ReactionChoice{}, false
	}
	index := int(random.Float64() * float64(len(choices)))
	if index >= len(choices) {
		index = len(choices) - 1
	}
	return choices[index], true
}
