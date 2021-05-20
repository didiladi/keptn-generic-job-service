package config

import (
	"gopkg.in/yaml.v2"

	"github.com/PaesslerAG/jsonpath"
)

type Config struct {
	Actions []Action `yaml:"actions"`
}

type Action struct {
	Name     string   `yaml:"name"`
	Event    string   `yaml:"event"`
	JSONPath JSONPath `yaml:"jsonpath"`
	Tasks    []Task   `yaml:"tasks"`
}

type JSONPath struct {
	Property string `yaml:"property"`
	Match    string `yaml:"match"`
}

type Task struct {
	Name  string   `yaml:"name"`
	Files []string `yaml:"files"`
	Image string   `yaml:"image"`
	Cmd   string   `yaml:"cmd"`
}

func NewConfig(yamlContent []byte) (*Config, error) {

	config := Config{}
	err := yaml.Unmarshal(yamlContent, &config)

	return &config, err
}

func (c *Config) IsEventMatch(event string, jsonEventData interface{}) (bool, *Action) {

	for _, action := range c.Actions {

		// does the event type match?
		if action.Event == event {

			value, err := jsonpath.Get(action.JSONPath.Property, jsonEventData)
			if err != nil {
				continue
			}

			if value == action.JSONPath.Match {
				return true, &action
			}
		}
	}
	return false, nil
}

func (c *Config) FindActionByName(actionName string) (bool, *Action) {

	for _, action := range c.Actions {
		if actionName == action.Name {
			return true, &action
		}
	}
	return false, nil
}

func (a *Action) FindTaskByName(taskName string) (bool, *Task) {

	for _, task := range a.Tasks {
		if taskName == task.Name {
			return true, &task
		}
	}
	return false, nil
}
