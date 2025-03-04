package message

import (
	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
)

// SignalingMessage — structure for WebSocket messages
type SignalingMessage struct {
	Type           string                     `json:"type"`
	RoomID         uuid.UUID                  `json:"roomId,omitempty"`
	ClientID       uuid.UUID                  `json:"clientId,omitempty"`
	TargetClientID uuid.UUID                  `json:"targetClientId,omitempty"`
	SDP            *webrtc.SessionDescription `json:"sdp,omitempty"`
	Candidate      *webrtc.ICECandidateInit   `json:"candidate,omitempty"`
	Volume         *float64                   `json:"volume,omitempty"`
	Participants   []uuid.UUID                `json:"participants,omitempty"`
	Message        string                     `json:"message,omitempty"`
}

func NewErrorMessage(message string) *SignalingMessage {
	return &SignalingMessage{Type: MsgTypeError, Message: message}
}
