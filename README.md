### deploy-bot

#### Assumptions and Simplifications

1. Docker image tags should always be of the form `<branch>-<shortsha>`

	a.  Following this convention eliminates the need to download the current workflow run log file to parse out the promoted image tag.


#### TODOs

In order for this to work we need to move from the model where Argo polls Github every 3 minutes for changes to a webhook model.
https://github.com/argoproj/argo-cd/blob/master/docs/operator-manual/webhook.md
This is going to require making DNS changes.


#### How it works

1. The user summons the bot and passes to it arguments of an application and PR/branch
2. The bot validates the soundness of these args, 
3. locates the appropriate docker image associated with them, 
4. and updates the `values.yaml` file for the specified application
5. A github [webhook](https://github.com/capco-ea/gitops-testing/settings/hooks/333359890) is configured to then send a payload with this update to the bot API, 
6. where the request is then inspected to confirm it came from the bot and not a human;

	a. we do this because Github doesn't provide the granularity to configure webhooks to send only under specific conditions, such as who the committer was, 

	b. so the hook will always fire, and we use the bot service to determine if the payload should be forwarded to Argo

	c. This also gives us a guarantee that Argo receives the update,

	d. and ultimately will only enact an auto-sync when told to do so through Slack.

7.  If the payload is confirmed to originate from Slack, it is forwarded to the Argo API
8.  The Argo API immediately sees that the desired state has changed and enters and OutOfSync state
9.  Another request is then sent to Sync the application
10. Sync status updates are returned to Slack in real time


#### Notes

The original intention was to only enable auto-sync for changes received via Github hook, and for it to be disabled
under all other conditions, but because this is not possible we just leave auto-sync disabled across the board and use 2 POST
requests to immediately invoke the desired change.  See steps 7 and 8 above.
