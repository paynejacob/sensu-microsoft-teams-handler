package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sensu/sensu-go/types"
	"github.com/spf13/cobra"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
)

var (
	webhookURL    string
	channel       string
	messagePrefix string
	iconURL       string
	actionName    string
	dashboard     string
	stdin         *os.File
)

type Section struct {
	Text string `json:"text"`
}

type Target struct {
	OS  string `json:"os"`
	URI string `json:"uri"`
}

type PotentialAction struct {
	Type string `json:"@type"`
	Name string `json:"name"`

	Targets []Target `json:"targets"`
}

type Message struct {
	ThemeColor string `json:"themeColor"`
	Text       string `json:"text"`
	Channel    string `json:"channel"`

	Sections []Section `json:"section"`

	PotentialAction []PotentialAction
}

func NewEventMessage(event *types.Event) *Message {
	message := &Message{ThemeColor: getColor(event), Text: getMessageStatus(event), Channel: channel} // TODO support channel from annotation
	message.Sections = append(message.Sections, Section{event.Check.Output})
	message.PotentialAction = append(message.PotentialAction, PotentialAction{Type: "OpenUri", Name: "View in Sensu"})
	message.PotentialAction[0].Targets = append(message.PotentialAction[0].Targets, Target{"default", getLink(event)})

	return message
}

func getLink(event *types.Event) string {
	var (
		dashboardUrl *url.URL
		eventPath    *url.URL
		err          error
	)

	if dashboardUrl, err = url.Parse(dashboard); err != nil {
		return ""
	}

	if eventPath, err = url.Parse(event.URIPath()); err != nil {
		return ""
	}

	return dashboardUrl.ResolveReference(eventPath).String()
}

func getColor(event *types.Event) string {
	switch event.Check.Status {
	case 0:
		return "#36A64F"
	case 1:
		return "#FFCC00"
	case 2:
		return "#FF0000"
	default:
		return "#6600CC"
	}
}

func getMessageStatus(event *types.Event) string {
	switch event.Check.Status {
	case 0:
		return "RESOLVED"
	case 1:
		return "WARNING"
	case 2:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

func sendMessage(event *types.Event) error {
	var message = NewEventMessage(event)
	var MessageString, _ = json.Marshal(message)

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(MessageString))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			panic(err)
		}
	}() // TODO: assert 200

	return nil
}

func configureRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sensu-microsoft-teams-handler",
		Short: "The Sensu Go Microsoft Teams handler for notifying a channel",
		RunE:  run,
	}

	cmd.Flags().StringVarP(&webhookURL,
		"webhook-url",
		"w",
		os.Getenv("MS_TEAMS_WEBHOOK_URL"),
		"The webhook url to send messages to")

	cmd.Flags().StringVarP(&channel,
		"channel",
		"c",
		"#general",
		"#notifications-room, optional defaults to webhook defined")

	cmd.Flags().StringVarP(&messagePrefix,
		"message-prefix",
		"p",
		"",
		"optional prefix - can be used for mentions")

	cmd.Flags().StringVarP(&iconURL,
		"icon-url",
		"i",
		"http://s3-us-west-2.amazonaws.com/sensuapp.org/sensu.png",
		"A URL to an image to use as the user avatar")

	cmd.Flags().StringVarP(&actionName,
		"action-name",
		"a",
		"View in Sensu",
		"The text that will be displayed on screen for the action")

	cmd.Flags().StringVarP(&dashboard,
		"dashboard",
		"d",
		"",
		"The url to the sensu dashboard")

	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		_ = cmd.Help()
		return errors.New("invalid argument(s) received")
	}

	if webhookURL == "" {
		_ = cmd.Help()
		return fmt.Errorf("webhook url is empty")

	}
	if stdin == nil {
		stdin = os.Stdin
	}

	eventJSON, err := ioutil.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %s", err.Error())
	}

	event := &types.Event{}
	err = json.Unmarshal(eventJSON, event)
	if err != nil {
		return fmt.Errorf("failed to unmarshal stdin data: %s", eventJSON)
	}

	if err := event.Entity.Validate(); err != nil {
		return err
	}

	if err := event.Check.Validate(); err != nil {
		return err
	}

	if err = sendMessage(event); err != nil {
		return errors.New(err.Error())
	}

	return nil
}

func main() {
	rootCmd := configureRootCommand()
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err.Error())
	}
}
