package aws

import (
	"context"
	"deploy-bot/util"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/google/go-github/v40/github"
	"log"
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

	// pr will be nil if anything other than a positive int was presented as 2nd slackbot arg
	// so then we get the HEAD commit on main
	if pr == nil {
		opts := &github.CommitsListOptions{SHA: "main"}
		repoCommits, _, _ := ghclient.Repositories.ListCommits(ctx, util.Owner, app, opts)
		imgTag = util.BuildDockerImageString("main", *repoCommits[0].SHA)
		sha = *repoCommits[0].SHA
	} else {
		ref := pr.Head.GetRef()
		sha = pr.Head.GetSHA()
		imgTag = util.BuildDockerImageString(ref, sha)
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
