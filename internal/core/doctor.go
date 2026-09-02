package core

type CheckStatus string

const (
	CheckOK    CheckStatus = "ok"
	CheckError CheckStatus = "error"
)

type Check struct {
	Name    string      `json:"name"`
	Status  CheckStatus `json:"status"`
	Message string      `json:"message,omitempty"`
}

type DoctorReport struct {
	OK     bool    `json:"ok"`
	Checks []Check `json:"checks"`
}
