package player

import (
	"fmt"
	"math/rand/v2"
)

var first = [...]string{
	"Amber", "Arctic", "Ash", "Aspen", "Autumn",
	"Azure", "Blaze", "Bloom", "Blue", "Bright",
	"Bronze", "Cedar", "Cherry", "Cloud", "Coral",
	"Crimson", "Crystal", "Dawn", "Echo", "Ember",
	"Emerald", "Falcon", "Fern", "Flame", "Forest",
	"Frost", "Galaxy", "Glacier", "Golden", "Harbor",
	"Haven", "Hazel", "Honey", "Indigo", "Iris",
	"Ivory", "Jade", "Juniper", "Lake", "Lavender",
	"Leaf", "Lemon", "Lilac", "Luna", "Maple",
	"Marble", "Meadow", "Mist", "Moon", "Morning",
	"Moss", "Night", "Nova", "Oak", "Ocean",
	"Olive", "Onyx", "Opal", "Pearl", "Pebble",
	"Pine", "Quartz", "Rain", "Raven", "River",
	"Robin", "Rose", "Ruby", "Sage", "Scarlet",
	"Shadow", "Silver", "Sky", "Snow", "Solar",
	"Spring", "Star", "Stone", "Storm", "Summer",
	"Sunset", "Sunny", "Thunder", "Vale", "Velvet",
	"Violet", "Wave", "West", "Whisper", "White",
	"Wild", "Willow", "Wind", "Winter", "Wood",
}

var second = [...]string{
	"Arrow", "Badger", "Bear", "Bee", "Berry",
	"Bird", "Bloom", "Brook", "Butterfly", "Canyon",
	"Cascade", "Castle", "Cloud", "Comet", "Creek",
	"Crown", "Daisy", "Deer", "Dolphin", "Dragon",
	"Dream", "Eagle", "Falcon", "Feather", "Field",
	"Fire", "Flower", "Forest", "Fox", "Frog",
	"Garden", "Glade", "Harbor", "Hawk", "Hill",
	"Horizon", "Island", "Jungle", "Lake", "Leaf",
	"Leopard", "Light", "Lion", "Lotus", "Meadow",
	"Meteor", "Moon", "Mountain", "Oak", "Ocean",
	"Otter", "Owl", "Panther", "Path", "Peak",
	"Pearl", "Phoenix", "Pine", "Planet", "Pond",
	"Rabbit", "Rain", "Raven", "Reef", "River",
	"Robin", "Rose", "Shadow", "Shore", "Sky",
	"Snow", "Song", "Spark", "Spirit", "Spring",
	"Star", "Stone", "Storm", "Stream", "Sun",
	"Sunrise", "Sunset", "Tiger", "Trail", "Tree",
	"Valley", "Wave", "Whale", "Willow", "Wind",
	"Wing", "Wolf", "Wood", "Woodland", "Wren",
}

func RandomName() string {
	return fmt.Sprintf(
		"%s %s",
		first[rand.IntN(len(first))],
		second[rand.IntN(len(second))],
	)
}
