package slack

import (
	"fmt"
	"github.com/slack-go/slack"
	"log"
	"os"
	//	"github.com/slack-go/slack/slackevents"
)

func Client() *slack.Client {
	slackToken := os.Getenv("SLACK_AUTH_TOKEN")
	api := slack.New(slackToken)
	return api
}

func SendMessage(api *slack.Client, channel, msg string) {
	api.PostMessage(channel, slack.MsgOptionText(fmt.Sprintf("%s", msg), false))
	//log.Printf("Error: %s", msg)
	//attachments := slack.Attachment{Color: "blue"}
	//params := slack.MsgOption(slack.MsgOptionAttachments(attachments))
	//fmt.Println(params)
	log.Printf("%s", msg)
}
