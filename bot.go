// Copyright (c) 2016 Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

// I am using the Golang driveR. Documentation can be found
// at https://godoc.org/github.com/mattermost/platform/model#Client

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
	DFLT_BOT_HOST      = "localhost:8065"
	DFLT_BOT_LOGIN     = "yakafokon"
	DFLT_BOT_FIRSTNAME = "Yakafokon"
	DFLT_BOT_LASTNAME  = "Bot"
	DFLT_BOT_TEAM      = "myteam"
	DFLT_CHANNEL_NAME  = "debugging-for-sample-bot"
)

type AnswerHandler func(*model.WebSocketEvent, *model.Post) string

type Entry struct {
	Expr   string
	RegExp *regexp.Regexp
	Answer string
	Hdler  AnswerHandler
}

type MattermostBot struct {
	host  string
	login string
	password string
	firstName string
	lastName string
	fullName string
	teamName string
	channelName string
	
	entries []*Entry

	client *model.Client
	webSocketClient *model.WebSocketClient
	
	mUser *model.User
	mTeam *model.Team
	initialLoad *model.InitialLoad
	debuggingChannel *model.Channel
}

func GetFromEnv(key string, dfltValue string) string {
	result := os.Getenv(key)
	if result == "" {
		result = dfltValue
	}
	return result;
}

func MattermostBotFromOsEnv() *MattermostBot {
	var bot MattermostBot
	bot.host = GetFromEnv("MATTERMOST_BOT_HOST", DFLT_BOT_HOST)
	bot.login = GetFromEnv("MATTERMOST_BOT_LOGIN", DFLT_BOT_LOGIN)
	bot.password = os.Getenv("MATTERMOST_BOT_PASSWORD")
	if bot.password == "" {
		println("It is mandatory to set MATTERMOST_BOT_PASSWORD environment variable.")
		return nil
	}
	bot.firstName = GetFromEnv("MATTERMOST_BOT_FIRSTNAME", DFLT_BOT_FIRSTNAME)
	bot.lastName = GetFromEnv("MATTERMOST_BOT_LASTNAME", DFLT_BOT_LASTNAME)
	bot.fullName = GetFromEnv("MATTERMOST_BOT_NAME", bot.firstName + bot.lastName)
	bot.teamName = GetFromEnv("MATTERMOST_BOT_TEAM", DFLT_BOT_TEAM)
	bot.channelName = GetFromEnv("MATTERMOST_BOT_CHANNEL", DFLT_CHANNEL_NAME)
	return &bot
}

func (bot *MattermostBot) initEntries() {
	bot.entries = []*Entry{
		&Entry{`(?:^|\W)list_entries(?:$|\W)`, nil, ".", bot.listEntriesHdler},
		&Entry{`(?:^|\W)entries_delete(?:$|\W)`, nil, ".", bot.delEntriesHdler},
		&Entry{`(?:^|\W)entries_add(?:$|\W)`, nil, ".", bot.addEntriesHdler},
		&Entry{`(?:^|\W)(` + bot.login + `)|(` + bot.firstName + `)|(` + bot.fullName + `).+(alive)|(vivant)|(mort)(?:$|\W)`, nil, "Yes I'm alive", nil},
		&Entry{`(?:^|\W)((H|h)ello)|((S|s)alut)|((B|b)onjour)|((B|b)onsoir)(?:$|\W)`, nil, "Bonjour", nil},
		&Entry{`(?:^|\W)perdu(?:$|\W)`, nil, "Êtes-vous perdu ? http://perdu.com/", nil},
		&Entry{`(?:^|\W)Il faudrait que(?:$|\W)`, nil, "Pourquoi pas ? Mais surtout pourquoi ne pas le [faire toi-même](http://yakafokon.detected.fr/) ?", nil},
		&Entry{`(?:^|\W)Tu devrais(?:$|\W)`, nil, "Ben tiens, ça tombe bien, j'avais que ça à faire.\nhttp://yakafokon.detected.fr/ ", nil},
		&Entry{`(?:^|\W)Il n'y a qu'à(?:$|\W)`, nil, "T'as raison, Mmmhh, tu t'en occupe ? \nhttp://yakafokon.detected.fr/", nil},
		&Entry{`(?:^|\W)Il faut qu'on(?:$|\W)`, nil, "C'est celui-qui dit [qui fait](http://yakafokon.detected.fr/) ?", nil},	
	}
	for _, e := range bot.entries {
		r, err := regexp.Compile(e.Expr)
		if err == nil {
			e.RegExp = r
		} else {
			println("We failed to compile ", e.Expr)
		}
	}
}

func (bot *MattermostBot) listEntriesHdler(event *model.WebSocketEvent, post *model.Post) string {
	ans := "I know " + fmt.Sprintf("%v", len(bot.entries)) + " rules\n\n"
	ans = ans + "|   id   |   RegExp   |   Answer   |\n"
	ans = ans + "| :----: |:----------:|:----------:|\n"
	for k, e := range bot.entries {
		ans = ans + "| " + fmt.Sprintf("%v", k) + " | " + strings.Replace(e.Expr, "|", "&#124;", -1) + " | " + e.Answer + " |\n"
	}
	return ans
}

func (bot *MattermostBot) delEntriesHdler(event *model.WebSocketEvent, post *model.Post) string {
	if bot.isTeamAdmin(event.UserId) {
		entryIdStr := strings.Split(post.Message, " ")[1]
		entryId, err := strconv.Atoi(entryIdStr)
		if err != nil {
			return "I do not understand which entry you wanted to delete: " + entryIdStr
		}
		e := bot.entries[entryId]
		if e.Hdler == nil {
			bot.entries = append(bot.entries[:entryId], bot.entries[entryId+1:]...)
			return "Done. I have deleted " + entryIdStr
		} else {
			return "Sorry, I cannot delete an internal command!"
		}

	} else {
		return "No never, you are not a team administrator."
	}
}

func (bot *MattermostBot) addEntriesHdler(event *model.WebSocketEvent, post *model.Post) string {
	if bot.isTeamAdmin(event.UserId) {
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
		bot.entries = append(bot.entries[:entryId], append([]*Entry{newEntry}, bot.entries[entryId:]...)...)
		return bot.listEntriesHdler(event, post)
	} else {
		return "No never, you are not a team administrator."
	}
}

func main() {

	var bot *MattermostBot = MattermostBotFromOsEnv()
	if bot == nil {
		return
	}
	bot.start()

	// You can block forever with
	select {}
}

func (bot *MattermostBot) start() {
	println(bot.fullName)
	bot.initEntries()

	bot.SetupGracefulShutdown()

	bot.client = model.NewClient("http://" + bot.host)

	// Lets test to see if the mattermost server is up and running
	bot.MakeSureServerIsRunning()

	// lets attempt to login to the Mattermost server as the bot user
	// This will set the token required for all future calls
	// You can get this token with client.AuthToken
	bot.LoginAsTheBotUser()

	// If the bot user doesn't have the correct information lets update his profile
	bot.UpdateTheBotUserIfNeeded()

	// Lets load all the stuff we might need
	bot.InitialLoad()

	// Lets find our bot team
	bot.FindBotTeam()

	// This is an important step.  Lets make sure we use the botTeam
	// for all future web service requests that require a team.
	bot.client.SetTeamId(bot.mTeam.Id)

	// Lets create a bot channel for logging debug messages into
	bot.CreateBotDebuggingChannelIfNeeded()
	bot.SendMsgToDebuggingChannel("_" + bot.fullName + " has **started** running_", "")

	// Lets start listening to some channels via the websocket!
	webSocketClient, err := model.NewWebSocketClient("ws://" + bot.host, bot.client.AuthToken)
	if err != nil {
		println("We failed to connect to the web socket")
		PrintError(err)
	}
	webSocketClient.Listen()

	go func() {
		for {
			select {
			case resp := <-webSocketClient.EventChannel:
				bot.HandleWebSocketResponse(resp)
			}
		}
	}()
}

func (bot *MattermostBot) MakeSureServerIsRunning() {
	if props, err := bot.client.GetPing(); err != nil {
		println("There was a problem pinging the Mattermost server.  Are you sure it's running?")
		PrintError(err)
		os.Exit(1)
	} else {
		println("Server detected and is running version " + props["version"])
	}
}

func (bot *MattermostBot) LoginAsTheBotUser() {
	if loginResult, err := bot.client.Login(bot.login, bot.password); err != nil {
		println("There was a problem logging into the Mattermost server.  Are you sure ran the setup steps from the README.md?")
		PrintError(err)
		os.Exit(1)
	} else {
		bot.mUser = loginResult.Data.(*model.User)
		println("I am user with id " + bot.mUser.Id)
	}
}

func (bot *MattermostBot) UpdateTheBotUserIfNeeded() {
	botUser := bot.mUser
	if botUser.FirstName != bot.firstName || botUser.LastName != bot.lastName || botUser.Username != bot.fullName {
		botUser.FirstName = bot.firstName
		botUser.LastName = bot.lastName
		botUser.Username = bot.fullName

		if updateUserResult, err := bot.client.UpdateUser(botUser); err != nil {
			println("We failed to update the Yakafokon Bot user")
			PrintError(err)
			os.Exit(1)
		} else {
			bot.mUser = updateUserResult.Data.(*model.User)
			println("Looks like this might be the first run so we've updated the bots account settings")
		}
	}
}

func (bot *MattermostBot) InitialLoad() {
	if initialLoadResults, err := bot.client.GetInitialLoad(); err != nil {
		println("We failed to get the initial load")
		PrintError(err)
		os.Exit(1)
	} else {
		bot.initialLoad = initialLoadResults.Data.(*model.InitialLoad)
	}
}

func (bot *MattermostBot) FindBotTeam() {
	for _, team := range bot.initialLoad.Teams {
		if team.Name == bot.teamName {
			bot.mTeam = team
			break
		}
	}

	if bot.mTeam == nil {
		println("We do not appear to be a member of the team '" + bot.teamName + "'")
		os.Exit(1)
	}
}

func (bot *MattermostBot) CreateBotDebuggingChannelIfNeeded() {
	if channelsResult, err := bot.client.GetChannels(""); err != nil {
		println("We failed to get the channels")
		PrintError(err)
	} else {
		channelList := channelsResult.Data.(*model.ChannelList)
		for _, channel := range channelList.Channels {

			// The logging channel has alredy been created, lets just use it
			println("chan name: " + channel.Name)
			if channel.Name == bot.channelName {
				bot.debuggingChannel = channel
				return
			}
		}
	}

	// Looks like we need to create the logging channel
	// TODO this will fails if the chan already exists but the bot is not member of it already.
	channel := &model.Channel{}
	channel.Name = bot.channelName
	channel.DisplayName = "Debugging For Sample Bot"
	channel.Purpose = "This is used as a test channel for logging bot debug messages"
	channel.Type = model.CHANNEL_OPEN
	if channelResult, err := bot.client.CreateChannel(channel); err != nil {
		println("We failed to create the channel " + channel.Name)
		PrintError(err)
	} else {
		bot.debuggingChannel = channelResult.Data.(*model.Channel)
		println("Looks like this might be the first run so we've created the channel " + channel.Name)
	}
}

func (bot *MattermostBot) SendMsgToDebuggingChannel(msg string, replyToId string) {
	post := &model.Post{}
	post.ChannelId = bot.debuggingChannel.Id
	post.Message = msg

	post.RootId = replyToId

	if _, err := bot.client.CreatePost(post); err != nil {
		println("We failed to send a message to the logging channel")
		PrintError(err)
	}
}

func (bot *MattermostBot) HandleWebSocketResponse(event *model.WebSocketEvent) {
	bot.HandleMsgFromDebuggingChannel(event)
}

func  (bot *MattermostBot) isTeamAdmin(userId string) bool {
	teamMembersAnswer, _ := bot.client.GetTeamMembers(bot.mTeam.Id)

	for _, member := range teamMembersAnswer.Data.([]*model.TeamMember) {
		if member.UserId == userId {
			return member.IsTeamAdmin()
		}
	}
	return false
}

func (bot *MattermostBot) HandleMsgFromDebuggingChannel(event *model.WebSocketEvent) {
	// If this isn't the debugging channel then lets ingore it
	if event.ChannelId != bot.debuggingChannel.Id {
		return
	}

	// Lets only reponded to messaged posted events
	if event.Event != model.WEBSOCKET_EVENT_POSTED {
		return
	}

	// Lets ignore if it's my own events just in case
	if event.UserId == bot.mUser.Id {
		return
	}

	println("responding to debugging channel msg")

	post := model.PostFromJson(strings.NewReader(event.Data["post"].(string)))

	if post != nil {
		for _, e := range bot.entries {
			if e.RegExp.MatchString(post.Message) {
				var answer string
				if e.Hdler == nil {
					answer = e.Answer
				} else {
					answer = e.Hdler(event, post)
				}
				bot.SendMsgToDebuggingChannel(answer, post.Id)
				return
			}
		}
	}

//	bot.SendMsgToDebuggingChannel("I did not understand you!", post.Id) 
}

func PrintError(err *model.AppError) {
	println("\tError Details:")
	println("\t\t" + err.Message)
	println("\t\t" + err.Id)
	println("\t\t" + err.DetailedError)
}

func (bot *MattermostBot) SetupGracefulShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			if bot.webSocketClient != nil {
				bot.webSocketClient.Close()
			}

			bot.SendMsgToDebuggingChannel("_" + bot.fullName + " has **stopped** running_", "")
			os.Exit(0)
		}
	}()
}
