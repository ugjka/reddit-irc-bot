//Package bot is Reddit Irc Bot that posts newest reddit posts from your frontpage or any subreddit
package bot

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ugjka/dumbirc"

	"github.com/martinlindhe/base36"
)

var client = &http.Client{}

//Bot is a bot construct
type Bot struct {
	oauth       Oauth2
	irc         Irc
	api         API
	token       *token
	ircConn     *dumbirc.Connection
	fetchTicker *time.Ticker
	lastID      uint64
	send        chan string
	pp          chan bool
}

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

// Post ...
type post struct {
	Subreddit string
	Title     string
	Permalink string
	ID        string
	IDdecoded uint64
}

// Posts ...
type posts []post

func (p post) String() string {
	return "\x02\x035[reddit]\x03 \x0312[/r/" + p.Subreddit + "]\x03 " + p.Title + "\x02" + " " + "https://redd.it/" + p.ID
}

// UnmarshalJSON ...
func (p *posts) UnmarshalJSON(data []byte) error {
	var v = struct {
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
	}{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	for _, v := range v.Data.Children {
		*p = append(*p, post{
			Subreddit: v.Data.Subreddit,
			Title:     v.Data.Title,
			Permalink: v.Data.Permalink,
			ID:        v.Data.ID,
			IDdecoded: base36.Decode(v.Data.ID),
		})
	}
	return nil
}

const getTokenURL = "https://www.reddit.com/api/v1/access_token"

// Get Oaut2 token
func (o Oauth2) getToken() (t *token, err error) {
	values := url.Values{}
	values.Add("grant_type", "password")
	values.Add("username", o.Developer)
	values.Add("password", o.Password)
	req, err := http.NewRequest("POST", getTokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", o.UserAgent)
	req.SetBasicAuth(o.ClientID, o.Secret)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Token response error: " + resp.Status)
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return t, nil
}

// Get posts
func (b *Bot) fetch(endpoint string) (p *posts, err error) {
	req, err := http.NewRequest("GET", "https://oauth.reddit.com"+endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", b.oauth.UserAgent)
	req.Header.Set("Authorization", "bearer "+b.token.AccessToken)
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("fetch response error: " + resp.Status)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &p)
	return
}

//New creates a new bot object
func New(oauth Oauth2, irc Irc, api API) *Bot {
	return &Bot{
		oauth:       oauth,
		irc:         irc,
		api:         api,
		ircConn:     dumbirc.New(irc.IrcNick, irc.IrcName, irc.IrcServer, irc.IrcTLS),
		fetchTicker: time.NewTicker(api.Refresh),
		send:        make(chan string, 100),
		pp:          make(chan bool, 1),
	}
}

func (b *Bot) addCallbacks() {
	irc := b.ircConn
	irc.AddCallback(dumbirc.WELCOME, func(msg *dumbirc.Message) {
		log.Println("Joining channels")
		irc.Join(b.irc.IrcChannel)
	})
	irc.AddCallback(dumbirc.PING, func(msg *dumbirc.Message) {
		log.Println("PING received, sending PONG")
		irc.Pong()
	})
	irc.AddCallback(dumbirc.NICKTAKEN, func(msg *dumbirc.Message) {
		log.Println("Nick taken, changing nick")
		irc.Nick = changeNick(irc.Nick)
		irc.NewNick(irc.Nick)
	})
	irc.AddCallback(dumbirc.ANYMESSAGE, func(msg *dumbirc.Message) {
		pingpong(b.pp)
	})
}

func pingpong(c chan bool) {
	select {
	case c <- true:
	default:
		return
	}
}

func changeNick(n string) string {
	if len(n) < 16 {
		n += "_"
		return n
	}
	n = strings.TrimRight(n, "_")
	if len(n) > 12 {
		n = n[:12] + "_"
	}
	return n
}

func (b *Bot) firstRun() error {
	for _, v := range b.api.Endpoint {
		posts, err := b.fetch(v)
		if err != nil {
			log.Println("First run", err)
			return err
		}
		for _, v := range *posts {
			if b.lastID < v.IDdecoded {
				b.lastID = v.IDdecoded
			}
		}
	}
	return nil
}

//Start starts the bot
func (b *Bot) Start() {
	b.addCallbacks()
	b.ircConn.Start()
	var err error
	for {
		log.Println("Fetching oauth token")
		b.token, err = b.oauth.getToken()
		if err == nil {
			log.Println("Got oauth token!")
			break
		}
		log.Println("Could not get oauth token", err)
		log.Println("Retrying to get ouauth token")
		time.Sleep(time.Minute)
	}
	for {
		err := b.firstRun()
		if err == nil {
			log.Println("first run succeeded")
			break
		}
		log.Println("first run failed:", err)
		time.Sleep(time.Minute)
		log.Println("retrying first run")
	}
	go b.printer()
	go b.ircControl()
	b.mainLoop()
}

func (b *Bot) printer() {
	irc := b.ircConn
	for v := range b.send {
		irc.MsgBulk(b.irc.IrcChannel, v)
		time.Sleep(time.Second * 1)
	}
}

func (b *Bot) ircControl() {
	irc := b.ircConn
	pingTick := time.NewTicker(time.Minute * 1)
	for {
		select {
		case err := <-irc.Errchan:
			log.Println("Irc error", err)
			log.Println("Restarting irc")
			time.Sleep(time.Minute * 1)
			irc.Start()
		case <-pingTick.C:
			select {
			case <-b.pp:
				irc.Ping()
			default:
				log.Println("Got No Pong")
			}
		}
	}
}

func (b *Bot) mainLoop() {
	if b.token == nil {
		log.Fatal("Token is nil")
	}
	tokenTimer := time.NewTimer((time.Second*time.Duration(b.token.ExpiresIn) - b.api.Refresh))
	var err error
	for {
		select {
		case <-b.fetchTicker.C:
			b.getPosts()
		case <-tokenTimer.C:
			for {
				b.token, err = b.oauth.getToken()
				if err == nil {
					break
				}
				log.Println("Could not get oauth token", err)
				log.Println("Retrying to get oauth token")
				time.Sleep(time.Minute)
			}
			tokenTimer.Stop()
			tokenTimer = time.NewTimer((time.Second*time.Duration(b.token.ExpiresIn) - b.api.Refresh))
		}

	}
}

func (b *Bot) getPosts() {
	var tmpLargest uint64
	for _, v := range b.api.Endpoint {
		posts, err := b.fetch(v)
		if err != nil {
			log.Println("Could not fetch posts:", err)
			return
		}
		for _, v := range *posts {
			if tmpLargest < v.IDdecoded {
				tmpLargest = v.IDdecoded
			}
			if b.lastID < v.IDdecoded {
				b.send <- v.String()
			}
		}
	}
	b.lastID = tmpLargest
}
