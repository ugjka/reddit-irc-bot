// Reddit Irc Bot that posts newest reddit posts from your frontpage or
// Subreddits you like
package bot

import (
	"encoding/json"
	"errors"
	"github.com/martinlindhe/base36"
	irc "github.com/thoj/go-ircevent"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

type Oauth2 struct {
	Client_id string
	Secret    string
	Developer string
	Password  string
	UserAgent string
}

type Irc struct {
	Irc_nick    string
	Irc_name    string
	Irc_server  string
	Irc_channel string
}

type Api struct {
	Refresh  time.Duration
	Endpoint string
}

// Oauth2 json
type token struct {
	Access_token string `json:"access_token"`
	Token_type   string `json:"token_type"`
	Expires_in   uint   `json:"expires_in"`
	Scope        string `json:"scope"`
}

// Posts json
type posts struct {
	Data struct {
		Children []struct {
			Data struct {
				Subreddit string `json:"subreddit"`
				Title     string `json:"title"`
				Permalink string `json:"permalink"`
				Id        string `json:"id"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

const getTokenUrl = "https://www.reddit.com/api/v1/access_token"

// Get Oaut2 token
func getToken(auth Oauth2, t *token) error {
	post := "grant_type=password&username=" + auth.Developer + "&password=" + auth.Password
	req, err := http.NewRequest("POST", getTokenUrl, strings.NewReader(post))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", auth.UserAgent)
	req.SetBasicAuth(auth.Client_id, auth.Secret)

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

	err = json.Unmarshal(body, &t)
	if err != nil {
		return err
	}
	return nil
}

// Get posts
func fetchNewest(auth Oauth2, api Api, t *token, p *posts) error {
	req, err := http.NewRequest("GET", "https://oauth.reddit.com"+api.Endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", auth.UserAgent)
	req.Header.Set("Authorization", "bearer "+t.Access_token)

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
	err = json.Unmarshal(body, &p)
	if err != nil {
		return err
	}
	return nil
}

// Makes a map of posts formatted for IRC
func (p posts) parse(last_id *uint64) (s map[int]string, e error) {
	s = make(map[int]string)
	for i, _ := range p.Data.Children {
		id_uint := base36.Decode(p.Data.Children[i].Data.Id)
		if id_uint > *last_id {
			s[i] = "\x02\x035[reddit]\x03 \x0312[/r/" + p.Data.Children[i].Data.Subreddit + "]\x03 " + p.Data.Children[i].Data.Title + "\x02" + " " + "https://reddit.com" + p.Data.Children[i].Data.Permalink
		}
	}

	// Id's dont come ordered so we need to figure out the biggest ID I'd so that we
	// dont get duplicate posts
	var max uint64 = 0
	for i := 0; i < len(p.Data.Children); i++ {
		id_uint := base36.Decode(p.Data.Children[i].Data.Id)
		if max < id_uint {
			max = id_uint
		}
	}
	*last_id = max
	return s, nil
}

// Start the bot
func Start(auth Oauth2, bot Irc, api Api) {
	// Updated by getToken
	var t token
	// Updated by fetchNewest
	var p posts

	// Variable for checking highest id
	var last_id uint64 = 0

	// For initializing
	started := false
	// Start the Irc Bot
	ircobj := irc.IRC(bot.Irc_nick, bot.Irc_name)
	ircobj.Connect(bot.Irc_server)
	ircobj.Join(bot.Irc_channel)
	go ircobj.Loop()

	// posts posts to irc
	print := func() {
		s, err := p.parse(&last_id)
		if err != nil {
			log.Println(err)
			return
		}
		for _, v := range s {
			ircobj.Privmsg(bot.Irc_channel, v)
			// Delay between posts to avoid flooding
			time.Sleep(time.Second * 1)
		}
	}

	// Initialize
	for {
		if started == true {
			break
		}
		err := getToken(auth, &t)
		if err != nil {
			log.Println(err)
			time.Sleep(time.Minute)
			continue
		}
		err = fetchNewest(auth, api, &t, &p)
		if err != nil {
			log.Println(err)
			time.Sleep(time.Minute)
			continue
		}
		started = true
		p.parse(&last_id)
	}

	tokenTicker := time.NewTicker(time.Second*time.Duration(t.Expires_in) - api.Refresh)
	postsTicker := time.NewTicker(api.Refresh)

	// Perform tasks on tickers
	for {
		select {
		case <-tokenTicker.C:
			err := getToken(auth, &t)
			if err != nil {
				log.Println("Error getting oauth2 token: ", err)
				for {
					time.Sleep(time.Minute)
					if getToken(auth, &t) == nil {
						break
					}
				}
			}
		case <-postsTicker.C:
			err := fetchNewest(auth, api, &t, &p)
			if err == nil {
				print()
			} else {
				log.Println("Error in fetching posts: ", err)
			}
		}
	}
}
