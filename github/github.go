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

func DownloadValues(ctx context.Context, client *github.Client, app string) (io.ReadCloser, *github.RepositoryContent, string, error) {
	repo, path := util.GetRepoAndPath(app)
	opts := github.RepositoryContentGetOptions{Ref: "main"}
	// TODO: Setup retry in case github download fails?

	rc, content, _, err := client.Repositories.DownloadContentsWithMeta(ctx, util.Owner, repo, path, &opts)
	if err != nil {
		log.Printf("Error downloading contents with meta: %v", err)
		return nil, nil, "", err
	}
	dlMsg := fmt.Sprintf("_ Downloading %s _", content.GetHTMLURL())
	return rc, content, dlMsg, err
}

func UpdateValues(rc io.Reader, imgTag string) ([]byte, error, string) {
	bytes, _ := io.ReadAll(rc)
	oldValues := string(bytes)
	newValues, err, msg := updateValuesFileContent(oldValues, imgTag)
	return newValues, err, msg
}

func updateValuesFileContent(content, imgTag string) ([]byte, error, string) {
	m := make(map[interface{}]interface{})
	err := yaml.Unmarshal([]byte(content), &m)
	if err != nil {
		log.Printf("Error updating values file content: %v", err)
	}
	// TODO: FIX THIS
	tag := m["image"].(map[interface{}]interface{})["tag"]
	if tag != imgTag {
		m["image"].(map[interface{}]interface{})["tag"] = imgTag
	} else {
		return nil, nil, fmt.Sprintf("_The image tag is already set to `%s`_", imgTag)
	}
	bytes, err := yaml.Marshal(m)
	return bytes, err, ""
}

func PushCommit(ctx context.Context, client *github.Client, app, imgTag string, values []byte, content *github.RepositoryContent) error {
	repo, path := util.GetRepoAndPath(app)
	branch := "main"
	commitMsg := fmt.Sprintf("Deploy %s:%s", app, imgTag)
	opts := github.RepositoryContentFileOptions{
		Message: &commitMsg,
		Branch:  &branch,
		Content: values,
		SHA:     content.SHA,
	}

	_, _, err := client.Repositories.UpdateFile(ctx, util.Owner, repo, path, &opts)
	if err != nil {
		log.Printf("Error updating file: %s", err.Error())
		return err
	}
	return nil
}

// Check that all checks have passed on latest commit for specified PR
func ConfirmChecksCompleted(ctx context.Context, client *github.Client, app, sha string, opts *github.ListCheckRunsOptions) bool {
	crr, _, err := client.Checks.ListCheckRunsForRef(ctx, util.Owner, app, sha, nil)
	if err != nil {
		log.Printf("Error confiring checks completed: %v", err)
	}

	for _, cr := range crr.CheckRuns {
		if cr.GetName() == "promote_image" && cr.GetStatus() == "completed" {
			return true
		}
	}
	return false
}

func GetPullRequest(ctx context.Context, client *github.Client, app string, prNum int) (*github.PullRequest, *github.Response, error) {
	// TODO: Not sure it's best to call PullRequests.Get even when prNum is known to be "main"
	pr, resp, err := client.PullRequests.Get(ctx, util.Owner, app, prNum)
	return pr, resp, err
}
