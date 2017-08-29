// Copyright (c) 2016 Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package main

import (
	//"fmt"
	"os"
	"os/signal"
	"strings"
	"io/ioutil"
	 "regexp"
	"gopkg.in/yaml.v2"
	"github.com/mattermost/platform/model"
)

const (
	BOT_NAME = "Pillar Bot"
)

type Params struct {
	Email string `yaml: "email"`
	Password string `yaml: "password"`
	Username string `yaml: "username"`
	FirstName string `yaml: "firstname"`
	LastName string `yaml: "lastname"`
	Server string `yaml: "server"`
	DebugChannel string `yaml: "debugchannel"`
	Team string `yaml: "team"`
	Channel string `yaml: "channel"`
	Autoadd map[string][]string `yaml: "autoadd"`
	
}	

var params Params
var client *model.Client4
var webSocketClient *model.WebSocketClient

var botUser *model.User
var botTeam *model.Team
var currentTeam *model.Team
var debuggingChannel *model.Channel
var monitoredChannel *model.Channel
var allChannel *model.Channel

var  channelList []string 

// Documentation for the Go driver can be found
// at https://godoc.org/github.com/mattermost/platform/model#Client
func main() {
	println(BOT_NAME)

	SetupGracefulShutdown()

	LoadConfiguration();

	client = model.NewAPIv4Client("http://" + params.Server)

	// Lets test to see if the mattermost server is up and running
	MakeSureServerIsRunning()

	// lets attempt to login to the Mattermost server as the bot user
	// This will set the token required for all future calls
	// You can get this token with client.AuthToken
	LoginAsTheBotUser()

	// If the bot user doesn't have the correct information lets update his profile
	UpdateTheBotUserIfNeeded()

	// Lets find our bot team
	FindBotTeam()

	// This is an important step.  Lets make sure we use the botTeam
	// for all future web service requests that require a team.
	//client.SetTeamId(botTeam.Id)

	// Lets create a bot channel for logging debug messages into
	CreateBotDebuggingChannelIfNeeded()
	//SendMsgToDebuggingChannel("_"+BOT_NAME+" has **started** running_", "")

	println( "_"+BOT_NAME+" has **started** running_" + params.Server )

	JoinMonitoredChannel()

	// Lets start listening to some channels via the websocket!
	webSocketClient, err := model.NewWebSocketClient("ws://" + params.Server, client.AuthToken)
	if err != nil {
		println("We failed to connect to the web socket")
		PrintError(err)

		return
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

func LoadConfiguration() {
	source, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		panic(err)
	}
	err = yaml.Unmarshal(source, &params)
	if err != nil {
		panic(err)
	}
}

func MakeSureServerIsRunning() {
	if props, resp := client.GetOldClientConfig(""); resp.Error != nil {
		println("There was a problem pinging the Mattermost server.  Are you sure it's running?")
		PrintError(resp.Error)
		os.Exit(1)
	} else {
		println("Server detected and is running version " + props["Version"])
	}
}

func LoginAsTheBotUser() {
	if user, resp := client.Login(params.Email, params.Password); resp.Error != nil {
		println("There was a problem logging into the Mattermost server.  Are you sure ran the setup steps from the README.md?")
		PrintError(resp.Error)
		os.Exit(1)
	} else {
		botUser = user
	}
}

func UpdateTheBotUserIfNeeded() {
	if botUser.FirstName != params.FirstName || botUser.LastName != params.LastName || botUser.Username != params.Username {
		botUser.FirstName = params.FirstName
		botUser.LastName = params.LastName
		botUser.Username = params.Username

		if user, resp := client.UpdateUser(botUser); resp.Error != nil {
			println("We failed to update the Sample Bot user")
			PrintError(resp.Error)
			os.Exit(1)
		} else {
			botUser = user
			println("Looks like this might be the first run so we've updated the bots account settings")
		}
	}
}

func FindBotTeam() {
	if team, resp := client.GetTeamByName(params.Team, ""); resp.Error != nil {
		println("We failed to get the initial load")
		println("or we do not appear to be a member of the team '" + params.Team + "'")
		PrintError(resp.Error)
		os.Exit(1)
	} else {
		botTeam = team
	}
}

func CreateBotDebuggingChannelIfNeeded() {
	if rchannel, resp := client.GetChannelByName(params.DebugChannel, botTeam.Id, ""); resp.Error != nil {
		println("We failed to get the channels")
		PrintError(resp.Error)
	} else {
		debuggingChannel = rchannel
		return
	}

	// Looks like we need to create the logging channel
	channel := &model.Channel{}
	channel.Name = params.DebugChannel
	channel.DisplayName = "Debugging For Sample Bot"
	channel.Purpose = "This is used as a test channel for logging bot debug messages"
	channel.Type = model.CHANNEL_OPEN
	channel.TeamId = botTeam.Id
	if rchannel, resp := client.CreateChannel(channel); resp.Error != nil {
		println("We failed to create the channel " + params.DebugChannel)
		PrintError(resp.Error)
	} else {
		debuggingChannel = rchannel
		println("Looks like this might be the first run so we've created the channel " + params.DebugChannel)
	}
}

func JoinMonitoredChannel() {
	if rchannel, resp := client.GetChannelByName(params.Channel, botTeam.Id, ""); resp.Error != nil {
		println("We failed to get the channels")
		PrintError(resp.Error)
	} else {
		monitoredChannel = rchannel
		return
	}

	// TODO: join the channel if failed
}

func SendMsgToDebuggingChannel(msg string, replyToId string) {
	post := &model.Post{}
	post.ChannelId = debuggingChannel.Id
	post.Message = msg

	post.RootId = replyToId

	if _, resp := client.CreatePost(post); resp.Error != nil {
		println("We failed to send a message to the logging channel")
		PrintError(resp.Error)
	}
}


// delete message added by Bot
func deleteBotPostMessage( post_id string ) {

	

	if _, resp := client.DeletePost(post_id); resp.Error != nil {
		println("post unable to delete")
		PrintError(resp.Error)
	}else{
		println("bot post  deleted")
	}
	
}

func HandleWebSocketResponse(event *model.WebSocketEvent) {

	HandleMsgFromMonitoredChannel(event)
}

func HandleMsgFromMonitoredChannel(event *model.WebSocketEvent) {

		

				// monitor event for new users
    		if event.Event ==  model.WEBSOCKET_EVENT_NEW_USER{
    			println("event" + event.Data["user_id"].(string))

         		HandleNewUserOrExistingUserAdding(event.Data["user_id"].(string))
         		}
	
         
         // if its post event
		if event.Event == model.WEBSOCKET_EVENT_POSTED {

			
				 if post != nil {

				 	
					post := model.PostFromJson(strings.NewReader(event.Data["post"].(string))

		 	 // if the User leave  channel and join back
					if post.Type == model.POST_JOIN_CHANNEL {
					

						// get the current user that joined this channel
						joinedUserName := post.Props["username"].(string)

						user, resp := client.GetUserByUsername(joinedUserName, "")
						HandleNewUserOrExistingUserAdding(user.Id)	

						if resp.Error != nil {
							println(" error getting user " + joinedUserName)
							
							}

							
		 				  }
					
					

		 				

		 					// if you see any word matching 'add existing users' then respond
					
			//     println("Message" + post.Message)

			//  if existingUsers ,resp  := client.GetUsersInChannel(debuggingChannel.Id,0 ,100, "");

			//   resp != nil{
			//  	for _,existingUser := range existingUsers{
			 		
			//  		HandleNewUserOrExistingUserAdding(existingUser.Id)
			//  	}
			//  	// delete the   " add existing user" message 
			//  	deleteBotPostMessage(post.Id)
			//  }
			// return
			// }

			


					

		}


		 
		
		}// / end  post event

		 

      				
	
}

func AddUserToTeam(user string, team_id string, team_name string, channels []string, tr *model.Team) {
	_, resp  := client.AddTeamMember( team_id, user);
	if resp.Error != nil {
		// SendMsgToDebuggingChannel("Could not add user to team!", "")
		println("Could not add user to team!" + team_id)

		return
	}

	for _, channel_to_join := range channels {
		rchannel, resp1 := client.GetChannelByName(channel_to_join, team_id, "");
		if resp1.Error != nil {
			// SendMsgToDebuggingChannel("Could not get channel by name: " + channel_to_join, "")

			continue
		}

		_, err := AddUserToChannel(rchannel.Id , user , "member")
		if err != nil {
			//SendMsgToDebuggingChannel("Could not join channel: " + channel_to_join, "")

			println("Could not join channel: " + channel_to_join)
			PrintError(err)
		}
	}
}


func HandleNewUserOrExistingUserAdding( user_id string) {


	  println("add to all channels" + user_id)

	for k, v := range params.Autoadd {

				if team, resp := client.GetTeamByName(k, ""); resp.Error == nil {


						// if its the pillar team, add user to channnel too
                         if(k == "pillarteam"){
					
						 println("add to all channels" + k)



					if allChannel , err := client.GetPublicChannelsForTeam(team.Id,0,100 , ""); err.Error == nil{
					 	
                         channelList := make([]string, len(allChannel))

						 for i, channelInTeam := range allChannel{


						 isChannelAvailable :=in_array(channelInTeam.Name, v)

						 	 if !isChannelAvailable{
 							
 								channelList[i] = channelInTeam.Name

 								}
 								
 								}
 							AddUserToTeam(user_id, team.Id, k, channelList, team)
 							}

						 	 	

						}else{
					
					  AddUserToTeam(user_id, team.Id, k, v, team)
					}
						
					
				} else {
					//SendMsgToDebuggingChannel(" error getting team " + k, "")
					println( " error getting team " + k, "")

					PrintError(resp.Error)
				}

			}
	
}

// https://api.mattermost.com/#tag/channels%2Fpaths%2F~1channels~1%7Bchannel_id%7D~1members%2Fpost
func AddUserToChannel(channel_id string, user_id string, roles string) (*model.Result, *model.AppError) {
	//request := fmt.Sprintf(`{
	//	"channel_id": %s,
	//	"user_id": %s,
	//	"roles": %s,
	//	}`, channel_id, user_id, roles)

	member := model.ChannelMember{ChannelId: channel_id, UserId: user_id}
	request := member.ToJson()

	if r, err := client.DoApiPost("/channels/" + channel_id + "/members", request); err != nil {
		return nil, err
	} else {
		//defer model.closeBody(r)
		return &model.Result{r.Header.Get(model.HEADER_REQUEST_ID),
			r.Header.Get(model.HEADER_ETAG_SERVER), model.TeamFromJson(r.Body)}, nil
	}
}

func PrintError(err *model.AppError) {
	println("\tError Details:")
	println("\t\t" + err.Message)
	println("\t\t" + err.Id)
	println("\t\t" + err.DetailedError)
}

// array to check if exist

func in_array(val string, array []string) (exists bool) {
    exists = false
    //index = -1;

    for _, v := range array {
        if val == v {
            //index = i
            exists = true
            return
        }   
    }

    return
}

func SetupGracefulShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			if webSocketClient != nil {
				webSocketClient.Close()
			}

			//SendMsgToDebuggingChannel("_"+BOT_NAME+" has **stopped** running_", "")
			os.Exit(0)
		}
	}()
}
