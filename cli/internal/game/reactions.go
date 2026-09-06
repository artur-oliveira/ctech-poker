package game

import "strings"

// Reaction is one entry in the table reaction catalog. Mirrors
// ui/src/lib/reactions.ts (TABLE_REACTIONS) — kept in sync by hand; the wire
// protocol carries only the id, the server owns the authoritative list and
// premium-ownership checks.
type Reaction struct {
	ID       string
	Label    string
	Targeted bool // needs a target player (thrown at someone) vs. a broadcast emote
}

// TableReactions is the catalog in display order: broadcast emotes first,
// then the targeted ones.
var TableReactions = []Reaction{
	{"clap", "Aplausos", false},
	{"laugh", "Risada", false},
	{"wow", "Uau", false},
	{"angry", "Raiva", false},
	{"cry", "Choro", false},
	{"nervous", "Nervoso", false},
	{"cold", "Frio na mesa", false},
	{"fire", "Pegando fogo", false},
	{"respect", "Respeito", false},
	{"sleepy", "Sono", false},
	{"heartbeat", "Coração all-in", false},
	{"shark", "Modo tubarão", false},
	{"pokerface", "Pokerface", false},

	{"chip", "Jogar ficha", true},
	{"coffee", "Mandar café", true},
	{"clover", "Dar sorte", true},
	{"horseshoe", "Jogar ferradura", true},
	{"tear", "Jogar lágrima", true},
	{"tomato", "Jogar tomate", true},
	{"poop", "Jogar cocô", true},
	{"rofl", "Rir da cara", true},
	{"duck", "Jogar pato", true},
	{"turtle", "Chamar de lento", true},
	{"knife", "Jogar faca", true},
	{"flowers", "Mandar flores", true},
	{"spotlight", "Boa leitura", true},
	{"crown", "Passar a coroa", true},
	{"bandage", "Curar bad beat", true},
	{"cucumber", "Botar pepino", true},
	{"boomerang", "Jogar bumerangue", true},
}

// LookupReaction returns the catalog entry for id (case-insensitive).
func LookupReaction(id string) (Reaction, bool) {
	for _, r := range TableReactions {
		if strings.EqualFold(r.ID, id) {
			return r, true
		}
	}
	return Reaction{}, false
}
