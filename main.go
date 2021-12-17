package main

import (
	"bytes"
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

	//	"github.com/google/go-github/v40/github"
	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

func run(event *slackevents.AppMentionEvent, c chan string) {
	log.Printf("args received %s", event.Text)
	slackToken := os.Getenv("SLACK_AUTH_TOKEN")
	api := slack.New(slackToken)
	ctx, ghclient := util.GithubClient()
	args := strings.Split(event.Text, " ")

	valid, errMsg := util.CheckArgsValid(ctx, ghclient, args)
	if valid != true {
		api.PostMessage(event.Channel, slack.MsgOptionText(fmt.Sprintf("%s", errMsg), false))
		log.Printf("%s", errMsg)
		return
	}
	app := args[1]
	//fmt.Println(c)
	//c <- app
	//x := <-c
	//fmt.Println(x)
	//close(c)

	prNum, _ := strconv.Atoi((args[2]))

	// TODO: Implement additional contexts for subsequent requests
	// TODO: Not sure it's best to call PullRequests.Get even when prNum is intended to be "main"
	pr, resp, err := ghclient.PullRequests.Get(ctx, util.Owner, app, prNum)
	if resp.StatusCode == 200 {
		fetchMsg := fmt.Sprintf("Fetching %v.", pr.GetHTMLURL())
		api.PostMessage(event.Channel, slack.MsgOptionText(fetchMsg, false))
	} else if pr != nil {
		api.PostMessage(event.Channel, slack.MsgOptionText(fmt.Sprintf("Error: %s.", err), false))
		log.Printf("Error: %s", err)
	}

	tagExists, imgTag, sha := util.ConfirmImageExists(ctx, ghclient, pr, app)
	fmt.Println(tagExists, imgTag, sha)
	if tagExists != true {
		//attachments := slack.Attachment{Color: "blue"}
		//params := slack.MsgOption(slack.MsgOptionAttachments(attachments))
		//fmt.Println(params)

		ecrMsg := fmt.Sprintf("%s does not exist in ECR", imgTag)
		api.PostMessage(event.Channel, slack.MsgOptionText(ecrMsg, false))
		log.Printf("%s does not exist in ECR", imgTag)
		return
	}

	completed := util.ConfirmChecksCompleted(ctx, ghclient, app, sha, nil)
	if completed != true {
		actMsg := fmt.Sprintf("Github Actions are still underway for %s", imgTag)
		api.PostMessage(event.Channel, slack.MsgOptionText(actMsg, false))
		log.Printf("%s has not been promoted to ECR", imgTag)
		return
	}

	rdClser, repoContent, dlMsg, err := util.DownloadValues(ctx, ghclient, app)
	if err != nil {
		log.Fatalf("Error %s", err.Error())
		return
	} else {
		api.PostMessage(event.Channel, slack.MsgOptionText(dlMsg, false))
	}

	newVFC, _, msg := util.UpdateValues(rdClser, imgTag)
	if msg != "" {
		api.PostMessage(event.Channel, slack.MsgOptionText(msg, false))
		return
	}

	deployMsg, err := util.PushCommit(ctx, ghclient, app, imgTag, newVFC, repoContent)
	if err != nil {
		log.Fatalf("Error %s", err.Error())
		return
	} else {
		api.PostMessage(event.Channel, slack.MsgOptionText(deployMsg, false))
	}
	// TODO: The status may return as Synced before the Argo server has received or processed
	// the webhook, so figure out best way to confirm that webhook has been received and status
	// is Progressing before breaking out of loop
	time.Sleep(time.Second * 2) // Argo typically starts processing webhooks in <1s upon receipt

	for {
		client := util.ArgoClient()
		dStatus := util.GetArgoDeploymentStatus(client, app)
		// TODO: Figure out how to format status output "map[time-app:Synced time-sidekiq:Synced] nicely"
		api.PostMessage(event.Channel, slack.MsgOptionText(fmt.Sprintf("%s", dStatus), false))
		dSynced := 0
		for _, status := range dStatus {
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

	var c chan string = make(chan string, 1)
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
			client := util.ArgoClient()
			err := util.ForwardGitshot(client, payload)
			if err != nil {
				return
			}

			//			app := <-c
			//fmt.Println(app)
			// TODO: Send app to channel in /events listener and read from that here
			//			util.SyncApplication(client, app)

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
				go run(e, c)
				return
			}
		}
	})

	fmt.Println("[INFO] Server listening ...")
	http.ListenAndServe(fmt.Sprintf(":%s", os.Getenv("PORT")), nil)
}
