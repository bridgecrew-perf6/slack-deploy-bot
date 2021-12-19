package slack

import (
	"errors"
	"log"
	"os"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

func ConnectToSlackViaSocketmode() (*socketmode.Client, error) {

	appToken := os.Getenv("SLACK_APP_TOKEN")
	if appToken == "" {
		return nil, errors.New("SLACK_APP_TOKEN must be set")
	}

	botToken := os.Getenv("SLACK_BOT_TOKEN")
	if botToken == "" {
		return nil, errors.New("SLACK_BOT_TOKEN must be set.")
	}

	api := slack.New(
		botToken,
		slack.OptionDebug(true),
		slack.OptionAppLevelToken(appToken),
		slack.OptionLog(log.New(os.Stdout, "api: ", log.Lshortfile|log.LstdFlags)),
	)

	client := socketmode.New(
		api,
		socketmode.OptionDebug(true),
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)

	return client, nil
}
