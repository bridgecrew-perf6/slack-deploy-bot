package argo

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	//"time"
)

func Client() *http.Client {
	// TODO: Figure out why argo server returns x509: certificate signed by unknown authority error
	trnsPrt := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives: true,
		//MaxIdleConns:      0,
		//MaxConnsPerHost:   0,
	}
	client := &http.Client{
		Transport: trnsPrt,
		//Timeout:   time.Second * 15, // this should be replaced with request scoped ctx timeouts
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

func HardRefresh(client *http.Client) error {
	path := fmt.Sprintf("api/v1/applications/performance?refresh=hard")
	refReq := buildRequest(path, "GET", nil)
	if res, err := client.Do(refReq); err != nil {
		fmt.Printf("\n\nresult from call to hard refresh app: %v; and error: %v", res, err)
		return err
	} else {
		log.Printf("\n\nstatus code hard refresh app :%v", res.StatusCode)
	}
	return nil
}

func ForwardGitshot(client *http.Client, payload io.Reader) error {
	// TODO: A more sophisticated way to do this is to forward the request
	// with headers intact instead of reconstructing as a new request
	path := "api/webhook"
	req := buildRequest(path, "POST", payload)
	req.Header.Add("X-Github-Event", "push")
	if res, err := client.Do(req); err != nil {
		fmt.Printf("\n\nresult from call to api/webhook: %v; and error: %v", res, err)
		return err
	} else {
		log.Printf("\n\nstatus code forwrd gitshot :%v", res.StatusCode)
	}
	return nil
}

func SyncApplication(client *http.Client, app string) error {
	path := fmt.Sprintf("api/v1/applications/%s/sync", app)
	req := buildRequest(path, "POST", nil)
	if res, err := client.Do(req); err != nil {
		fmt.Printf("\n\nresult from call to sync app: %v; and error: %v", res, err)
		return err
	} else {
		log.Printf("\n\nstatus code sync application:%v", res.StatusCode)
	}
	return nil
}

func GetArgoDeploymentStatus(client *http.Client, app string) (map[string]string, string, error) {
	path := fmt.Sprintf("api/v1/applications/%s", app)
	req := buildRequest(path, "GET", nil)
	resp, err := client.Do(req)
	if err != nil {
		msg := fmt.Sprintf("_Error: `%s` - `%s` - %s _", app, resp.Status, err)
		return nil, msg, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("second err")
		msg := fmt.Sprintf("_Error: `%s` - `%s` - %s _", app, resp.Status, err)
		return nil, msg, err
	}
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
