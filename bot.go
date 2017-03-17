//Package bot is Reddit Irc Bot that posts newest reddit posts from your frontpage or any subreddit
package bot

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/martinlindhe/base36"
	irc "github.com/ugjka/dumbirc"
)

//Oauth2 settings
type Oauth2 struct {
	ClientID  string
	Secret    string
	Developer string
	Password  string
	UserAgent string
}

//Irc settings
type Irc struct {
	IrcNick    string
	IrcName    string
	IrcServer  string
	IrcChannel []string
	IrcTLS     bool
}

//API settings
type API struct {
	Refresh  time.Duration
	Endpoint []string
}

// Oauth2 json
type token struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   uint   `json:"expires_in"`
	Scope       string `json:"scope"`
}

// Posts json
type posts struct {
	Data struct {
		Children []struct {
			Data struct {
				Subreddit string `json:"subreddit"`
				Title     string `json:"title"`
				Permalink string `json:"permalink"`
				ID        string `json:"id"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type multi struct {
	endpoint string
	p        posts
	lastID   uint64
}

const getTokenURL = "https://www.reddit.com/api/v1/access_token"

// Get Oaut2 token
func getToken(auth Oauth2, t *token) error {
	post := "grant_type=password&username=" + auth.Developer + "&password=" + auth.Password
	req, err := http.NewRequest("POST", getTokenURL, strings.NewReader(post))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", auth.UserAgent)
	req.SetBasicAuth(auth.ClientID, auth.Secret)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.Status != "200 OK" {
		return errors.New("getToken response error: " + resp.Status)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, &t); err != nil {
		return err
	}
	return nil
}

// Get posts
func fetchNewest(auth Oauth2, t *token, p *posts, endpoint string) error {
	req, err := http.NewRequest("GET", "https://oauth.reddit.com"+endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", auth.UserAgent)
	req.Header.Set("Authorization", "bearer "+t.AccessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.Status != "200 OK" {
		return errors.New("fetchNewest response error: " + resp.Status)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	return nil
}

// Makes a map of posts formatted for IRC
func (p posts) parse(lastID *uint64) (s map[int]string) {
	s = make(map[int]string)
	for i := range p.Data.Children {
		idUint := base36.Decode(p.Data.Children[i].Data.ID)
		if idUint > *lastID {
			s[i] = "\x02\x035[reddit]\x03 \x0312[/r/" + p.Data.Children[i].Data.Subreddit + "]\x03 " + p.Data.Children[i].Data.Title + "\x02" + " " + "https://redd.it/" + p.Data.Children[i].Data.ID
		}
	}

	// Id's dont come ordered so we need to figure out the biggest ID I'd so that we
	// dont get duplicate posts
	var max uint64
	for i := 0; i < len(p.Data.Children); i++ {
		idUint := base36.Decode(p.Data.Children[i].Data.ID)
		if max < idUint {
			max = idUint
		}
	}
	*lastID = max
	return s
}

// Start the bot
func Start(auth Oauth2, bot Irc, api API) {
	// Updated by getToken
	var t token
	// Initialize empty struct
	var p posts
	multiSlice := make([]multi, 0)
	for _, k := range api.Endpoint {
		multiSlice = append(multiSlice, multi{k, p, 0})
	}
	//Ignore first run
	started := false
	// Start the Irc Bot
	ircobj := irc.New(bot.IrcNick, bot.IrcName, bot.IrcServer, bot.IrcTLS)
	//Rejoin the channel on reconnect
	ircobj.AddCallback(irc.WELCOME, func(msg irc.Message) {
		ircobj.Join(bot.IrcChannel)
	})
	ircobj.AddCallback(irc.PING, func(msg irc.Message) {
		ircobj.Pong()
	})
	ircobj.AddCallback(irc.NICKTAKEN, func(msg irc.Message) {
		ircobj.Nick += "_"
		ircobj.NewNick(ircobj.Nick)
	})
	//Connect
	ircobj.Start()
	// Prints to IRC channel
	print := func(p *multi) {
		s := p.p.parse(&p.lastID)
		for _, v := range s {
			for _, ch := range bot.IrcChannel {
				ircobj.PrivMsg(ch, v)
			}
			// Delay between posts to avoid flooding
			time.Sleep(time.Second * 1)
		}
	}

	go func() {
		// Initialize
		for {
			if started == true {
				break
			}

			if err := getToken(auth, &t); err != nil {
				log.Println(err)
				time.Sleep(time.Minute)
				continue
			}
			for i := range multiSlice {
				if err := fetchNewest(auth, &t, &multiSlice[i].p, multiSlice[i].endpoint); err != nil {
					log.Println(err)
					time.Sleep(time.Minute)
					continue
				}
				time.Sleep(time.Second)
			}
			started = true
			for i := range multiSlice {
				multiSlice[i].p.parse(&multiSlice[i].lastID)
			}
		}

		tokenTicker := time.NewTicker(time.Second*time.Duration(t.ExpiresIn) - api.Refresh)
		postsTicker := time.NewTicker(api.Refresh)

		// Perform tasks on tickers
		for {
			select {
			case <-tokenTicker.C:
				if err := getToken(auth, &t); err != nil {
					log.Println("Oauth2: ", err)
					for {
						time.Sleep(time.Minute)
						if getToken(auth, &t) == nil {
							break
						}
					}
				}
			case <-postsTicker.C:
				for i := range multiSlice {
					if err := fetchNewest(auth, &t, &multiSlice[i].p, multiSlice[i].endpoint); err == nil {
						print(&multiSlice[i])
					} else {
						log.Println("Fetching Posts: ", err)
					}
					time.Sleep(time.Second)
				}
			}
		}
	}()
	//Pinger
	go func() {
		for {
			time.Sleep(time.Minute)
			ircobj.Ping()
		}
	}()
	//Irc loop/Recconect logic
	for {
		log.Println(<-ircobj.Errchan)
		time.Sleep(time.Second * 30)
		ircobj.Start()
	}
}
