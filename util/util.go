package util

import (
	"context"
	"encoding/json"
	"fmt"
	//	"log"
	"os"
	"strconv"
	"strings"
)

const (
	Owner = "capco-ea"
)

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

func CheckArgsValid(ctx context.Context, args []string) (bool, string) {
	// Show usage in the case no input is provided
	if len(args) == 1 {
		return false, "Usage: @me <app> <pr_number/main>"
	}

	// Check provided number of args are correct
	if len(args) != 3 {
		return false, "Wrong number of args provided"
	}

	// Check ref arg is either PR num or main branch
	_, err := strconv.Atoi((args[2]))
	if err != nil && args[2] != "main" {
		return false, "You can only deploy a PR or main branch"
	}

	// Check provided app is included in supported apps array
	validApp := false
	for _, app := range getApps() {
		if app == args[1] {
			validApp = true
			break
		}
	}
	if validApp == false {
		return false, fmt.Sprintf("Supported apps include %s", getApps())
	}
	return true, "success"
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
