package util

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/google/go-github/v40/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
)

const (
	Owner = "capco-ea"
)

func ecrSession() *ecr.ECR {
	sess := session.Must(session.NewSession())
	return ecr.New(sess)

}

func getEcrImages(svc *ecr.ECR, app string) (*ecr.ListImagesOutput, error) {
	input := ecr.ListImagesInput{RepositoryName: &app}
	images, err := svc.ListImages(&input)
	return images, err

}

// Checks to ensure the image exists in ECR
func ConfirmImageExists(ctx context.Context, ghclient *github.Client, pr *github.PullRequest, app string) (bool, string, string) {
	svc := ecrSession()
	var imgTag *string
	var sha string

	if pr == nil {
		opts := &github.CommitsListOptions{SHA: "main"}
		repoCommits, _, _ := ghclient.Repositories.ListCommits(ctx, Owner, app, opts)
		imgTag = buildDockerImageString("main", *repoCommits[0].SHA)
		sha = *repoCommits[0].SHA
	} else {
		ref := pr.Head.GetRef()
		sha = pr.Head.GetSHA()
		imgTag = buildDockerImageString(ref, sha)
	}

	images, err := getEcrImages(svc, app)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	for _, img := range images.ImageIds {
		if *img.ImageTag == *imgTag {
			return true, *imgTag, sha
		}
	}
	return false, *imgTag, sha
}

// Explicitly declare supported apps instead of make additional network call to Github
func getApps() []string {
	apps := strings.Split(os.Getenv("SUPPORTED_APPS"), ",")
	return apps
}

func buildDockerImageString(ref, sha string) *string {
	tag := ref + "-" + sha[:7]
	return &tag
}

func GithubClient() (context.Context, *github.Client) {
	godotenv.Load(".env")
	token := os.Getenv("GITHUB_API_TOKEN")
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	return ctx, client
}

func DownloadValues(ctx context.Context, ghclient *github.Client, app string) (io.ReadCloser, *github.RepositoryContent, string, error) {
	repo, path := getRepoAndPath(app)
	repoOpts := github.RepositoryContentGetOptions{Ref: "main"}
	// TODO: Setup retry in case github download fails?
	rdClser, repoContent, _, err := ghclient.Repositories.DownloadContentsWithMeta(ctx, Owner, repo, path, &repoOpts)
	if err != nil {
		log.Fatalf("error: %v", err)
		return nil, nil, "", err
	}
	dlMsg := fmt.Sprintf("Downloading contents of %s", repoContent.GetHTMLURL())
	return rdClser, repoContent, dlMsg, err
}

func UpdateValues(rdClser io.Reader, imgTag string) ([]byte, error, string) {
	bytes, _ := io.ReadAll(rdClser)
	valuesFileContent := string(bytes)
	newVFC, err, msg := UpdateValuesFileContent(valuesFileContent, imgTag)
	return newVFC, err, msg
}

func UpdateValuesFileContent(content, imgTag string) ([]byte, error, string) {
	m := make(map[interface{}]interface{})
	err := yaml.Unmarshal([]byte(content), &m)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	// TODO: FIX THIS
	tag := m["image"].(map[interface{}]interface{})["tag"]
	if tag != imgTag {
		m["image"].(map[interface{}]interface{})["tag"] = imgTag
	} else {
		return nil, nil, fmt.Sprintf("The desired image tag is already set to %s", imgTag)
	}
	bytes, err := yaml.Marshal(m)
	return bytes, err, ""
}

func getRepoAndPath(app string) (string, string) {
	repo := os.Getenv("GITOPS_REPO")
	path := fmt.Sprintf("%s/values.yaml", app)
	return repo, path
}

func PushCommit(ctx context.Context, ghclient *github.Client, app, imgTag string, valuesFile []byte, repoContent *github.RepositoryContent) (string, error) {
	repo, path := getRepoAndPath(app)
	branch := "main"
	commitMsg := fmt.Sprintf("Deploy %s:%s", app, imgTag)
	repoCFO := github.RepositoryContentFileOptions{
		Message: &commitMsg,
		Branch:  &branch,
		Content: valuesFile,
		SHA:     repoContent.SHA}

	// This triggers Github webhook with request inbound for /githook
	repoRespContent, _, err := ghclient.Repositories.UpdateFile(ctx, Owner, repo, path, &repoCFO)
	// TODO: Return newly created commit URL in slack message
	if err != nil {
		log.Fatalf("Error %s", err.Error())
		return "", err
	}
	fmt.Println(repoRespContent.GetHTMLURL())
	deployMsg := fmt.Sprintf("Updating image.tag to %s", imgTag)
	return deployMsg, err
}

func CheckArgsValid(ctx context.Context, ghclient *github.Client, args []string) (bool, string) {
	// Show usage in the case no input is provided
	if len(args) == 1 {
		return false, "Usage: @me <app> <pr_number>"
	}

	// Check provided number of args are correct
	if len(args) != 3 {
		return false, "Wrong number of args provided"
	}

	// Check ref arg is either PR or main branch
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

// Check that all checks have passed on latest commit for specified PR
func ConfirmChecksCompleted(ctx context.Context, ghclient *github.Client, app, sha string, opts *github.ListCheckRunsOptions) bool {
	checkRunResults, _, err := ghclient.Checks.ListCheckRunsForRef(ctx, Owner, app, sha, nil)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	for _, crr := range checkRunResults.CheckRuns {
		if (crr.GetName() == "promote_image") && (crr.GetStatus() == "completed") {
			return true
		}
	}
	return false
}

func ArgoClient() *http.Client {
	// TODO: Figure out why argo server returns x509: certificate signed by unknown authority error
	trnsPrt := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: trnsPrt}
	return client
}

func buildRequest(path, method string, payload io.Reader) *http.Request {
	url := fmt.Sprintf("%s/%s", os.Getenv("ARGOCD_SERVER"), path)
	req, err := http.NewRequest(method, url, payload)
	var bearer = "Bearer " + os.Getenv("ARGOCD_JWT")
	req.Header.Add("Authorization", bearer)
	if err != nil {
		log.Fatalf("Error %s", err.Error())
	}
	return req
}

func ForwardGitshot(client *http.Client, payload io.Reader) error {
	// TODO: A more sophisticated way to do this is to forward the request
	// with headers intact instead of reconstructing as a new request
	path := "api/webhook"
	req := buildRequest(path, "POST", payload)
	req.Header.Add("X-Github-Event", "push")
	_, err := client.Do(req)

	if err != nil {
		log.Fatalf("Error %s", err.Error())
		return err
	}
	return nil
}

func SyncApplication(client *http.Client, app string) error {
	//app := "time"
	path := fmt.Sprintf("api/v1/applications/%s/sync", app)
	req := buildRequest(path, "POST", nil)
	_, err := client.Do(req)

	if err != nil {
		log.Fatalf("Error %s", err.Error())
		return err
	}
	return nil
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

func GetArgoDeploymentStatus(client *http.Client, app string) map[string]string {
	path := fmt.Sprintf("api/v1/applications/%s", app)
	req := buildRequest(path, "GET", nil)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error %s", err.Error())
	}

	body, _ := io.ReadAll(resp.Body)
	//	fmt.Println(string(body))
	// TODO: Figure out most idiomatic way to parse this json
	application := make(map[string]interface{})
	json.Unmarshal([]byte(body), &application)
	status := application["status"]
	resources := status.(map[string]interface{})["resources"]
	deploymentStatus := make(map[string]string)
	for _, r := range resources.([]interface{}) {
		if r.(map[string]interface{})["kind"] == "Deployment" {
			name := r.(map[string]interface{})["name"].(string)
			status := r.(map[string]interface{})["status"].(string)
			deploymentStatus[name] = status
		}
	}
	return deploymentStatus
}
