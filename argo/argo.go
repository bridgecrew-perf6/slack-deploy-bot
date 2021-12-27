package argo

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func Client() *http.Client {
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
	path := fmt.Sprintf("api/v1/applications/%s/sync", app)
	req := buildRequest(path, "POST", nil)
	_, err := client.Do(req)

	if err != nil {
		log.Fatalf("Error %s", err.Error())
		return err
	}
	return nil
}

func GetArgoDeploymentStatus(client *http.Client, app string) (map[string]string, string) {
	path := fmt.Sprintf("api/v1/applications/%s", app)
	req := buildRequest(path, "GET", nil)
	resp, _ := client.Do(req)
	if resp.StatusCode == 404 {
		msg := fmt.Sprintf("_Error: `%s` %s_", app, resp.Status)
		log.Printf(msg)
		return nil, msg
	}

	body, _ := io.ReadAll(resp.Body)
	//if err != nil {
	//	log.Fatalf("Error %s", err.Error())
	//	return nil
	//}
	// TODO: Figure out most idiomatic way to parse this json
	//By defining suitable Go dat a
	//st ruc tures in this way, we can selec t which par ts of the JSON inp ut to decode and which to discard . Wh en Unmarshal returns, it has filled in the slice wit h the Title infor mat ion; other
	//names in the JSON are ignored.
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

	return deploymentStatus, ""
}
