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
	//"time"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

// Bad global var that can be read between handlers to enable threaded Slack responses
var slackEventTimestamp = "thisIsAHugeAntiPattern"
var slackEventChannel = "C02MYKCQG9W"

func doEvent(event *slackevents.AppMentionEvent, connInfo slackbot.ConnInfo) {
	log.Printf("Event received: %s", event.Text)
	// TODO: Implement additional contexts for subsequent requests
	ctx, ghc := github.Client()
	valid, msg, app, ref := util.CheckArgsValid(event.Text)
	if valid != true {
		slackbot.SendMessage(connInfo, msg)
		return
	}
	prNum, _ := strconv.Atoi(ref)
	pr, resp, err := github.GetPullRequest(ctx, ghc, app, prNum)

	if resp.StatusCode == 200 { // A known PR was provided
		msg := fmt.Sprintf("_Fetching %v _", pr.GetHTMLURL())
		slackbot.SendMessage(connInfo, msg)
	} else if resp.StatusCode == 404 && ref != "main" { // Non-main branch was provided
		msg := fmt.Sprintf("_Error: %s_", err)
		slackbot.SendMessage(connInfo, msg)
		return
	} else { // Main branch was provided
		msg := fmt.Sprintf("_Fetching `%s` for %s _", ref, app)
		slackbot.SendMessage(connInfo, msg)
	}

	tagExists, imgTag, sha := aws.ConfirmImageExists(ctx, ghc, pr, app)
	if tagExists != true {
		msg := fmt.Sprintf("_`%s` does not exist in ECR_", imgTag)
		slackbot.SendMessage(connInfo, msg)
		return
	}

	completed := github.ConfirmChecksCompleted(ctx, ghc, app, sha, nil)
	if completed != true {
		msg := fmt.Sprintf("`_%s` has not been promoted to ECR; Github Actions are still underway_", imgTag)
		slackbot.SendMessage(connInfo, msg)
		return
	}

	rc, repoContent, dlMsg, err := github.DownloadValues(ctx, ghc, app)
	if err != nil {
		msg := fmt.Sprintf("_Error %s_", err.Error())
		slackbot.SendMessage(connInfo, msg)
		return
	} else {
		slackbot.SendMessage(connInfo, dlMsg)
	}

	values, _, msg := github.UpdateValues(rc, imgTag)
	if msg != "" {
		slackbot.SendMessage(connInfo, msg)
		return
	}

	// This triggers Github webhook with request inbound for /githook
	if github.PushCommit(ctx, ghc, app, imgTag, values, repoContent); err != nil {
		msg := fmt.Sprintf("_Error %s_", err.Error())
		slackbot.SendMessage(connInfo, msg)
		return
	} else {
		deployMsg := fmt.Sprintf("_Updating image.tag to `%s`_", imgTag)
		slackbot.SendMessage(connInfo, deployMsg)
	}
}

func doHook(body []byte, connInfo slackbot.ConnInfo) {
	//TODO: Have Adam create unique GH user with PAT that can be used to identify as Slackbot user
	app, err := util.GetAppFromPayload(body)
	if err != nil {
		log.Printf("Error parsing app from git webhook payload: %s", err.Error())
		msg := fmt.Sprintf("_Error parsing app from git webhook payload: %s_", err.Error())
		slackbot.SendMessage(connInfo, msg)
		return
	}
	argoc := argo.Client()
	//if err := argo.HardRefresh(argoc); err != nil {
	//	//log.Printf("Error refreshing Argo application: %s", err.Error())
	//}
	payload := bytes.NewReader(body)
	if msg, err := argo.ForwardGitshot(argoc, payload); err != nil {
		log.Printf("Error forwarding gitshot to Argocd: %s", err.Error())
		slackbot.SendMessage(connInfo, msg)
		return
	}

	if msg, err := argo.SyncApplication(argoc, app); err != nil {
		log.Printf("Error syncing application in Argocd: %s", err.Error())
		slackbot.SendMessage(connInfo, msg)
		return
	} else {
		go argo.DoStatusLoop(argoc, app, connInfo)
		slackbot.SendMessage(connInfo, msg) //comment if desired "Argocd application Sync underway"
	}
}

func main() {
	// TODO: Remove this when all testing is complete
	godotenv.Load(".env")
	http.HandleFunc("/githook", gitHook)
	http.HandleFunc("/slackevent", slackEvent)
	s := &http.Server{
		Addr: fmt.Sprintf(":%s", os.Getenv("PORT")),
	}
	log.Printf("[INFO] Server listening on localhost:%s", os.Getenv("PORT"))
	s.ListenAndServe()
}

func gitHook(w http.ResponseWriter, r *http.Request) {
	log.Printf("Githook received: %v", r)
	body, _ := io.ReadAll(r.Body)
	connInfo := slackbot.ConnInfo{
		Client:    slackbot.Client(),
		Channel:   slackEventChannel,
		Timestamp: slackEventTimestamp,
	}

	if len(body) == 0 {
		log.Printf("Could not read gitHook request body, body length: %d", len(body))
		w.WriteHeader(http.StatusBadRequest)
		return
	} else {
		w.WriteHeader(http.StatusAccepted)
		defer r.Body.Close()

		switch util.ConfirmCallerSlackbot(body) {
		case true:
			go doHook(body, connInfo)
		default:
			log.Printf("Caller not Slackbot, returning...")
			return
		}
	}
}

func slackEvent(w http.ResponseWriter, r *http.Request) {
	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")
	body, err := io.ReadAll(r.Body)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

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
			connInfo := slackbot.ConnInfo{
				Client:    slackbot.Client(),
				Channel:   e.Channel,
				Timestamp: e.TimeStamp, // Required for threaded responses
			}
			// Modifying a global variable like this huge-anti pattern,
			// but it's the only solution I have for now that solves the problem
			// of passing the original slackEvent timestamp to the the githook handler
			slackEventTimestamp = e.TimeStamp
			slackEventChannel = e.Channel
			if e.Channel == os.Getenv("PROTECTED_CHANNEL") { // If the channel is deployments-production
				authorized := util.AuthorizeUser(e.User)
				if authorized != true {
					msg := fmt.Sprintf("_あなたはふさわしくない_")
					slackbot.SendMessage(connInfo, msg)
					return
				}
			}
			go doEvent(e, connInfo)

		default:
			return
		}
	}
}
