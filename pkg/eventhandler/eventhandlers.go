package eventhandler

import (
	"didiladi/keptn-generic-job-service/pkg/config"
	"didiladi/keptn-generic-job-service/pkg/k8s"
	"fmt"
	"log"
	"os"
	"strconv"

	cloudevents "github.com/cloudevents/sdk-go/v2" // make sure to use v2 cloudevents here
	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"
)

/**
* Here are all the handler functions for the individual event
* See https://github.com/keptn/spec/blob/0.8.0-alpha/cloudevents.md for details on the payload
**/

// HandleEvent handles all events in a generic manner
func HandleEvent(myKeptn *keptnv2.Keptn, incomingEvent cloudevents.Event, data interface{}, eventData *keptnv2.EventData, serviceName string) error {

	log.Printf("Attempting to handle event %s of type %s ...", incomingEvent.Context.GetID(), incomingEvent.Type())
	log.Printf("CloudEvent %T: %v", data, data)

	resource, err := myKeptn.GetKeptnResource("generic-job/config.yaml")
	if err != nil {
		log.Printf("Could not find config for generic Job service: %s", err.Error())
		return err
	}

	configuration, err := config.NewConfig(resource)
	if err != nil {
		log.Printf("Could not parse config: %s", err)
		log.Printf("The config was: %s", string(resource))
		return err
	}

	match, action := configuration.IsEventMatch(incomingEvent.Type(), data)
	if !match {
		log.Printf("No match found for event %s of type %s. Skipping...", incomingEvent.Context.GetID(), incomingEvent.Type())
		return nil
	}

	log.Printf("Match found for event %s of type %s. Starting k8s job to run action '%s'", incomingEvent.Context.GetID(), incomingEvent.Type(), action.Name)

	startK8sJob(myKeptn, eventData, action, serviceName)

	return nil
}

func startK8sJob(myKeptn *keptnv2.Keptn, eventData *keptnv2.EventData, action *config.Action, serviceName string) {

	event, err := myKeptn.SendTaskStartedEvent(eventData, serviceName)
	if err != nil {
		log.Printf("Error while sending started event: %s\n", err.Error())
		return
	}

	namespace, _ := os.LookupEnv("JOB_NAMESPACE")
	configServiceApiUrl, _ := os.LookupEnv("KEPTN_CONFIGURATION_SERVICE_API_ENDPOINT")
	configServiceToken, _ := os.LookupEnv("KEPTN_API_TOKEN")
	logs := ""

	for index, task := range action.Tasks {
		log.Printf("Starting task %s/%s: '%s' ...", strconv.Itoa(index+1), strconv.Itoa(len(action.Tasks)), task.Name)

		jobName := "keptn-generic-job-" + event + "-" + strconv.Itoa(index+1)

		clientset, err := k8s.ConnectToCluster(namespace)
		if err != nil {
			log.Printf("Error while connecting to cluster: %s\n", err.Error())
			sendTaskFailedEvent(myKeptn, jobName, serviceName, err, "")
			return
		}

		jobErr := k8s.CreateK8sJob(clientset, namespace, jobName, action, task, eventData, configServiceApiUrl, configServiceToken)
		defer func() {
			err = k8s.DeleteK8sJob(clientset, namespace, jobName)
			if err != nil {
				log.Printf("Error while deleting job: %s\n", err.Error())
			}
		}()

		logs, err = k8s.GetLogsOfPod(clientset, namespace, jobName)
		if err != nil {
			log.Printf("Error while retrieving logs: %s\n", err.Error())
		}

		if jobErr != nil {
			log.Printf("Error while creating job: %s\n", err.Error())
			sendTaskFailedEvent(myKeptn, jobName, serviceName, err, logs)
			return
		}
	}

	log.Printf("Successfully finished processing of event: %s\n", myKeptn.CloudEvent.ID())

	myKeptn.SendTaskFinishedEvent(&keptnv2.EventData{
		Status:  keptnv2.StatusSucceeded,
		Result:  keptnv2.ResultPass,
		Message: fmt.Sprintf("Job %s finished successfully!\n\nLogs:\n%s", "keptn-generic-job-"+event, logs),
	}, serviceName)
}

func sendTaskFailedEvent(myKeptn *keptnv2.Keptn, jobName string, serviceName string, err error, logs string) {

	_, err = myKeptn.SendTaskFinishedEvent(&keptnv2.EventData{
		Status:  keptnv2.StatusErrored,
		Result:  keptnv2.ResultFailed,
		Message: fmt.Sprintf("Job %s failed: %s\n\nLogs: \n%s", jobName, err.Error(), logs),
	}, serviceName)

	if err != nil {
		log.Printf("Error while sending started event: %s\n", err.Error())
	}
}
