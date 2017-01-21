# reddit-irc-bot
Irc bot that posts newest posts from your frontpage or anywhere else

```go
func main() {
	bot.Start(bot.Oauth2{
		Client_id: "<your reddit app id>",
		Secret:    "<your reddit app secret>"",
		Developer: "<your reddit username>",
		Password:  "<your reddit password>",
		UserAgent: "<unique user agent (reddit bans generic user agents)>",
	}, bot.Irc{
		Irc_nick:    "ircnick123",
		Irc_name:    "reddit_bot",
		Irc_server:  "irc.freenode.net:6666",
		Irc_channel: []string{"#testchannel875"},
		Irc_tls:		 false,
	}, bot.Api{
		Refresh:  time.Minute, // How often check for new posts
		// Api endpoints for your frontpage, can be anything like /r/askreddit/new
		Endpoint: []string{"/new?limit=20", "/r/askreddit/new?limit=20", "/me/m/mymulti/new?limit=20"}
	})
}
```
