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

func doEvent(event *slackevents.AppMentionEvent, connInfo slackbot.ConnInfo) {
	log.Printf("Event received, returning...: %s", event.Text)
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
		msg := fmt.Sprintf("_Fetching `%s` for %s app_", ref, app)
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

	if github.PushCommit(ctx, ghc, app, imgTag, values, repoContent); err != nil {
		msg := fmt.Sprintf("_Error %s_", err.Error())
		slackbot.SendMessage(connInfo, msg)
		return
	} else {
		deployMsg := fmt.Sprintf("_Updating image.tag to `%s`_", imgTag)
		slackbot.SendMessage(connInfo, deployMsg)
	}

	// TODO: The status may return as Synced before the Argo server has received or processed
	// the webhook, so figure out best way to confirm that webhook has been received and status
	// is Progressing before breaking out of loop

	//time.Sleep(time.Second * 4) // Argo typically starts processing webhooks in <1s upon receipt
	//argoc := argo.Client()
	//fmt.Printf("argo client: %T", argoc)
	//for {

	//	fmt.Println("inside deploy status for loop now")
	//	status, msg, _ := argo.GetArgoDeploymentStatus(argoc, app)
	//	if msg != "" {
	//		slackbot.SendMessage(connInfo, msg)
	//		return
	//	}
	//	synced := 0

	//	//for d, s := range deployStatus {
	//	//	msg := fmt.Sprintf("_Status: %s:%s_", d, s)
	//	//	slackbot.SendMessage(connInfo, msg)
	//	//}
	//	for d, s := range status {
	//		msg := fmt.Sprintf("_Status: %s:`%s`_", d, s)
	//		slackbot.SendMessage(connInfo, msg)
	//		if s == "Synced" {
	//			synced++
	//			continue
	//		} else {
	//			break
	//		}
	//	}
	//	time.Sleep(time.Second * 4)
	//	if synced == 2 { // The app and sidekiq deployments have Synced, representing a good proxy for complete application Sync
	//		break
	//	}
	//}
}

func doHook(w http.ResponseWriter, body []byte) {
	//TODO: Have Adam create unique GH user with PAT that can be used to identify as Slackbot user
	app, err := util.GetAppFromPayload(body)
	if err != nil {
		log.Printf("\n\nError parsing app from git webhook payload: %s", err.Error())
		return
	}
	argoc := argo.Client()
	if err := argo.HardRefresh(argoc); err != nil {
		log.Printf("\n\nError refreshing Argo application: %s", err.Error())
	}

	payload := bytes.NewReader(body)
	if err := argo.ForwardGitshot(argoc, payload); err != nil {
		log.Printf("\n\nError forwarding gitshot to argocd: %s", err.Error())
		return
	}
	argo.SyncApplication(argoc, app)
}

func main() {
	// TODO: Remove this when all testing is complete
	godotenv.Load(".env")
	http.HandleFunc("/githook", gitHook)
	http.HandleFunc("/slackevent", slackEvent)
	s := &http.Server{
		Addr:    fmt.Sprintf(":%s", os.Getenv("PORT")),
		Handler: nil,
		//ReadTimeout:  30,
		//WriteTimeout: 30,
	}
	log.Printf("[INFO] Server listening on port %s ...", os.Getenv("PORT"))
	s.ListenAndServe()
}

func gitHook(w http.ResponseWriter, r *http.Request) {
	log.Printf("Githook received, returning: %v", r.Header)
	body, _ := io.ReadAll(r.Body)
	if len(body) == 0 {
		log.Printf("Could not read gitHook request body, body length: %d", len(body))
		w.WriteHeader(http.StatusBadRequest)
		return
	} else {
		w.WriteHeader(http.StatusAccepted)
		defer r.Body.Close()

		switch util.ConfirmCallerSlackbot(body) {
		case true:
			go doHook(w, body)
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
