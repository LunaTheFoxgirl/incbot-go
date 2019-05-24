package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	CmdNotFoundError string = "Command %s not found!"
)

type Command func(CmdContext) (error, string)

type Config struct {
	Token       string   `json:"token"`
	BotChannels []string `json:"botchannels"`
	CmdPrefix   string   `json:"prefix"`
	Roles       struct {
		Admins  []string `json:"admins"`
		Allowed []string `json:"allowed"`
	} `json:"roles"`
}

func (cfg *Config) Save() {
	jstr, err := json.Marshal(cfg)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = ioutil.WriteFile("config.json", jstr, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
}

type CmdContext struct {
	params     []string
	session    *discordgo.Session
	msgcontext *discordgo.MessageCreate
	origin     *discordgo.Member
}

var CmdMap map[string]Command = map[string]Command{
	// Role
	"role":      cmdRoles,
	"pronoun":   cmdRoles,
	"neuro":     cmdRoles,
	"addrole":   cmdAdd,
	"help":      cmdHelp,
	"listroles": cmdList,
}

var CONFIG Config

var discord *discordgo.Session

func main() {
	// default value
	CONFIG.CmdPrefix = "!"

	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println(err)
		return
	}

	err = json.Unmarshal(file, &CONFIG)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(CONFIG)

	discord, err = discordgo.New("Bot " + CONFIG.Token)
	if err != nil {
		fmt.Println(err)
		return
	}

	discord.AddHandler(onCommand)
	discord.AddHandler(func(session *discordgo.Session, message *discordgo.Ready) {
		session.UpdateStatus(0, "Say !help for help")
	})

	err = discord.Open()
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Bot is now running, press Ctrl+C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	discord.Close()
}

func onCommand(session *discordgo.Session, message *discordgo.MessageCreate) {
	meUser, err := discord.User("@me")
	if err != nil {
		fmt.Println(err)
		return
	}

	if message.Author.ID != meUser.ID {
		member, err := session.GuildMember(message.GuildID, message.Author.ID)
		if err != nil {
			fmt.Println(err)
			return
		}
		if strings.HasPrefix(message.Content, CONFIG.CmdPrefix) {

			if !canFind(message.ChannelID, CONFIG.BotChannels) {
				session.MessageReactionAdd(message.ChannelID, message.ID, "❌")
				time.Sleep(time.Second * 2)
				session.ChannelMessageDelete(message.ChannelID, message.ID)
				return
			}
			cmd := getCommand(message.Content)
			var params []string = make([]string, 0)
			if len(cmd)+2 < len(message.Content) {
				params = getParams(message.Content[len(cmd)+2:])
			}

			if _, ok := CmdMap[strings.ToLower(cmd)]; ok {
				ctx := CmdContext{
					params:     params,
					session:    session,
					msgcontext: message,
					origin:     member,
				}
				err, output := CmdMap[strings.ToLower(cmd)](ctx)
				if err != nil {
					session.ChannelMessageSend(message.ChannelID, "⚠️ `"+err.Error()+"`")
					session.MessageReactionAdd(message.ChannelID, message.ID, "❌")
					return
				}
				if output != "" {
					session.ChannelMessageSend(message.ChannelID, output)
				}
				session.MessageReactionAdd(message.ChannelID, message.ID, "✅")
				return
			}
			session.ChannelMessageSend(message.ChannelID, fmt.Sprintf("⚠️ `"+CmdNotFoundError+"`", cmd))
			session.MessageReactionAdd(message.ChannelID, message.ID, "❌")
			return
		}
		return
	}
}

func getCommand(input string) string {
	split := strings.Split(input, " ")
	return split[0][len(CONFIG.CmdPrefix):]
}

func getParams(input string) []string {
	return strings.Split(input, " ")
}

func canFind(searchItem string, arr []string) bool {
	for _, n := range arr {
		if searchItem == n {
			return true
		}
	}
	return false
}

func canFindAny(searchItems []string, arr []string) bool {
	for _, n := range arr {
		for _, searchItem := range searchItems {
			if searchItem == n {
				return true
			}
		}
	}
	return false
}

func findRoleByName(searchItem string, arr []*discordgo.Role) *discordgo.Role {
	for _, n := range arr {
		if searchItem == n.Name {
			return n
		}
	}
	return nil
}

func findRoleById(searchItem string, arr []*discordgo.Role) *discordgo.Role {
	for _, n := range arr {
		if searchItem == n.ID {
			return n
		}
	}
	return nil
}

func cmdRoles(context CmdContext) (error, string) {
	guild, err := context.session.Guild(context.msgcontext.GuildID)
	if err != nil {
		fmt.Println("Could not find guild")
		return err, ""
	}
	roles := guild.Roles

	for _, arg := range context.params {
		role := findRoleByName(arg, roles)
		if role != nil {
			if canFind(role.ID, CONFIG.Roles.Allowed) {
				origin := context.origin
				if !canFind(role.ID, origin.Roles) {
					context.session.GuildMemberRoleAdd(context.msgcontext.GuildID, origin.User.ID, role.ID)
				} else {
					context.session.GuildMemberRoleRemove(context.msgcontext.GuildID, origin.User.ID, role.ID)
				}
			} else {
				return errors.New(fmt.Sprintf("I'm sorry %s, I'm afraid I can't do that.", getDisplayName(context.origin))), ""
			}
		} else {
			return errors.New(fmt.Sprintf("Sorry %s, I could not find the role %s!", getDisplayName(context.origin), arg)), ""
		}
	}
	return nil, ""
}

func getDisplayName(member *discordgo.Member) string {
	if member.Nick == "" {
		return member.User.Username
	}
	return member.Nick
}

func cmdAdd(context CmdContext) (error, string) {
	if canFindAny(context.origin.Roles, CONFIG.Roles.Admins) {
		roles, err := context.session.GuildRoles(context.msgcontext.GuildID)
		if err != nil {
			return err, ""
		}

		for _, role := range context.params {
			if findRoleById(role, roles) != nil {
				if canFind(role, CONFIG.Roles.Admins) {
					context.session.MessageReactionAdd(context.msgcontext.ChannelID, context.msgcontext.ID, "❌")
					return errors.New(fmt.Sprintf("Sorry %s, adding an admin role as settable is a dangerous operation; and thus is not permitted.", getDisplayName(context.origin))), ""
				}
				continue
			}
			context.session.MessageReactionAdd(context.msgcontext.ChannelID, context.msgcontext.ID, "❌")
			return errors.New(fmt.Sprintf("Sorry %s, I could not find the role id %s!", getDisplayName(context.origin), role)), ""
		}

		CONFIG.Roles.Allowed = append(CONFIG.Roles.Allowed, context.params...)
		CONFIG.Save()
		return nil, ""
	}
	return errors.New(fmt.Sprintf("I'm sorry %s, I'm afraid I can't do that.", getDisplayName(context.origin))), ""
}

func cmdList(context CmdContext) (error, string) {
	roles, err := context.session.GuildRoles(context.origin.GuildID)
	if err != nil {
		return err, ""
	}

	output := "```\n"
	for _, role := range roles {
		if canFind(role.ID, CONFIG.Roles.Allowed) {
			output += role.Name + "\n"
		}
	}
	output += "```"
	return nil, output
}

func cmdHelp(context CmdContext) (error, string) {
	return nil, fmt.Sprintf("```\n"+
		"%srole/pronoun/neuro name (name...)     - Set cosmetic roles for pronouns and neurodiverse traits.\n"+
		"%slistroles                             - List the available roles\n"+
		"```", CONFIG.CmdPrefix, CONFIG.CmdPrefix)
}
