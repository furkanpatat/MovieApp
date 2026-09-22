// Package hub implements the Watch-Party real-time engine: a Hub that manages
// dynamically created Rooms, each fanning messages out to its Clients.
//
// Wire protocol (JSON text frames)
//
//	client -> server
//	  {"type":"chat_message","text":"hello"}
//	  {"type":"playback_sync","action":"play|pause|seek","timestamp":123.4}   // seconds
//
//	server -> clients (user_id is attached by the server, never trusted from the payload)
//	  {"type":"chat_message","room":"r","user_id":"u","text":"hello","sent_at":"..."}
//	  {"type":"playback_sync","room":"r","user_id":"u","action":"seek","timestamp":123.4,"sent_at":"..."}
//	  {"type":"user_joined"|"user_left","room":"r","user_id":"u","sent_at":"..."}
//	  {"type":"room_state","room":"r","participants":["u",...],"playback":{...}|null}   // to a new joiner only
//	  {"type":"error","message":"..."}                                                  // to the sender only
package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// Message types.
const (
	TypeChat         = "chat_message"
	TypePlaybackSync = "playback_sync"
	TypeUserJoined   = "user_joined"
	TypeUserLeft     = "user_left"
	TypeRoomState    = "room_state"
	TypeError        = "error"
)

// Playback actions.
const (
	ActionPlay  = "play"
	ActionPause = "pause"
	ActionSeek  = "seek"
)

// maxTimestamp bounds a playback position (seconds): one week.
const maxTimestamp = 7 * 24 * 3600

// inbound is what a client may send. Anything else (user_id, room, ...) is ignored.
type inbound struct {
	Type      string   `json:"type"`
	Text      string   `json:"text"`
	Action    string   `json:"action"`
	Timestamp *float64 `json:"timestamp"`
}

// Playback is the room's last known playback state (sent to late joiners).
type Playback struct {
	Action    string    `json:"action"`
	Timestamp float64   `json:"timestamp"`
	UserID    string    `json:"user_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

// badMessage is a client mistake: reported back to the sender, connection stays open.
type badMessage struct{ reason string }

func (e badMessage) Error() string { return e.reason }

var errBinary = badMessage{"binary frames are not supported; send JSON text"}

// parse validates a client frame and builds the outbound (server-attributed)
// message. playback is non-nil for playback_sync.
func parse(data []byte, userID, room string, maxRunes int, now time.Time) (out []byte, pb *Playback, err error) {
	var in inbound
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, nil, badMessage{"invalid JSON"}
	}
	switch in.Type {
	case TypeChat:
		text := strings.TrimSpace(in.Text)
		if text == "" {
			return nil, nil, badMessage{"text is required"}
		}
		if utf8.RuneCountInString(text) > maxRunes {
			return nil, nil, badMessage{fmt.Sprintf("text must be at most %d characters", maxRunes)}
		}
		out, err = json.Marshal(map[string]any{
			"type": TypeChat, "room": room, "user_id": userID, "text": text, "sent_at": now,
		})
		return out, nil, err

	case TypePlaybackSync:
		switch in.Action {
		case ActionPlay, ActionPause, ActionSeek:
		default:
			return nil, nil, badMessage{"action must be play, pause or seek"}
		}
		if in.Timestamp == nil {
			return nil, nil, badMessage{"timestamp is required"}
		}
		ts := *in.Timestamp
		if math.IsNaN(ts) || math.IsInf(ts, 0) || ts < 0 || ts > maxTimestamp {
			return nil, nil, badMessage{"timestamp must be between 0 and 604800 seconds"}
		}
		pb = &Playback{Action: in.Action, Timestamp: ts, UserID: userID, UpdatedAt: now}
		out, err = json.Marshal(map[string]any{
			"type": TypePlaybackSync, "room": room, "user_id": userID,
			"action": in.Action, "timestamp": ts, "sent_at": now,
		})
		return out, pb, err

	case "":
		return nil, nil, badMessage{"type is required"}
	default:
		return nil, nil, badMessage{"unknown message type"}
	}
}

func presence(typ, room, userID string, now time.Time) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "room": room, "user_id": userID, "sent_at": now})
	return b
}

func roomState(room string, participants []string, pb *Playback) []byte {
	b, _ := json.Marshal(map[string]any{"type": TypeRoomState, "room": room, "participants": participants, "playback": pb})
	return b
}

func errorMessage(reason string) []byte {
	b, _ := json.Marshal(map[string]string{"type": TypeError, "message": reason})
	return b
}

var (
	ErrRoomFull  = errors.New("room is full")
	ErrHubClosed = errors.New("hub is shutting down")
)
