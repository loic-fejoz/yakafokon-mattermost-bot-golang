// Copyright (c) 2016 Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package main

import (
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"

	"github.com/loic-fejoz/platform/model"
)

const (
	BOT_FIRSTNAME    = "Yakafokon"
	BOT_LASTNAME     = "Bot"
	BOT_NAME = BOT_FIRSTNAME + " " + BOT_LASTNAME
	TEAM_NAME        = "myteam"
	CHANNEL_LOG_NAME = "debugging-for-sample-bot"
	BOT_EMAIL    = "yakafokon@fejoz.net"	
	USER_PASSWORD = "password1"
)

type AnswerHandler func(*model.WebSocketEvent, *model.Post) string

type Entry struct {
	Expr   string
	RegExp *regexp.Regexp
	Answer string
	Hdler  AnswerHandler
}

var client *model.Client
var webSocketClient *model.WebSocketClient

var botUser *model.User
var botTeam *model.Team
var initialLoad *model.InitialLoad
var debuggingChannel *model.Channel
var entries []*Entry

func listEntriesHdler(event *model.WebSocketEvent, post *model.Post) string {
	ans := "I know " + fmt.Sprintf("%v", len(entries)) + " rules\n\n"
	ans = ans + "|   id   |   RegExp   |   Answer   |\n"
	ans = ans + "| :----: |:----------:|:----------:|\n"
	for k, e := range entries {
		ans = ans + "| " + fmt.Sprintf("%v", k) + " | " + strings.Replace(e.Expr, "|", "&#124;", -1) + " | " + e.Answer + " |\n"
	}
	return ans
}

func delEntriesHdler(event *model.WebSocketEvent, post *model.Post) string {
	if isTeamAdmin(event.UserId) {
		entryIdStr := strings.Split(post.Message, " ")[1]
		entryId, err := strconv.Atoi(entryIdStr)
		if err != nil {
			return "I do not understand which entry you wanted to delete: " + entryIdStr
		}
		e := entries[entryId]
		if e.Hdler == nil {
			entries = append(entries[:entryId], entries[entryId+1:]...)
			return "Done. I have deleted " + entryIdStr
		} else {
			return "Sorry, I cannot delete an internal command!"
		}

	} else {
		return "No never, you are not a team administrator."
	}
}

func addEntriesHdler(event *model.WebSocketEvent, post *model.Post) string {
	if isTeamAdmin(event.UserId) {
		pattern := `entries_add\s+([0-9]+)\s+When\s+(.+)\s+answer(.+)`
		re := regexp.MustCompile(pattern)
		parts := re.FindStringSubmatch(post.Message)
		if parts == nil {
			return "No comprendo. Must be something along: " + pattern
		}
		entryId, err := strconv.Atoi(parts[1])
		if err != nil {
			return "I do not understand which index you wanted to insert: " + parts[1]
		}
		newEntry := &Entry{parts[2], nil, parts[3], nil}
		r, err := regexp.Compile(newEntry.Expr)
		if err != nil {
			return "This is not a valid regular expression: " + newEntry.Expr
		}
		newEntry.RegExp = r
		entries = append(entries[:entryId], append([]*Entry{newEntry}, entries[entryId:]...)...)
		return listEntriesHdler(event, post)
	} else {
		return "No never, you are not a team administrator."
	}
}

func initEntries() {
	entries = []*Entry{
		&Entry{`(?:^|\W)list_entries(?:$|\W)`, nil, ".", listEntriesHdler},
		&Entry{`(?:^|\W)entries_delete(?:$|\W)`, nil, ".", delEntriesHdler},
		&Entry{`(?:^|\W)entries_add(?:$|\W)`, nil, ".", addEntriesHdler},
		&Entry{`(?:^|\W)bot_alive(?:$|\W)`, nil, "Yes I'm alive", nil},
		&Entry{`(?:^|\W)up(?:$|\W)`, nil, "Yes I'm up", nil},
		&Entry{`(?:^|\W)running(?:$|\W)`, nil, "Yes I'm running", nil},
		&Entry{`(?:^|\W)hello(?:$|\W)`, nil, "Hi", nil},
		&Entry{`(?:^|\W)perdu(?:$|\W)`, nil, "Êtes-vous perdu ? http://perdu.com/", nil},
	}
	for _, e := range entries {
		r, err := regexp.Compile(e.Expr)
		if err == nil {
			e.RegExp = r
		} else {
			println("We failed to compile ", e.Expr)
		}
	}
}

// Documentation for the Go driver can be found
// at https://godoc.org/github.com/mattermost/platform/model#Client
func main() {
	println(BOT_NAME)
	initEntries()

	SetupGracefulShutdown()

	client = model.NewClient("http://localhost:8065")

	// Lets test to see if the mattermost server is up and running
	MakeSureServerIsRunning()

	// lets attempt to login to the Mattermost server as the bot user
	// This will set the token required for all future calls
	// You can get this token with client.AuthToken
	LoginAsTheBotUser()

	// If the bot user doesn't have the correct information lets update his profile
	UpdateTheBotUserIfNeeded()

	// Lets load all the stuff we might need
	InitialLoad()

	// Lets find our bot team
	FindBotTeam()

	// This is an important step.  Lets make sure we use the botTeam
	// for all future web service requests that require a team.
	client.SetTeamId(botTeam.Id)

	// Lets create a bot channel for logging debug messages into
	CreateBotDebuggingChannelIfNeeded()
	SendMsgToDebuggingChannel("_"+BOT_NAME+" has **started** running_", "")

	// Lets start listening to some channels via the websocket!
	webSocketClient, err := model.NewWebSocketClient("ws://localhost:8065", client.AuthToken)
	if err != nil {
		println("We failed to connect to the web socket")
		PrintError(err)
	}

	webSocketClient.Listen()

	go func() {
		for {
			select {
			case resp := <-webSocketClient.EventChannel:
				HandleWebSocketResponse(resp)
			}
		}
	}()

	// You can block forever with
	select {}
}

func MakeSureServerIsRunning() {
	if props, err := client.GetPing(); err != nil {
		println("There was a problem pinging the Mattermost server.  Are you sure it's running?")
		PrintError(err)
		os.Exit(1)
	} else {
		println("Server detected and is running version " + props["version"])
	}
}

func LoginAsTheBotUser() {
	if loginResult, err := client.Login(BOT_EMAIL, USER_PASSWORD); err != nil {
		println("There was a problem logging into the Mattermost server.  Are you sure ran the setup steps from the README.md?")
		PrintError(err)
		os.Exit(1)
	} else {
		botUser = loginResult.Data.(*model.User)
		println("I am user with id " + botUser.Id)
	}
}

func UpdateTheBotUserIfNeeded() {
	if botUser.FirstName != BOT_FIRSTNAME || botUser.LastName != BOT_LASTNAME || botUser.Username != BOT_NAME {
		botUser.FirstName = BOT_FIRSTNAME
		botUser.LastName = BOT_LASTNAME
		botUser.Username = BOT_NAME

		if updateUserResult, err := client.UpdateUser(botUser); err != nil {
			println("We failed to update the Yakafokon Bot user")
			PrintError(err)
			os.Exit(1)
		} else {
			botUser = updateUserResult.Data.(*model.User)
			println("Looks like this might be the first run so we've updated the bots account settings")
		}
	}
}

func InitialLoad() {
	if initialLoadResults, err := client.GetInitialLoad(); err != nil {
		println("We failed to get the initial load")
		PrintError(err)
		os.Exit(1)
	} else {
		initialLoad = initialLoadResults.Data.(*model.InitialLoad)
	}
}

func FindBotTeam() {
	for _, team := range initialLoad.Teams {
		if team.Name == TEAM_NAME {
			botTeam = team
			break
		}
	}

	if botTeam == nil {
		println("We do not appear to be a member of the team '" + TEAM_NAME + "'")
		os.Exit(1)
	}
}

func CreateBotDebuggingChannelIfNeeded() {
	if channelsResult, err := client.GetChannels(""); err != nil {
		println("We failed to get the channels")
		PrintError(err)
	} else {
		channelList := channelsResult.Data.(*model.ChannelList)
		for _, channel := range channelList.Channels {

			// The logging channel has alredy been created, lets just use it
			if channel.Name == CHANNEL_LOG_NAME {
				debuggingChannel = channel
				return
			}
		}
	}

	// Looks like we need to create the logging channel
	channel := &model.Channel{}
	channel.Name = CHANNEL_LOG_NAME
	channel.DisplayName = "Debugging For Sample Bot"
	channel.Purpose = "This is used as a test channel for logging bot debug messages"
	channel.Type = model.CHANNEL_OPEN
	if channelResult, err := client.CreateChannel(channel); err != nil {
		println("We failed to create the channel " + CHANNEL_LOG_NAME)
		PrintError(err)
	} else {
		debuggingChannel = channelResult.Data.(*model.Channel)
		println("Looks like this might be the first run so we've created the channel " + CHANNEL_LOG_NAME)
	}
}

func SendMsgToDebuggingChannel(msg string, replyToId string) {
	post := &model.Post{}
	post.ChannelId = debuggingChannel.Id
	post.Message = msg

	post.RootId = replyToId

	if _, err := client.CreatePost(post); err != nil {
		println("We failed to send a message to the logging channel")
		PrintError(err)
	}
}

func HandleWebSocketResponse(event *model.WebSocketEvent) {
	HandleMsgFromDebuggingChannel(event)
}

func isTeamAdmin(userId string) bool {
	teamMembersAnswer, _ := client.GetTeamMembers(botTeam.Id)

	for _, member := range teamMembersAnswer.Data.([]*model.TeamMember) {
		if member.UserId == userId {
			return member.IsTeamAdmin()
		}
	}
	return false
}

func HandleMsgFromDebuggingChannel(event *model.WebSocketEvent) {
	// If this isn't the debugging channel then lets ingore it
	if event.ChannelId != debuggingChannel.Id {
		return
	}

	// Lets only reponded to messaged posted events
	if event.Event != model.WEBSOCKET_EVENT_POSTED {
		return
	}

	// Lets ignore if it's my own events just in case
	if event.UserId == botUser.Id {
		return
	}

	println("responding to debugging channel msg")

	post := model.PostFromJson(strings.NewReader(event.Data["post"].(string)))

	if post != nil {
		for _, e := range entries {
			if e.RegExp.MatchString(post.Message) {
				var answer string
				if e.Hdler == nil {
					answer = e.Answer
				} else {
					answer = e.Hdler(event, post)
				}
				SendMsgToDebuggingChannel(answer, post.Id)
				return
			}
		}
	}

	SendMsgToDebuggingChannel("I did not understand you!", post.Id)
}

func PrintError(err *model.AppError) {
	println("\tError Details:")
	println("\t\t" + err.Message)
	println("\t\t" + err.Id)
	println("\t\t" + err.DetailedError)
}

func SetupGracefulShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			if webSocketClient != nil {
				webSocketClient.Close()
			}

			SendMsgToDebuggingChannel("_"+BOT_NAME+" has **stopped** running_", "")
			os.Exit(0)
		}
	}()
}
