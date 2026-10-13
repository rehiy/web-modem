package service

import (
	"strings"
)

// ModemURC is an unsolicited result code emitted by a modem.
type ModemURC struct {
	ModemName string         `json:"modem_name"`
	Event     string         `json:"event"`
	Params    map[int]string `json:"params"`
}

func (m *ModemService) WatchURC(modemName string) (<-chan ModemURC, func()) {
	m.urcMu.Lock()
	defer m.urcMu.Unlock()

	m.nextURCID++
	id := m.nextURCID
	if m.urcWaiters == nil {
		m.urcWaiters = map[string]map[int]chan ModemURC{}
	}
	if m.urcWaiters[modemName] == nil {
		m.urcWaiters[modemName] = map[int]chan ModemURC{}
	}

	ch := make(chan ModemURC, 8)
	m.urcWaiters[modemName][id] = ch

	cancel := func() {
		m.urcMu.Lock()
		defer m.urcMu.Unlock()
		if waiters, ok := m.urcWaiters[modemName]; ok {
			delete(waiters, id)
			if len(waiters) == 0 {
				delete(m.urcWaiters, modemName)
			}
		}
	}

	return ch, cancel
}

func (m *ModemService) dispatchURC(modemName string, event string, params map[int]string) {
	m.urcMu.Lock()
	defer m.urcMu.Unlock()

	for _, ch := range m.urcWaiters[modemName] {
		urc := ModemURC{
			ModemName: modemName,
			Event:     event,
			Params:    cloneURCParams(params),
		}
		select {
		case ch <- urc:
		default:
		}
	}
}

func cloneURCParams(params map[int]string) map[int]string {
	if params == nil {
		return map[int]string{}
	}
	cloned := make(map[int]string, len(params))
	for key, value := range params {
		cloned[key] = value
	}
	return cloned
}

func URCMatches(event string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.EqualFold(event, prefix) || strings.HasPrefix(strings.ToUpper(event), strings.ToUpper(prefix)) {
			return true
		}
	}
	return false
}
