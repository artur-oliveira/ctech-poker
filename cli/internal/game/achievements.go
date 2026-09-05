package game

import "strings"

// winCategoryPrefix marks a per-hand-category achievement key
// ("win_category_flush" etc.) whose label/description derive from the hand
// category itself rather than a fixed catalog entry.
const winCategoryPrefix = "win_category_"

// achievementLabels ports ui/src/lib/utils.ts's ACHIEVEMENT_LABELS — the
// single source of Portuguese display names for achievement keys.
var achievementLabels = map[string]string{
	"wins":                          "Vitórias",
	"hands_played":                  "Mãos Jogadas",
	"comeback":                      "De Volta ao Jogo",
	"bluff":                         "Mestre do Blefe",
	"survivor":                      "Sobrevivente",
	"looser":                        "Não Foi Dessa Vez",
	"almost_winner":                 "Por Um Detalhe",
	"tied":                          "Dividindo o Pote",
	"bad_beat":                      "Que Azar!",
	"cooler":                        "Sem Escapatória",
	"cracked_aces":                  "Maldito Ás",
	"fallen_king":                   "KKKKKKKKK",
	"giant_slayer":                  "Virou o Jogo",
	"showdown_warrior":              "Paga pra Ver",
	"all_in":                        "Tudo ou Nada",
	"sandbox_chips_earned":          "Montanha de Fichas",
	"real_money_earned":             "Banca de Verdade",
	"won_with_pocket_pair":          "Par na Mão",
	"won_full_table":                "Dono da Mesa",
	"won_heads_up":                  "Duelo Vencido",
	"won_with_nuts":                 "Mão Imbatível",
	"won_runner_runner":             "Turn e River Perfeitos",
	"three_bet_won_no_showdown":     "Pressão no 3-bet",
	"beat_pocket_aces":              "Quebrou os Ases",
	"beat_trips_or_better":          "Passou por Cima",
	"first_hand_allin_win":          "Chegou Chegando",
	"same_pocket_pair_streak":       "Par de Estimação",
	"folded_streak":                 "Paciência de Pedra",
	"all_in_blind":                  "All-in às Cegas",
	"blind_magic":                   "Magia das Cartas",
	"no_rush":                       "Sem pressa",
	"four_to_royal_missed":          "Quase Royal",
	"four_to_straight_flush_missed": "Quase Straight Flush",
	"paid_river_draw_missed":        "Pagou e Não Veio",
	"lost_river_after_leading_turn": "Perdeu no River",
	"lost_straight_flush_to_royal":  "Azar Histórico",
	"win_category_high_card":        "Carta Alta",
	"win_category_pair":             "Um Par",
	"win_category_two_pair":         "Dois Pares",
	"win_category_three_of_a_kind":  "Trinca",
	"win_category_straight":         "Sequência",
	"win_category_flush":            "Flush",
	"win_category_full_house":       "Full House",
	"win_category_four_of_a_kind":   "Quadra",
	"win_category_straight_flush":   "Straight Flush",
	"win_category_royal_flush":      "Royal Flush",
}

// achievementDescriptions ports achievements.ts's DESCRIPTIONS.
var achievementDescriptions = map[string]string{
	"wins":                          "Toda mão vencida conta um ponto.",
	"hands_played":                  "Toda mão jogada soma, ganhando ou perdendo.",
	"comeback":                      "Foi all-in, ficou por um fio e ainda assim virou a mesa.",
	"bluff":                         "Ganhou sem showdown com a mão mais fraca, blefe puro, sem carta na manga.",
	"survivor":                      "Jogou na mesma mesa por muitas mãos seguidas, sem sair.",
	"looser":                        "Perdeu no showdown. Faz parte do jogo, ninguém vence sempre.",
	"almost_winner":                 "Perdeu para alguém com a mesma mão, só que um pouco mais forte.",
	"tied":                          "Empatou no showdown e dividiu o pote com o adversário.",
	"bad_beat":                      "Perdeu com trinca ou mais forte, uma mão ótima, mas não o suficiente.",
	"cooler":                        "Perdeu com full house ou mais forte, quase impossível fugir dessa.",
	"cracked_aces":                  "Foi ao showdown com par de ases e ainda assim perdeu.",
	"fallen_king":                   "Foi ao showdown com par de reis e ainda assim perdeu.",
	"giant_slayer":                  "Ganhou all-in contra um adversário com stack maior que o seu.",
	"showdown_warrior":              "Chegou ao showdown. Não teve medo de ver as cartas do adversário.",
	"all_in":                        "Empurrou todas as fichas para o meio da mesa.",
	"sandbox_chips_earned":          "Soma de todas as fichas de sandbox que você já levou dos potes.",
	"real_money_earned":             "Soma de todo o dinheiro real que você já levou dos potes.",
	"won_with_pocket_pair":          "Venceu uma mão que começou com um par na mão.",
	"won_full_table":                "Venceu com a mesa cheia, contra o máximo de adversários.",
	"won_heads_up":                  "Venceu no mano a mano, só você e um adversário na mesa.",
	"won_with_nuts":                 "Venceu com a melhor mão possível para aquele board.",
	"won_runner_runner":             "Precisava do turn e do river, e os dois vieram.",
	"three_bet_won_no_showdown":     "Deu o terceiro aumento e levou o pote sem mostrar as cartas.",
	"beat_pocket_aces":              "Ganhou de um adversário que estava com par de ases.",
	"beat_trips_or_better":          "Ganhou de um adversário com trinca ou mais forte.",
	"first_hand_allin_win":          "Foi all-in na primeira mão da mesa e venceu.",
	"same_pocket_pair_streak":       "Venceu mãos seguidas com o mesmo par na mão.",
	"folded_streak":                 "Passou muitas mãos seguidas sem colocar uma ficha no pote.",
	"four_to_royal_missed":          "Chegou a quatro cartas do royal flush e a quinta não veio.",
	"four_to_straight_flush_missed": "Chegou a quatro cartas do straight flush e a quinta não veio.",
	"paid_river_draw_missed":        "Pagou para ver o river atrás de um projeto que não fechou.",
	"lost_river_after_leading_turn": "Estava na frente no turn e perdeu no river.",
	"lost_straight_flush_to_royal":  "Perdeu com straight flush para um royal flush.",
	"all_in_blind":                  "Foi all-in sem ver nenhuma das suas cartas.",
	"blind_magic":                   "Venceu a mão sem ver nenhuma das suas cartas.",
	"no_rush":                       "Deixou o relógio correr e usou seu tempo extra para decidir.",
}

// AchievementLabel is the display name for an achievement key (falls back to
// the raw key with underscores turned to spaces, same as the UI).
func AchievementLabel(key string) string {
	if strings.HasPrefix(key, winCategoryPrefix) {
		category := strings.TrimPrefix(key, winCategoryPrefix)
		if label, ok := handCategoryLabels[category]; ok {
			return label
		}
		return category
	}
	if label, ok := achievementLabels[key]; ok {
		return label
	}
	return strings.ReplaceAll(key, "_", " ")
}

// AchievementDescription is the Portuguese one-liner for an achievement key
// (empty when the catalog has none, same as the UI).
func AchievementDescription(key string) string {
	if strings.HasPrefix(key, winCategoryPrefix) {
		category := strings.TrimPrefix(key, winCategoryPrefix)
		label := category
		if l, ok := handCategoryLabels[category]; ok {
			label = l
		}
		return "Venceu no showdown com " + strings.ToLower(label) + "."
	}
	return achievementDescriptions[key]
}

// handCategoryLabels mirrors HAND_CATEGORY_LABELS (ui/src/lib/handCategories.ts)
// for the win_category_* keys, matching HandStrength's own category names.
var handCategoryLabels = map[string]string{
	"high_card":       "Carta Alta",
	"pair":            "Um Par",
	"two_pair":        "Dois Pares",
	"three_of_a_kind": "Trinca",
	"straight":        "Sequência",
	"flush":           "Flush",
	"full_house":      "Full House",
	"four_of_a_kind":  "Quadra",
	"straight_flush":  "Straight Flush",
	"royal_flush":     "Royal Flush",
}
