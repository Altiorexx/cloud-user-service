package types

/*
	what was done
	did it go through?
	who did it
	when did it happen
*/

type LogEntry struct {
	GroupId   string `json:"groupId"`
	Action    string `json:"action"`
	Status    string `json:"status"` // did the action go well? transform status code to OK or smthing else
	UserId    string `json:"-"`
	Email     string `json:"email"`
	Timestamp string `json:"timestamp"`
}
