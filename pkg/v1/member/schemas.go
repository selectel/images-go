package member

import "time"

// Member statuses reported by the API.
const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
)

// View is an image member as the API reports it. MemberID is the project that was granted access.
type View struct {
	MemberID  string    `json:"member_id"`
	ImageID   string    `json:"image_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Schema    string    `json:"schema"`
}

type listEnvelope struct {
	Members []View `json:"members"`
}
