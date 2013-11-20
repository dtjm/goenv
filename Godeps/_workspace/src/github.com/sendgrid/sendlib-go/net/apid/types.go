package apid

import (
	"fmt"
)

type Error struct {
	Code      int
	Message   string
	Traceback string
	Repro     string
}

func (e *Error) Error() string {
	if e == nil {
		fmt.Printf("e is nil!")
	}
	return fmt.Sprintf("apid.Client error %d: '%s'", e.Code, e.Message)
}

type ParseHostSettings struct {
	UserID            int    `json:"user_id"`
	URL               string `json:"url"`
	SpamCheckOutgoing bool   `json:"spam_check_outgoing"`
	SendRaw           bool   `json:"send_raw"`
}

type TimezoneInfo struct {
	Display  string
	Name     string
	Offset   int
	Timezone string
	ID       int
}

type User struct {
	Active            int
	Id                int
	OutboundClusterID int `json:"outbound_cluster_id"`

	Email, UserName string
	MailDomain      string `json:"mail_domain"`
	UrlDomain       string `json:"url_domain"`
	PlainTextToHTML bool   `json:"plain_text_to_html"`
	PostEventURL    string `json:"post_event_url"`
	TimezoneInfo    `json:"tzInfo"`
}
