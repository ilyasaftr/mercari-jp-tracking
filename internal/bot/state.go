package bot

import (
	"sync"

	"github.com/ilyasaftr/mercari-price-tracking/domain"
)

type viewState int

const (
	viewMain viewState = iota
	viewWaitTrackKeyword
	viewTrackSort
	viewTrackCondition
	viewTrackOptions
	viewTrackList
	viewTrackActions
	viewWaitAlertKeyword
	viewAlertMatchType
	viewAlertList
	viewWaitPriceRange
	viewWaitExcludeKeyword
)

type chatState struct {
	View         viewState
	Setup        *trackSetup
	SearchID     int64
	PendingAlert string
	Tracks       []domain.TrackedSearch
	Alerts       []domain.AlertKeyword
}

type trackSetup struct {
	Input  string
	Params domain.SearchParams
}

type stateManager struct {
	mu     sync.RWMutex
	states map[int64]*chatState
}

func newStateManager() *stateManager {
	return &stateManager{states: make(map[int64]*chatState)}
}

func (sm *stateManager) get(chatID int64) *chatState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.states[chatID]
}

func (sm *stateManager) set(chatID int64, state *chatState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.states[chatID] = state
}

func (sm *stateManager) clear(chatID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.states, chatID)
}
