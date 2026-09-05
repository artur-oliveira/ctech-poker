package social

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type memoryEdgeStore struct {
	mu     sync.Mutex
	edges  map[string]Edge
	counts map[string]map[Relationship]int
}

func newMemoryEdgeStore() *memoryEdgeStore {
	return &memoryEdgeStore{edges: make(map[string]Edge), counts: make(map[string]map[Relationship]int)}
}

func edgeMapKey(owner, other string) string { return owner + "\x00" + other }

func (m *memoryEdgeStore) Get(_ context.Context, owner, other string) (*Edge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	edge, ok := m.edges[edgeMapKey(owner, other)]
	if !ok {
		return nil, nil
	}
	return cloneEdge(&edge), nil
}

func (m *memoryEdgeStore) List(context.Context, string, Relationship, bool, int, map[string]types.AttributeValue) ([]Edge, map[string]types.AttributeValue, error) {
	return nil, nil, nil
}

func (m *memoryEdgeStore) Count(_ context.Context, owner string, relationship Relationship, _ int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if configured := m.counts[owner]; configured != nil {
		return configured[relationship], nil
	}
	count := 0
	for _, edge := range m.edges {
		if edge.OwnerPlayerID == owner && edge.Relationship == relationship {
			count++
		}
	}
	return count, nil
}

func (m *memoryEdgeStore) Apply(_ context.Context, transition Transition) (*Edge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	actorKey := edgeMapKey(transition.ActorPlayerID, transition.TargetPlayerID)
	targetKey := edgeMapKey(transition.TargetPlayerID, transition.ActorPlayerID)
	var actor, target *Edge
	if edge, ok := m.edges[actorKey]; ok {
		actor = cloneEdge(&edge)
	}
	if edge, ok := m.edges[targetKey]; ok {
		target = cloneEdge(&edge)
	}
	afterActor, afterTarget, err := PlanTransition(actor, target, transition)
	if err != nil {
		return nil, err
	}
	if afterActor == nil {
		delete(m.edges, actorKey)
	} else {
		m.edges[actorKey] = *afterActor
	}
	if afterTarget == nil {
		delete(m.edges, targetKey)
	} else {
		m.edges[targetKey] = *afterTarget
	}
	return cloneEdge(afterActor), nil
}

func TestConcurrentCrossedRequestsConvergeToMutualFriendship(t *testing.T) {
	store := newMemoryEdgeStore()
	service := NewService(store, true)
	service.now = func() time.Time { return transitionTime }
	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, pair := range [][2]string{{"a", "b"}, {"b", "a"}} {
		pair := pair
		go func() {
			<-start
			_, err := service.Request(context.Background(), pair[0], pair[1], pair[0])
			errs <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	a, _ := store.Get(context.Background(), "a", "b")
	b, _ := store.Get(context.Background(), "b", "a")
	assertPair(t, a, b, RelationshipFriend, RelationshipFriend)
}

func TestGraphFlagDoesNotDisableSafetyActions(t *testing.T) {
	store := newMemoryEdgeStore()
	service := NewService(store, false)
	if _, err := service.Request(context.Background(), "a", "b", "request"); err != ErrFeatureDisabled {
		t.Fatalf("request err=%v", err)
	}
	edge, err := service.Block(context.Background(), "a", "b", "block")
	if err != nil || edge == nil || !edge.Blocked || !edge.Muted {
		t.Fatalf("block edge=%+v err=%v", edge, err)
	}
}

func TestServiceEnforcesFriendAndPendingLimits(t *testing.T) {
	store := newMemoryEdgeStore()
	store.counts["a"] = map[Relationship]int{RelationshipOutgoing: MaxPendingOutgoing}
	service := NewService(store, true)
	if _, err := service.Request(context.Background(), "a", "b", "request"); err != ErrRequestLimitReached {
		t.Fatalf("request limit err=%v", err)
	}

	store.counts["a"] = map[Relationship]int{RelationshipFriend: MaxFriends}
	store.edges[edgeMapKey("a", "b")] = Edge{OwnerPlayerID: "a", OtherPlayerID: "b", Relationship: RelationshipIncoming, Version: 1}
	store.edges[edgeMapKey("b", "a")] = Edge{OwnerPlayerID: "b", OtherPlayerID: "a", Relationship: RelationshipOutgoing, Version: 1}
	if _, err := service.Accept(context.Background(), "a", "b", "accept"); err != ErrFriendLimitReached {
		t.Fatalf("friend limit err=%v", err)
	}
}
