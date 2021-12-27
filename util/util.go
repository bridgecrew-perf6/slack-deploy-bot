package util

import (
	"encoding/json"
	"fmt"
	//	"net/http"
	"os"
	"strconv"
	"strings"
	//	"time"
)

const (
	Owner = "capco-ea"
)

func AuthorizeUser(user string) bool {
	users := strings.Split(os.Getenv("AUTHORIZED_USERS"), ",")
	for _, u := range users {
		if u == user {
			return true
		}
	}
	return false
}

// Explicitly declare supported apps instead of make additional network call to Github
func getApps() []string {
	apps := strings.Split(os.Getenv("SUPPORTED_APPS"), ",")
	return apps
}

func BuildDockerImageString(ref, sha string) *string {
	tag := ref + "-" + sha[:7]
	return &tag
}

func GetRepoAndPath(app string) (string, string) {
	repo := os.Getenv("GITOPS_REPO")
	path := fmt.Sprintf("%s/values.yaml", app)
	return repo, path
}

func CheckAppValid(app string) bool {
	for _, a := range getApps() {
		if a == app {
			return true
		}
	}
	return false
}

func CheckArgsValid(event string) (bool, string, string, string) {
	args := strings.Split(event, " ")
	// Check provided number of args are correct
	if len(args) != 3 {
		msg := fmt.Sprintf("_議論が多すぎます, translation: Usage: @%s <app> <pr_number/main>_", os.Getenv("SLACKBOT_NAME"))
		return false, msg, "", ""
	}

	// Check provided app is included in supported apps array
	valid := CheckAppValid(args[1])
	if valid != true {
		msg := fmt.Sprintf("_私は認識しません, translation: I do not recognize %s app_", args[1])
		return false, msg, "", ""
	}

	// Check ref arg is either PR num or main branch
	num, _ := strconv.Atoi((args[2])) // Atoi will return 0 for any string
	if num < 0 {
		msg := fmt.Sprintf("_それは一体何だ?, translation: You do not want to know_")
		return false, msg, "", ""
	} else if num == 0 && args[2] != "main" {
		msg := fmt.Sprintf("_私は認識しません, translation: I do not recognize %s_ ref", args[2])
		return false, msg, "", ""
	}
	app := args[1]
	ref := args[2]

	return true, "", app, ref
}

func GetAppFromPayload(body []byte) string {
	application := make(map[string]interface{})
	json.Unmarshal(body, &application)
	commit := application["head_commit"]
	modified := commit.(map[string]interface{})["modified"]
	str := modified.([]interface{})[0].(string)
	app := strings.TrimSuffix(str, "/values.yaml")
	return app
}

// The Github hook should only be forwarded to Argo if initiated by the slackbot
func ConfirmCallerSlackbot(body []byte) bool {
	var githook map[string]interface{}
	json.Unmarshal(body, &githook)
	pusher := githook["pusher"]
	name, _ := pusher.(map[string]interface{})["name"]
	if name == os.Getenv("SLACKBOT_NAME") {
		return true
	} else {
		return false
	}
}

//func WaitForServer(url string) error {
//	util.WaitForResponse(client.Repositories.DownloadContentsWithMeta(ctx, util.Owner, repo, path, &repoOpts))
//	const timeout = 5 * time.Second
//	deadline := time.Now().Add(timeout)
//	for tries := 0; time.Now().Before(deadline); tries++ {
//		_, err := http.Head(url)
//		if err == nil {
//			return nil // success
//		}
//		log.Printf("server not responding (%s); retrying...", err)
//		time.Sleep(time.Second << uint(tries)) // exponential back-off
//	}
//	return fmt.Errorf("server %s failed to respond after %s", url, timeout)
//}
