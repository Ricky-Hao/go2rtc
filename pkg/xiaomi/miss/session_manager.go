package miss

import "sync"

type clientFactory func(rawURL string) (sessionClient, error)

func defaultClientFactory(rawURL string) (sessionClient, error) {
	return NewClient(rawURL)
}

// sessionManager owns shared MISS sessions. It never dials while holding mu;
// when both manager and session locks are needed, acquire manager.mu first.
type sessionManager struct {
	mu        sync.Mutex
	sessions  map[string]*session
	newClient clientFactory
}

var defaultSessionManager = newSessionManager(defaultClientFactory)

func newSessionManager(newClient clientFactory) *sessionManager {
	return &sessionManager{
		sessions:  make(map[string]*session),
		newClient: newClient,
	}
}

func (m *sessionManager) acquire(rawURL string, channel uint8) (*session, *stream, error) {
	key, err := sessionKey(rawURL)
	if err != nil {
		return nil, nil, err
	}

	if s, st, ok := m.acquireExisting(key, channel); ok {
		return s, st, nil
	}

	client, err := m.newClient(rawURL)
	if err != nil {
		return nil, nil, err
	}

	// Dafang-like models: no session sharing, return standalone session.
	if client.IsDafangLike() {
		s := newSession(client, key, nil)
		st, err := s.openStream(channel)
		if err != nil {
			_ = client.Close()
			return nil, nil, err
		}
		return s, st, nil
	}

	s := newSession(client, key, m)

	m.mu.Lock()
	if existing, ok := m.sessions[key]; ok {
		existing.mu.Lock()
		if existing.isActiveLocked() && !existing.client.IsDafangLike() {
			st, err := existing.openStreamLocked(channel)
			existing.mu.Unlock()
			m.mu.Unlock()
			_ = client.Close()
			if err != nil {
				return nil, nil, err
			}
			existing.startWorker()
			return existing, st, nil
		}
		existing.mu.Unlock()
	}

	s.mu.Lock()
	st, err := s.openStreamLocked(channel)
	s.mu.Unlock()
	if err != nil {
		m.mu.Unlock()
		_ = client.Close()
		return nil, nil, err
	}
	m.sessions[key] = s
	m.mu.Unlock()

	s.startWorker()
	return s, st, nil
}

func (m *sessionManager) acquireExisting(key string, channel uint8) (*session, *stream, bool) {
	m.mu.Lock()
	s, ok := m.sessions[key]
	if !ok {
		m.mu.Unlock()
		return nil, nil, false
	}

	s.mu.Lock()
	if !s.isActiveLocked() || s.client.IsDafangLike() {
		s.mu.Unlock()
		m.mu.Unlock()
		return nil, nil, false
	}

	st, err := s.openStreamLocked(channel)
	s.mu.Unlock()
	m.mu.Unlock()
	if err != nil {
		return nil, nil, false
	}

	s.startWorker()
	return s, st, true
}

func (m *sessionManager) remove(s *session) {
	m.mu.Lock()
	if m.sessions[s.key] == s {
		delete(m.sessions, s.key)
	}
	m.mu.Unlock()
}
