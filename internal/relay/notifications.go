package relay

import "copperline/internal/model"

// Called with syncMu held, so attachment and live delivery have one boundary.
func (s *Server) deliverMessage(msg model.Message, self string) {
	relayOwns := s.notifier.Enabled()
	s.peersMu.Lock()
	peers := make([]*serverPeer, 0, len(s.peers))
	var owner *serverPeer
	for p := range s.peers {
		peers = append(peers, p)
		if p.notifications && (owner == nil || p.order < owner.order) {
			owner = p
		}
	}
	s.peersMu.Unlock()
	if relayOwns {
		s.notifier.NotifyMessage(s.cfg.Gotify, msg, self)
	}
	for _, p := range peers {
		copy := msg
		copy.SuppressNotify = relayOwns || p != owner
		_ = p.send(frame{Type: "message", Message: &copy})
	}
}
