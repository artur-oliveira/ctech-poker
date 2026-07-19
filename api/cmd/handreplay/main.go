package main

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type scriptPlayer struct {
	ID    string `json:"id"`
	Stack int64  `json:"stack"`
	Ready bool   `json:"ready"`
}

type scriptAction struct {
	Player string `json:"player"`
	Action string `json:"action"`
	Amount int64  `json:"amount"`
}

type script struct {
	Players    []scriptPlayer `json:"players"`
	SmallBlind int64          `json:"small_blind"`
	BigBlind   int64          `json:"big_blind"`
	Actions    []scriptAction `json:"actions"`
}

func runScript(data []byte) (map[string]int64, error) {
	var s script
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse script: %w", err)
	}

	players := make([]*hand.Player, len(s.Players))
	for i, sp := range s.Players {
		players[i] = &hand.Player{ID: sp.ID, Stack: sp.Stack, Ready: sp.Ready}
	}
	table := hand.NewTable(players, s.SmallBlind, s.BigBlind)
	if err := table.StartHand(); err != nil {
		return nil, fmt.Errorf("start hand: %w", err)
	}

	for _, a := range s.Actions {
		if err := table.Act(a.Player, betting.Action(a.Action), a.Amount); err != nil {
			return nil, fmt.Errorf("action %+v: %w", a, err)
		}
	}

	if table.Stage() != hand.Complete {
		return nil, fmt.Errorf("hand did not complete — stage is %v after all scripted actions", table.Stage())
	}
	return table.Payouts(), nil
}

func main() {
	path := "script.example.json"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read script:", err)
		os.Exit(1)
	}
	payouts, err := runScript(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(payouts, "", "  ")
	fmt.Println(string(out))
}
