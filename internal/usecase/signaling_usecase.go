package usecase

import (
	"log/slog"

	"grok_voice/internal/domain/message"
	"grok_voice/internal/infrastructure/repository"

	"github.com/pion/webrtc/v3"
)

type SignalingUsecase interface {
	ProcessSignal(msg message.SignalingMessage) error
}

type SignalingUC struct {
	wsConnRepo repository.WsConnectionsRepository
}

func NewSignalingUC(wsConnRepo repository.WsConnectionsRepository) *SignalingUC {
	return &SignalingUC{wsConnRepo: wsConnRepo}
}

func (s *SignalingUC) ProcessSignal(message.SignalingMessage) error {
	switch msg.Type {
	case MsgTypeJoin:
		s.RoomsMu.Lock()
		room, ok := s.Rooms[msg.RoomID]
		if !ok {
			room = NewRoom(msg.RoomID)
			s.Rooms[msg.RoomID] = room
		}
		s.RoomsMu.Unlock()
		room.AddClient(client)

		clients := room.GetClients()
		participants := make([]string, 0, len(clients))
		for id := range clients {
			participants = append(participants, id)
		}
		return WebSocketMessage{Type: "participants", Participants: participants}, nil

	case MsgTypeOffer:
		if client.PeerConnection == nil {
			pc, err := createPeerConnection()
			if err != nil {
				return WebSocketMessage{
					Type:    MsgTypeError,
					Message: "create connection: " + err.Error(),
				}, nil
			}
			client.PeerConnection = pc

			pc.OnTrack(
				func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
					slog.Info("Track received", "clientID", client.ID)
					forwardTrack(client, track)
				},
			)

			pc.OnICECandidate(
				func(c *webrtc.ICECandidate) {
					if c != nil {
						slog.Info("Sending ICE candidate", "candidate", c.ToJSON())
						if err := client.Conn.WriteJSON(
							WebSocketMessage{
								Type: MsgTypeCandidate,
								// TODO костылик временно
								Candidate: func() *webrtc.ICECandidateInit {
									ice := c.ToJSON()
									return &ice
								}(),
							},
						); err != nil {
							slog.Error("send ICE candidate", "error", err)
						}
					}
				},
			)
		}
		answer, err := handleOffer(client.PeerConnection, *msg.SDP)
		if err != nil {
			return WebSocketMessage{Type: MsgTypeError, Message: "process offer: " + err.Error()}, nil
		}
		return WebSocketMessage{Type: "answer", SDP: &answer}, nil

	case MsgTypeCandidate:
		if client.PeerConnection == nil {
			return WebSocketMessage{Type: MsgTypeError, Message: "PeerConnection not initialized"}, nil
		}
		if err := addICECandidate(client.PeerConnection, *msg.Candidate); err != nil {
			return WebSocketMessage{Type: MsgTypeError, Message: "add candidate: " + err.Error()}, nil
		}
		return WebSocketMessage{}, nil

	case MsgTypeMute:
		client.MuteClient(msg.TargetClientID)
		return WebSocketMessage{Type: "mute_ack"}, nil

	case MsgTypeUnmute:
		client.UnmuteClient(msg.TargetClientID)
		return WebSocketMessage{Type: "unmute_ack"}, nil

	case MsgTypeSetVolume:
		if msg.Volume != nil {
			client.SetVolume(msg.TargetClientID, *msg.Volume)
			return WebSocketMessage{Type: "volume_ack"}, nil
		}

	case MsgTypeGetParticipants:
		clients := client.Room.GetClients()
		participants := make([]string, 0, len(clients))
		for id := range clients {
			participants = append(participants, id)
		}
		return WebSocketMessage{Type: "participants", Participants: participants}, nil
	}
	return WebSocketMessage{}, nil
}

// createPeerConnection — create WebRTC PeerConnection
func createPeerConnection() (*webrtc.PeerConnection, error) {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		slog.Error("create PeerConnection", "error", err)
		return nil, err
	}
	slog.Info("PeerConnection created")
	return pc, nil
}

// handleOffer — handle SDP offer
func handleOffer(pc *webrtc.PeerConnection, offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	if err := pc.SetRemoteDescription(offer); err != nil {
		slog.Error("set remote description", "error", err)
		return webrtc.SessionDescription{}, err
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		slog.Error("create answer", "error", err)
		return webrtc.SessionDescription{}, err
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		slog.Error("set local description", "error", err)
		return webrtc.SessionDescription{}, err
	}
	slog.Info("Answer created")
	return answer, nil
}

// addICECandidate — add ICE candidate
func addICECandidate(pc *webrtc.PeerConnection, candidate webrtc.ICECandidateInit) error {
	if err := pc.AddICECandidate(candidate); err != nil {
		slog.Error("add ICE candidate", "error", err)
		return err
	}
	slog.Info("ICE candidate added")
	return nil
}

// forwardTrack — forward audio track to other clients
func forwardTrack(sender *domain.Client, track *webrtc.TrackRemote, room *domain.Room) {
	clients := room.GetClients()
	for id, client := range clients {
		if id == sender.ID || client.PeerConnection == nil {
			continue
		}
		if client.IsMuted(sender.ID) {
			slog.Info("Skipping muted client", "clientID", id)
			continue
		}
		volume := client.GetVolume(sender.ID)
		slog.Info("Forwarding track", "from", sender.ID, "to", id, "volume", volume)

		localTrack, err := webrtc.NewTrackLocalStaticRTP(
			track.Codec().RTPCodecCapability,
			"audio",
			"stream_"+sender.ID.String(),
		)
		if err != nil {
			slog.Error("create local track", "error", err)
			continue
		}
		senderTrack, err := client.PeerConnection.AddTrack(localTrack)
		if err != nil {
			slog.Error("add track", "error", err)
			continue
		}

		go func(tLocal *webrtc.TrackLocalStaticRTP) {
			for {
				pkt, _, err := track.ReadRTP()
				if err != nil {
					slog.Error("read RTP", "error", err)
					break
				}
				if err := tLocal.WriteRTP(pkt); err != nil {
					slog.Error("write RTP", "error", err)
					break
				}
			}
			if client.PeerConnection != nil {
				client.PeerConnection.RemoveTrack(senderTrack)
			}
			slog.Info("Track forwarding stopped", "from", sender.ID, "to", id)
		}(localTrack)
	}
}
