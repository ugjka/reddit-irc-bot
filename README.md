# reddit-irc-bot
Irc bot that posts newest posts from your frontpage or anywhere else

```go
func main() {
	bot.New(bot.Oauth2{
		ClientID: "<your reddit app id>",
		Secret:    "<your reddit app secret>"",
		Developer: "<your reddit username>",
		Password:  "<your reddit password>",
		UserAgent: "<unique user agent (reddit bans generic user agents)>",
	}, bot.Irc{
		IrcNick:    "ircnick123",
		IrcName:    "reddit_bot",
		IrcServer:  "irc.freenode.net:6666",
		IrcChannel: []string{"#testchannel875"},
		IrcTLS:		 false,
	}, bot.API{
		Refresh:  time.Minute, // How often check for new posts
		// Api endpoints for your frontpage, can be anything like /r/askreddit/new
		Endpoint: []string{"/new?limit=20", "/r/askreddit/new?limit=20", "/me/m/mymulti/new?limit=20"}
	}).Start()
}
```
