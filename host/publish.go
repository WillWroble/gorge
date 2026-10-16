package host

import "github.com/adams-shaun/gorge/protocol"

// Publish pushes one embedder-built frame (eventhost's draft-event frames)
// to session s with the contract table frames have: it never blocks, the
// frame gets the session's next id and is kept in its resume ring, and a
// session whose channel is full overflows, is closed and is unregistered.
// It reports whether the frame was queued — false once s is closed or
// overflowed, which tells the caller to forget the subscriber.
func (r *Registry) Publish(s *Session, f protocol.Frame) bool {
	ok := s.push(f)
	r.dropOverflowed([]*Session{s})
	return ok
}
