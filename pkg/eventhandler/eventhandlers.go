package eventhandler

import (
	"didiladi/keptn-generic-job-service/pkg/config"
	"didiladi/keptn-generic-job-service/pkg/k8s"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	cloudevents "github.com/cloudevents/sdk-go/v2" // make sure to use v2 cloudevents here
	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"
)

/**
* Here are all the handler functions for the individual event
* See https://github.com/keptn/spec/blob/0.8.0-alpha/cloudevents.md for details on the payload
**/

type EventHandler struct {
	Keptn                                        *keptnv2.Keptn
	Event                                        cloudevents.Event
	EventData                                    *keptnv2.EventData
	ServiceName                                  string
	JobNamespace                                 string
	InitContainerConfigurationServiceAPIEndpoint string
	KeptnAPIToken                                string
}

// HandleEvent handles all events in a generic manner
func (eh *EventHandler) HandleEvent() error {

	var eventDataAsInterface interface{}
	err := json.Unmarshal(eh.Event.Data(), &eventDataAsInterface)
	if err != nil {
		log.Printf("failed to convert incoming cloudevent: %v", err)
		return err
	}

	log.Printf("Attempting to handle event %s of type %s ...", eh.Event.Context.GetID(), eh.Event.Type())
	log.Printf("CloudEvent %T: %v", eventDataAsInterface, eventDataAsInterface)

	resource, err := eh.Keptn.GetKeptnResource("generic-job/config.yaml")
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

	match, action := configuration.IsEventMatch(eh.Event.Type(), eventDataAsInterface)
	if !match {
		log.Printf("No match found for event %s of type %s. Skipping...", eh.Event.Context.GetID(), eh.Event.Type())
		return nil
	}

	log.Printf("Match found for event %s of type %s. Starting k8s job to run action '%s'", eh.Event.Context.GetID(), eh.Event.Type(), action.Name)

	eh.startK8sJob(action)

	return nil
}

func (eh *EventHandler) startK8sJob(action *config.Action) {

	event, err := eh.Keptn.SendTaskStartedEvent(eh.EventData, eh.ServiceName)
	if err != nil {
		log.Printf("Error while sending started event: %s\n", err.Error())
		return
	}

	logs := ""

	for index, task := range action.Tasks {
		log.Printf("Starting task %s/%s: '%s' ...", strconv.Itoa(index+1), strconv.Itoa(len(action.Tasks)), task.Name)

		jobName := "keptn-generic-job-" + event + "-" + strconv.Itoa(index+1)

		clientset, err := k8s.ConnectToCluster(eh.JobNamespace)
		if err != nil {
			log.Printf("Error while connecting to cluster: %s\n", err.Error())
			sendTaskFailedEvent(eh.Keptn, jobName, eh.ServiceName, err, "")
			return
		}

		jobErr := k8s.CreateK8sJob(clientset, eh.JobNamespace, jobName, action, task, eh.EventData, eh.InitContainerConfigurationServiceAPIEndpoint, eh.KeptnAPIToken)
		defer func() {
			err = k8s.DeleteK8sJob(clientset, eh.JobNamespace, jobName)
			if err != nil {
				log.Printf("Error while deleting job: %s\n", err.Error())
			}
		}()

		logs, err = k8s.GetLogsOfPod(clientset, eh.JobNamespace, jobName)
		if err != nil {
			log.Printf("Error while retrieving logs: %s\n", err.Error())
		}

		if jobErr != nil {
			log.Printf("Error while creating job: %s\n", err.Error())
			sendTaskFailedEvent(eh.Keptn, jobName, eh.ServiceName, err, logs)
			return
		}
	}

	log.Printf("Successfully finished processing of event: %s\n", eh.Keptn.CloudEvent.ID())

	eh.Keptn.SendTaskFinishedEvent(&keptnv2.EventData{
		Status:  keptnv2.StatusSucceeded,
		Result:  keptnv2.ResultPass,
		Message: fmt.Sprintf("Job %s finished successfully!\n\nLogs:\n%s", "keptn-generic-job-"+event, logs),
	}, eh.ServiceName)
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
