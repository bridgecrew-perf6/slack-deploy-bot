package main

import (
	"bytes"
	"deploy-bot/argo"
	"deploy-bot/aws"
	"deploy-bot/github"
	slackbot "deploy-bot/slack"
	"deploy-bot/util"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

func run(event *slackevents.AppMentionEvent) {
	log.Printf("Arguments received %s", event.Text)
	slackChannel := event.Channel
	slackClient := slackbot.Client()
	// TODO: Implement additional contexts for subsequent requests
	ctx, githubClient := github.Client()

	args := strings.Split(event.Text, " ")
	valid, errMsg := util.CheckArgsValid(ctx, args)
	if valid != true {
		slackbot.SendMessage(slackClient, slackChannel, errMsg)
		log.Printf("%s", errMsg)
		return
	}

	app := args[1]
	ref := args[2]
	prNum, _ := strconv.Atoi(ref)
	pr, resp, err := github.GetPullRequest(ctx, githubClient, app, prNum)

	if resp.StatusCode == 200 {
		msg := fmt.Sprintf("Fetching %v", pr.GetHTMLURL())
		slackbot.SendMessage(slackClient, slackChannel, msg)
	} else if resp.StatusCode == 404 && ref != "main" {
		msg := fmt.Sprintf("Error: %s", err)
		slackbot.SendMessage(slackClient, slackChannel, msg)
	} else {
		msg := fmt.Sprintf("Fetching %s %s", app, ref)
		slackbot.SendMessage(slackClient, slackChannel, msg)
	}

	return

	tagExists, imgTag, sha := aws.ConfirmImageExists(ctx, githubClient, pr, app)
	if tagExists != true {
		msg := fmt.Sprintf("%s does not exist in ECR", imgTag)
		slackbot.SendMessage(slackClient, slackChannel, msg)
		return
	}

	completed := github.ConfirmChecksCompleted(ctx, githubClient, app, sha, nil)
	if completed != true {
		msg := fmt.Sprintf("%s has not been promoted to ECR; Github Actions are still underway", imgTag)
		slackbot.SendMessage(slackClient, slackChannel, msg)
		return
	}

	rdClser, repoContent, dlMsg, err := github.DownloadValues(ctx, githubClient, app)
	if err != nil {
		msg := fmt.Sprintf("Error %s", err.Error())
		slackbot.SendMessage(slackClient, slackChannel, msg)
		return
	} else {
		slackbot.SendMessage(slackClient, slackChannel, dlMsg)
	}

	newVFC, _, msg := github.UpdateValues(rdClser, imgTag)
	if msg != "" {
		slackbot.SendMessage(slackClient, slackChannel, msg)
		return
	}

	deployMsg, err := github.PushCommit(ctx, githubClient, app, imgTag, newVFC, repoContent)
	if err != nil {
		msg := fmt.Sprintf("Error %s", err.Error())
		slackbot.SendMessage(slackClient, slackChannel, msg)
		return
	} else {
		slackbot.SendMessage(slackClient, slackChannel, deployMsg)
	}
	// TODO: The status may return as Synced before the Argo server has received or processed
	// the webhook, so figure out best way to confirm that webhook has been received and status
	// is Progressing before breaking out of loop
	time.Sleep(time.Second * 2) // Argo typically starts processing webhooks in <1s upon receipt

	for {
		client := argo.Client()
		deployStatus := argo.GetArgoDeploymentStatus(client, app)
		// TODO: Figure out how to format status output "map[time-app:Synced time-sidekiq:Synced] nicely"
		//slackbot.SendMessage(slackClient, slackChannel, deployStatus)
		slackClient.PostMessage(event.Channel, slack.MsgOptionText(fmt.Sprintf("%s", deployStatus), false))
		dSynced := 0
		for _, status := range deployStatus {
			if status == "Synced" {
				dSynced += 1
				continue
			} else {
				break
			}
		}
		time.Sleep(time.Second * 4)
		// The app and sidekiq deployments have Synced, which are good proxies for complete application Sync
		if dSynced == 2 {
			break
		}
	}
	//The typical flow of a multithreaded program in Go involves setting up communication channels,
	// and then passing these channels to all goroutines which need to communicate.
	// Worker goroutines send processed data to the channel, and goroutines which need
	// to wait on work done by others will do so by receiving from this channel.
	return
}

func main() {
	// TODO: Remove this when all testing is complete
	godotenv.Load(".env")

	// Listen for Github webhook
	http.HandleFunc("/gitshot", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		payload := bytes.NewReader(body)
		if err != nil {
			log.Fatalf("Error %s", err.Error())
		}
		callerSlackbot := util.ConfirmCallerSlackbot(body)
		//TODO: Have Adam create unique GH user with PAT that can be used to identify as Slackbot user
		if callerSlackbot == true {
			client := argo.Client()
			err := argo.ForwardGitshot(client, payload)
			if err != nil {
				return
			}
			app := util.GetAppFromPayload(body)
			argo.SyncApplication(client, app)
		} else {
			return
		}
	})

	// Listen for slackevents
	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		signingSecret := os.Getenv("SLACK_SIGNING_SECRET")
		body, err := io.ReadAll(r.Body)

		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		sv, err := slack.NewSecretsVerifier(r.Header, signingSecret)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if _, err := sv.Write(body); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err := sv.Ensure(); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		event, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if event.Type == slackevents.URLVerification {
			var r *slackevents.ChallengeResponse
			err := json.Unmarshal([]byte(body), &r)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text")
			w.Write([]byte(r.Challenge))
		}

		innerEvent := event.InnerEvent
		if event.Type == slackevents.CallbackEvent {
			w.Header().Set("X-Slack-No-Retry", os.Getenv("SLACK_NO_RETRY"))

			switch e := innerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				go run(e)
				return
			}
		}
	})

	fmt.Println("[INFO] Server listening ...")
	http.ListenAndServe(fmt.Sprintf(":%s", os.Getenv("PORT")), nil)
}
