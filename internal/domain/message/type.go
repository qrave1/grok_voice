package message

// Константы для типов сообщений WebSocket
const (
	MsgTypeJoin            = "join"
	MsgTypeOffer           = "offer"
	MsgTypeCandidate       = "candidate"
	MsgTypeMute            = "mute"
	MsgTypeUnmute          = "unmute"
	MsgTypeSetVolume       = "set_volume"
	MsgTypeGetParticipants = "get_participants"
	MsgTypeError           = "error"
)
