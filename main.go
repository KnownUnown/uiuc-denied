package main

import (
	"fmt"
	"flag"
	"time"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"

	"golang.org/x/net/publicsuffix"
	"github.com/PuerkitoBio/goquery"
	"github.com/gregdel/pushover"
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

	interval := int64(1)
	unit := time.Hour
	for {
		if check() {
			break
		}

		str := fmt.Sprintf("%s remaining until decision.\n", doomsday.Sub(time.Now()).String())
		fmt.Println(str)

		msg := pushover.NewMessageWithTitle(str, "You lose!")
		notif.SendMessage(msg, recipient)

		wakeupTime := time.Now().Truncate(unit).Add(unit*time.Duration(interval))
		fmt.Printf("Next wakeup: %s\n", wakeupTime.String())
		time.Sleep(time.Until(wakeupTime))

		r := doomsday.Sub(time.Now())
		switch {
		case r < 1 * time.Hour: // 1 hour remaining: 15 minute interval
			interval = 15
			unit = time.Minute
		case (1 * time.Hour) < r && r < (2 * time.Hour): // ~2 hours remaining: 30 minute interval
			interval = 30
			unit = time.Minute
		default: // any other time: 1 hour interval
			interval = 1
			unit = time.Hour
		}
	}

	diff := doomsday.Sub(time.Now())
	var timely string
	if diff < 0 {
		timely = "late"
	} else {
		timely = "early"
	}
	fmt.Printf("Decision released, exiting. Final time: %s (%s)\n", diff.String(), timely)
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
	statusMatch = regexp.MustCompile("(?i)decision:\\s+([a-z]+)")
)

func check() (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Error occurred while checking status: %s\n", r)
			return
		}
	}()

	var (
		r *http.Response
		retries = 3
		interval = 5 * time.Second // don't hammer the endpoint
	)
	loop:
	for {
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
			panic(fmt.Sprintf("unhandled status code %d (server bad?)", r.StatusCode))
		}

		retries--
		fmt.Printf("Trying to login, %d attempts remaining...\n", retries)
		time.Sleep(interval)
	}

	doc, err := goquery.NewDocumentFromResponse(r)
	if err != nil {
		panic(err)
	}
	status := doc.Find("div #statusArea").Text()
	matches := statusMatch.FindStringSubmatch(status)

	fmt.Printf("Result:\n%s\n", status)
	if len(matches) < 2 {
		return
	}

	// decisions released!
	ok = true
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

	return
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
