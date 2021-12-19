package slack

import (
	"github.com/slack-go/slack"
	//"log"
	"os"
)

type ConnInfo struct {
	Client    *slack.Client
	Channel   string
	Timestamp string
}

func Client() *slack.Client {
	slackToken := os.Getenv("SLACK_AUTH_TOKEN")
	api := slack.New(slackToken)
	return api
}

func SendMessage(conn ConnInfo, msg string) {
	channel := conn.Channel
	api := conn.Client
	ts := conn.Timestamp
	attachment := buildSlackAttachment(msg)
	api.PostMessage(channel, slack.MsgOptionAttachments(attachment), slack.MsgOptionTS(ts))
}

func buildSlackAttachment(msg string) slack.Attachment {
	attachment := slack.Attachment{
		//		Pretext: "some pretext",
		Text: msg,
		Fields: []slack.AttachmentField{
			slack.AttachmentField{
				Title: "",
				Value: "",
				Short: false,
			},
		},
	}
	return attachment
}
