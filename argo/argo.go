package argo

import (
	"crypto/tls"
	slackbot "deploy-bot/slack"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func Client() *http.Client {
	t := &http.Transport{
		//TLSHandshakeTimeout: 0,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			//SessionTicketsDisabled: true,
			//VerifyConnection:       tls.ConnectionState{HandshakeComplete: true},
			//Time: time.Time{},
		},
		//DisableKeepAlives: false,
		//		MaxIdleConns:        0,
		//		MaxConnsPerHost:     0,
		//		MaxIdleConnsPerHost: 0,
		//		IdleConnTimeout:     0,
		//		ForceAttemptHTTP2:   true,
	}
	client := &http.Client{
		Transport: t,
		Timeout:   time.Second * 20, // this should be replaced with request scoped ctx timeouts
	}
	return client
}

func buildRequest(path, method string, payload io.Reader) *http.Request {
	url := fmt.Sprintf("%s/%s", os.Getenv("ARGOCD_SERVER"), path)
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		log.Printf("Error building argo request: %s", err.Error())
	}
	var bearer = "Bearer " + os.Getenv("ARGOCD_JWT")
	req.Header.Add("Authorization", bearer)
	return req
}

// HardRefresh may not explicitly be necessary, and was
// only included to debug the TCP SYN_SENT issue
// when proxying requests through the Palo tun0 interface
// TODO: remove this after deploying this service to the cluster
// and confirming that issue does not exist
//func HardRefresh(client *http.Client) error {
//	path := fmt.Sprintf("api/v1/applications/performance?refresh=hard")
//	req := buildRequest(path, "GET", nil)
//	resp, err := client.Do(req)
//	if err != nil {
//		return err
//	}
//	defer resp.Body.Close()
//	return nil
//}

func ForwardGitshot(client *http.Client, payload io.Reader) (string, error) {
	// TODO: A more sophisticated way to do this is to forward the request
	// with headers intact instead of reconstructing as a new request
	path := "api/webhook"
	req := buildRequest(path, "POST", payload)
	req.Header.Add("X-Github-Event", "push")
	if _, err := client.Do(req); err != nil {
		return fmt.Sprintf("_Error forwarding gitshot to Argocd: `%v`_", err), err
	}
	return fmt.Sprintf("_Argocd received Github webhook_"), nil
}

func SyncApplication(client *http.Client, app string) (string, error) {
	path := fmt.Sprintf("api/v1/applications/%s/sync", app)
	req := buildRequest(path, "POST", nil)
	if _, err := client.Do(req); err != nil {
		return fmt.Sprintf("_Error syncing %s in Argocd: `%v`_", app, err), err
	}
	return fmt.Sprintf("_%s `Sync` underway_", app), nil
}

func DoStatusLoop(argoc *http.Client, app string, connInfo slackbot.ConnInfo) {
	time.Sleep(time.Second * 4) // Argo typically starts processing webhooks in <1s upon receipt
	loopCount := 0
	outOfSyncCount := 0
	unknownCount := 0
	for {
		if loopCount >= 6 {
			path := fmt.Sprintf("applications/%s", app)
			url := fmt.Sprintf("%s/%s", os.Getenv("ARGOCD_SERVER"), path)
			msg := fmt.Sprintf("_ Potential `Sync` error, please investigate: %s _", url)
			slackbot.SendMessage(connInfo, msg)
			return
		}
		status, msg, err := getDeploymentStatus(argoc, app)
		if err != nil {
			log.Printf("_Error getting deployment status: %s _", err)
		}
		if msg != "" {
			slackbot.SendMessage(connInfo, msg)
			return
		}
		syncCount := 0
		if status != nil {
			for d, s := range status {
				msg := fmt.Sprintf("_%s: `%s`_", d, s)
				if !(outOfSyncCount >= 2) {
					slackbot.SendMessage(connInfo, msg)
				} else if !(unknownCount >= 2) {
					slackbot.SendMessage(connInfo, msg)
				} else if syncCount == 2 {
					slackbot.SendMessage(connInfo, msg)
				}
				switch s {
				case "Synced":
					syncCount++
					continue
				case "OutOfSync":
					outOfSyncCount++
					fmt.Printf("outOfSyncCount == %d\n", outOfSyncCount)
				case "Unknown":
					unknownCount++
					fmt.Printf("unknownCount == %d\n", unknownCount)
				default:
					continue
				}
			}
		}
		loopCount++
		fmt.Printf("loopCount == %d\n", loopCount)
		time.Sleep(time.Second * 5)
		if syncCount == 2 { // The app and sidekiq deployments have Synced, representing a good proxy for complete application Sync
			//			msg := fmt.Sprintln("_Status: `Synced`_")
			//			slackbot.SendMessage(connInfo, msg)
			break
		}
	}
}

func getDeploymentStatus(client *http.Client, app string) (map[string]string, string, error) {
	path := fmt.Sprintf("api/v1/applications/%s", app)
	req := buildRequest(path, "GET", nil)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("_Error getting deployment status: `%s`_", err)
		return nil, "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	// TODO: Figure out most idiomatic way to parse this json
	application := make(map[string]interface{})
	json.Unmarshal(body, &application)
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
	return deploymentStatus, "", nil
}
