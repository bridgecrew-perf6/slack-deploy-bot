package github

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"deploy-bot/util"
	"github.com/google/go-github/v40/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
)

func Client() (context.Context, *github.Client) {
	godotenv.Load(".env")
	token := os.Getenv("GITHUB_API_TOKEN")
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	return ctx, client
}

func DownloadValues(ctx context.Context, ghclient *github.Client, app string) (io.ReadCloser, *github.RepositoryContent, string, error) {
	repo, path := util.GetRepoAndPath(app)
	repoOpts := github.RepositoryContentGetOptions{Ref: "main"}
	// TODO: Setup retry in case github download fails?
	rdClser, repoContent, _, err := ghclient.Repositories.DownloadContentsWithMeta(ctx, util.Owner, repo, path, &repoOpts)
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

func PushCommit(ctx context.Context, ghclient *github.Client, app, imgTag string, valuesFile []byte, repoContent *github.RepositoryContent) (string, error) {
	repo, path := util.GetRepoAndPath(app)
	branch := "main"
	commitMsg := fmt.Sprintf("Deploy %s:%s", app, imgTag)
	repoCFO := github.RepositoryContentFileOptions{
		Message: &commitMsg,
		Branch:  &branch,
		Content: valuesFile,
		SHA:     repoContent.SHA}

	// This triggers Github webhook with request inbound for /githook
	repoRespContent, _, err := ghclient.Repositories.UpdateFile(ctx, util.Owner, repo, path, &repoCFO)
	// TODO: Return newly created commit URL in slack message
	if err != nil {
		log.Fatalf("Error %s", err.Error())
		return "", err
	}
	fmt.Println(repoRespContent.GetHTMLURL())
	deployMsg := fmt.Sprintf("Updating image.tag to %s", imgTag)
	return deployMsg, err
}

// Check that all checks have passed on latest commit for specified PR
func ConfirmChecksCompleted(ctx context.Context, ghclient *github.Client, app, sha string, opts *github.ListCheckRunsOptions) bool {
	checkRunResults, _, err := ghclient.Checks.ListCheckRunsForRef(ctx, util.Owner, app, sha, nil)
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
