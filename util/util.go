package util

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
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
func GetApps() []string {
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

func checkAppValid(app string) bool {
	for _, a := range GetApps() {
		if a == app {
			return true
		}
	}
	return false
}

func CheckArgsValid(ctx context.Context, event string) (bool, string, string, string) {
	args := strings.Split(event, " ")
	// Check provided number of args are correct
	if len(args) != 3 {
		msg := fmt.Sprintf("_議論が多すぎます, translation: Usage: @%s <app> <pr_number/main>_", os.Getenv("SLACKBOT_NAME"))
		return false, msg, "", ""
	}

	// Check provided app is included in supported apps array
	valid := checkAppValid(args[1])
	if valid != true {
		msg := fmt.Sprintf("_私は認識しません, translation: I do not recognize %s app_", args[1]) //
		return false, msg, "", ""
	}

	// Check ref arg is either PR num or main branch
	_, err := strconv.Atoi((args[2]))
	if err != nil && args[2] != "main" {
		msg := fmt.Sprintf("_私は認識しません, translation: I do not recognize %s_ ref", args[2])
		return false, msg, "", ""
	}
	app := args[1]
	ref := args[2]
	return true, "", app, ref
}

//func parseEventString(eventText string) (string, string) {
//	pattern := regexp.MustCompile(`\s+`)
//	args := strings.Split((pattern.ReplaceAllString(eventText, " ")), " ")
//	return args[1], args[2]
//}

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
