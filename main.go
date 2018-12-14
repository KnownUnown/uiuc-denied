package main

import (
	"fmt"
	"flag"
	"time"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/publicsuffix"
	"github.com/PuerkitoBio/goquery"
	"github.com/gregdel/pushover"
	//"github.com/davecgh/go-spew/spew"
)

var doomsday = time.Date(2018, time.December, 14, 16, 0, 1, 0, mustLoadLocation("America/Chicago"))
var decisionReleased = false

func mustLoadLocation(loc string) *time.Location {
	if dat, err := time.LoadLocation(loc); err != nil {
		panic(err)
	} else {
		return dat
	}
}

func main() {
	key := flag.String("pushkey", "Pushover API key", "")
	rcp := flag.String("recipient", "Pushover recipient", "")
	u := flag.String("username", "MyIllini username", "")
	p := flag.String("password", "MyIllini password", "")
	flag.Parse()

	username = *u
	password = *p

	var err error

	jar, err = cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		panic(err)
	}

	client = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Jar: jar,
	}
	notif = pushover.New(*key)
	recipient = pushover.NewRecipient(*rcp)

	nextTime := doomsday.Add(-24 * time.Hour)
	for !decisionReleased {
		str := fmt.Sprintf("%.1fh remaining until decision.\n", doomsday.Sub(nextTime).Hours())
		fmt.Println(str)

		msg := pushover.NewMessageWithTitle(str, "You lose!")
		notif.SendMessage(msg, recipient)

		time.Sleep(time.Until(nextTime))
		check()

		if time.Now().Before(doomsday) {
			nextTime = time.Now().Add(time.Hour).Truncate(time.Hour)
		} else {
			nextTime = time.Now().Add(5 * time.Minute).Truncate(time.Minute)
		}
	}

	diff := doomsday.Sub(time.Now())
	var timely string
	if diff < 0 {
		timely = "late"
	} else {
		timely = "early"
	}
	fmt.Printf("Final time: %s\n %s", diff.String(), timely)
}

var (
	jar *cookiejar.Jar
	client *http.Client
	notif *pushover.Pushover
	recipient *pushover.Recipient

	username string
	password string
)

var (
	baseUrl = "https://myillini.illinois.edu"
	loginPage = baseUrl + "/IdentityManagement/Home/Login"
	statusPage = baseUrl + "/Apply/Application/Status"
	statusMatch = regexp.MustCompile("admission decision: ([a-z]+)") // https://talk.collegeconfidential.com/university-illinois-urbana-champaign/1496755-the-worst-rejection-ever.html
)

func check() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Error occurred while checking status: %s\n", r)
			return
		}
	}()

	var (
		r *http.Response
		retries = 3
		interval = 5 * time.Second
	)
	loop:
	for retries >= 0 {
		switch r = request(statusPage); r.StatusCode {
		case 302:
			if retries < 1 {
				panic("failed to log in")
			}

			login()
		case 200:
			fmt.Println("Login OK!")
			break loop
		default:
			panic(fmt.Sprintf("unhandled status code %d", r.StatusCode))
		}

		fmt.Printf("Login failed. %d attempts remaining...\n", retries-1)
		time.Sleep(interval)
		retries--
	}

	doc, err := goquery.NewDocumentFromResponse(r)
	if err != nil {
		panic(err)
	}
	status := strings.ToLower(doc.Find("div #statusArea").Text())
	matches := statusMatch.FindStringSubmatch(status)
	if len(matches) < 2 {
		return
	}

	// decisions released!
	decisionReleased = true
	result := matches[1]
	fmt.Printf("result: %s\n", result)

	msg := &pushover.Message{
		Message: "Admission Result: " + result,
		Title: "You lose!",
		Priority: pushover.PriorityEmergency,
		URL: statusPage,
		URLTitle: "Admissions Portal",
		Retry: 60 * time.Second,
		Expire: time.Hour,
		Sound: pushover.SoundBike,
	}

	_, err = notif.SendMessage(msg, recipient)
	if err != nil {
		fmt.Printf("Failed to push to device: %s\n", err)
	}
}

func login() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Error occurred while trying to log in: %s\n", r)
			return
		}
	}()

	r := request(loginPage)
	if r.StatusCode != 200 {
		panic(fmt.Sprintf("Unexpected status %d", r.StatusCode))
	}

	doc, err := goquery.NewDocumentFromResponse(r)
	if err != nil {
		panic(err)
	}
	token, _ := doc.Find("input[name=__RequestVerificationToken]").Attr("value")

	v := url.Values{}
	v.Set("Username", username)
	v.Set("Password", password)
	v.Set("__RequestVerificationToken", token)

	if _, err = client.PostForm(loginPage, v); err != nil {
		panic(err)
	}
}

func request(url string) *http.Response {
	resp, err := client.Get(url)
	if err != nil {
		panic(err)
	}

	return resp
}
