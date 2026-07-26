package store

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrUsernameTaken      = errors.New("username taken")
	ErrBadDayDuration     = errors.New("day duration must be between 60 and 86400 seconds")
	ErrRoomEnded          = errors.New("room has ended")
	ErrRoomNotRunning     = errors.New("room is not running")
	ErrAlreadyJoined      = errors.New("already joined this room")
	ErrCannotStart        = errors.New("room can only be started by its host while in lobby")
	ErrNotStarted         = errors.New("room not started")
	ErrBadOrder           = errors.New("invalid order")
	ErrUnknownInstrument  = errors.New("unknown instrument")
	ErrInsufficientCash   = errors.New("insufficient cash")
	ErrInsufficientShares = errors.New("insufficient shares")
	ErrNotCancellable     = errors.New("order cannot be cancelled")
	ErrBadChatMessage     = errors.New("chat message must be 1-500 characters")
)
